package work_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
	"agent-office/internal/work"
)

// autoFixture is a service with marketing, development and finance led
// by AIs on a test connection, with a running engine, both AI lanes, the
// delivery processor and the auto starter. answer decides what each AI
// call returns, by what it is for.
type autoFixture struct {
	db            *storage.DB
	eng           *engine.Engine
	ai            *work.AI
	proc          *work.Processor
	auto          *work.AutoStarter
	p             domain.Project
	conn          domain.ProviderConnection
	mkt, dev, fin domain.Department
	leads         map[string]domain.Assignment
	mu            sync.Mutex
	calls         []string
	answer        func(what, prompt string) (string, error)
}

func newAutoFixture(t *testing.T, mode domain.ProjectMode) *autoFixture {
	t.Helper()
	ctx := context.Background()
	f := &autoFixture{leads: map[string]domain.Assignment{}}
	f.db = open(t, filepath.Join(t.TempDir(), "auto.db"))
	t.Cleanup(func() { f.db.Close() })
	p, _ := f.db.CreateProject(ctx, domain.Project{Name: "캠페인"})
	p.Mode, p.Budget = mode, json.RawMessage(`{"maxTokens": 100000}`)
	p, err := f.db.UpdateProject(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	f.p = p
	f.conn, _ = f.db.SaveConnection(ctx, domain.ProviderConnection{Name: "test", Provider: "test"})
	f.allowConnections(t, f.conn.ID)
	dept := func(name string) domain.Department {
		role, _ := f.db.SaveRole(ctx, domain.Role{Name: name + " 담당"})
		a, _ := f.db.SaveAssignment(ctx, domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
			DisplayName: name + " AI", ConnectionID: f.conn.ID, Model: providers.AutoScenario})
		d, err := f.db.SaveDepartment(ctx, domain.Department{ProjectID: p.ID, Name: name, Responsibility: name + "의 책임",
			MemberIDs: []string{a.ID}, LeadAssignmentID: a.ID})
		if err != nil {
			t.Fatal(err)
		}
		f.leads[d.ID] = a
		return d
	}
	f.mkt, f.dev, f.fin = dept("마케팅팀"), dept("개발팀"), dept("재무팀")

	f.eng = engine.New(engine.Config{DB: f.db, DataDir: t.TempDir(), Logf: t.Logf,
		Providers: func(domain.ProviderConnection) (providers.Provider, error) {
			return &providers.TestProvider{Scripts: map[string][]providers.Step{}}, nil
		}})
	ectx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { f.eng.Run(ectx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	hold := func(ctx context.Context, projectID string) (string, error) {
		u, err := f.eng.ProjectUsage(ctx, projectID)
		return u.HoldReason, err
	}
	f.ai = work.NewAI(work.AIConfig{
		DB:        f.db,
		Providers: func(domain.ProviderConnection) (providers.Provider, error) { return nil, nil },
		Hold:      hold,
		Kinds: map[string]work.Kind{storage.JobPlan: work.PlanKind{DB: f.db}, storage.JobCollabDecision: work.CollabKind{DB: f.db}},
		Call: func(ctx context.Context, _ providers.Provider, c design.JSONCall) ([]byte, error) {
			f.mu.Lock()
			f.calls = append(f.calls, c.What)
			f.mu.Unlock()
			c.OnEvent(providers.Event{Kind: providers.KindUsage, Payload: json.RawMessage(`{"inputTokens":20,"outputTokens":10,"costUsd":0.001}`)})
			out, err := f.answer(c.What, c.Prompt)
			return []byte(out), err
		},
	})
	f.proc = work.New(f.db, work.Judge(f.db, nil))
	f.auto = work.NewAutoStarter(work.AutoConfig{DB: f.db, Hold: hold,
		Start: func(ctx context.Context, projectID, workItemID string) (string, error) {
			return f.eng.StartWork(ctx, projectID, workItemID, "")
		},
		Share: f.eng.ShareRunResults})
	f.answer = func(what, prompt string) (string, error) {
		switch {
		case what == "작업 계획 AI":
			id := between(prompt, "(보통 ", ")")
			return fmt.Sprintf(`{"assignmentId":%q,"instructions":"요청을 검토해 정리한다","outputs":[{"key":"result","type":"markdown"}],"notes":"가정 없음"}`, id), nil
		case strings.HasSuffix(what, " 협업 결정 AI"):
			return `{"decision":"accept","reason":"우리 부서의 책임이다","title":"` + strings.TrimSuffix(what, " 협업 결정 AI") + ` 검토","doneCriteria":"검토 결과를 공유한다"}`, nil
		}
		return "", errors.New("unexpected call: " + what)
	}
	return f
}

func between(s, a, b string) string {
	i := strings.Index(s, a)
	if i < 0 {
		return ""
	}
	s = s[i+len(a):]
	if j := strings.Index(s, b); j >= 0 {
		return s[:j]
	}
	return s
}

func (f *autoFixture) allowConnections(t *testing.T, ids ...string) {
	t.Helper()
	raw, _ := json.Marshal(design.AutoPolicy{AllowedConnectionIDs: ids})
	if err := f.db.SetAutoPolicy(context.Background(), f.p.ID, raw); err != nil {
		t.Fatal(err)
	}
}

func (f *autoFixture) rules(t *testing.T, d domain.Department, a domain.DepartmentAutomation) {
	t.Helper()
	if _, err := f.db.SetDepartmentAutomation(context.Background(), f.p.ID, d.ID, a); err != nil {
		t.Fatal(err)
	}
}

// settle drives everything until until() holds or nothing moves for a
// while (runs finish in the engine's own goroutine).
func (f *autoFixture) settle(t *testing.T, until func() bool) {
	t.Helper()
	ctx := context.Background()
	deadline := time.Now().Add(20 * time.Second)
	for {
		n1, err1 := f.proc.Drain(ctx)
		n2, err2 := f.ai.Drain(ctx, storage.LaneBackground)
		n3, err3 := f.ai.Drain(ctx, storage.LaneInteractive)
		n4, err4 := f.auto.Drain(ctx)
		if err := errors.Join(err1, err2, err3, err4); err != nil {
			t.Fatal(err)
		}
		if until != nil && until() {
			return
		}
		if until == nil && n1+n2+n3+n4 == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("did not settle")
		}
		if n1+n2+n3+n4 == 0 {
			time.Sleep(20 * time.Millisecond)
		}
	}
}

func (f *autoFixture) autoStart(t *testing.T, workID string) storage.AutoStart {
	t.Helper()
	list, err := f.db.AutoStarts(context.Background(), f.p.ID, storage.AutoStartFilter{WorkItemID: workID})
	if err != nil || len(list) != 1 {
		t.Fatalf("auto starts of %s: %+v %v", workID, list, err)
	}
	return list[0]
}

func (f *autoFixture) request(t *testing.T, from domain.WorkItem, to domain.Department, requirement string) storage.CollabRequest {
	t.Helper()
	r, err := f.db.RequestCollab(context.Background(), f.p.ID, storage.NewCollabRequest{FromWorkItemID: from.ID, ToDepartmentID: to.ID,
		Question: to.Name + "에서 검토해 주세요", Requirement: requirement})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

var allOn = domain.DepartmentAutomation{AutoStart: true, AutoShare: true, AutoCollab: true}

// Stage 7 acceptance: without any fixed graph between departments,
// development and finance take marketing's reference requests on through
// their lead AIs, each work item gets an AI plan, starts on its own and
// runs as a run of its own, and the results are shared as drafts that
// continue the request's chain. The person still completes the work.
func TestAutoModeRunsAllowedWorkAndSharesResults(t *testing.T) {
	f := newAutoFixture(t, domain.ModeAuto)
	ctx := context.Background()
	f.rules(t, f.dev, allOn)
	f.rules(t, f.fin, allOn)
	plan, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, DepartmentID: f.mkt.ID, Title: "캠페인 기획"})
	toDev := f.request(t, plan, f.dev, storage.CollabReference)
	toFin := f.request(t, plan, f.fin, storage.CollabReference)

	var devReq, finReq storage.CollabRequest
	f.settle(t, func() bool {
		devReq, _ = f.db.CollabRequest(ctx, f.p.ID, toDev.ID)
		finReq, _ = f.db.CollabRequest(ctx, f.p.ID, toFin.ID)
		if devReq.ResultWorkItemID == "" || finReq.ResultWorkItemID == "" {
			return false
		}
		return f.autoStart(t, devReq.ResultWorkItemID).Status == storage.AutoFinished && f.autoStart(t, finReq.ResultWorkItemID).Status == storage.AutoFinished
	})
	for _, r := range []storage.CollabRequest{devReq, finReq} {
		if r.Status != storage.CollabAccepted || r.DecidedBy != "ai" || r.DecisionJobID == "" || !strings.HasPrefix(r.History[len(r.History)-1].Author, "ai:") {
			t.Fatalf("AI decision = %+v", r)
		}
		a := f.autoStart(t, r.ResultWorkItemID)
		w, _ := f.db.WorkItem(ctx, f.p.ID, r.ResultWorkItemID)
		if w.Status != domain.WorkWaiting || !strings.Contains(a.Note, "초안으로 공유") {
			t.Fatalf("%s after the automatic run: %s, %+v", w.Title, w.Status, a)
		}
		if ap, _ := f.db.WorkStartApproval(ctx, f.p.ID, w.ID); ap.By != "auto" || !ap.Valid {
			t.Fatalf("approval = %+v", ap)
		}
		evs, _ := f.db.WorkEvents(ctx, f.p.ID, w.ID)
		created := evs[0].Event
		if !created.Automated || created.ChainDepth != 1 || created.ChainRootSequence == nil || *created.ChainRootSequence != r.EventSequence {
			t.Fatalf("work registered by AI is not in the request's chain: %+v", created)
		}
		docs, _ := f.db.Documents(ctx, f.p.ID, storage.DocumentFilter{})
		shared := 0
		for _, d := range docs {
			if d.WorkItemID == w.ID && d.Versions[0].Stage == "draft" {
				shared++
			}
		}
		if shared != 1 {
			t.Fatalf("%s shared %d drafts", w.Title, shared)
		}
	}
	devRun, finRun := f.autoStart(t, devReq.ResultWorkItemID).RunID, f.autoStart(t, finReq.ResultWorkItemID).RunID
	if devRun == finRun {
		t.Fatal("both departments ran in one run")
	}
	// Two decisions and two plans were written by AI; nothing else.
	if len(f.calls) != 4 {
		t.Fatalf("AI calls = %v", f.calls)
	}
}

// In review mode nothing happens on its own: requests wait for the
// person, no AI is called and no work starts.
func TestReviewModeWaits(t *testing.T) {
	f := newAutoFixture(t, domain.ModeReview)
	ctx := context.Background()
	f.rules(t, f.dev, allOn)
	plan, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, DepartmentID: f.mkt.ID, Title: "캠페인 기획"})
	r := f.request(t, plan, f.dev, storage.CollabReference)
	f.settle(t, nil)
	if got, _ := f.db.CollabRequest(ctx, f.p.ID, r.ID); got.Status != storage.CollabRequested {
		t.Fatalf("request in review mode = %s", got.Status)
	}
	acc, err := f.db.AcceptCollab(ctx, f.p.ID, r.ID, storage.CollabAcceptance{})
	if err != nil {
		t.Fatal(err)
	}
	f.settle(t, nil)
	if list, _ := f.db.AutoStarts(ctx, f.p.ID, storage.AutoStartFilter{}); len(list) != 0 || len(f.calls) != 0 {
		t.Fatalf("review mode started something: %+v %v", list, f.calls)
	}
	if runs, _ := f.db.WorkRunIDs(ctx, f.p.ID, acc.ResultWorkItemID); len(runs) != 0 {
		t.Fatal("review mode ran the work")
	}
}

// Every condition of an automatic start is checked; work that does not
// fit goes to the person with the reason and nothing runs.
func TestAutoStartChecksSendWorkToReview(t *testing.T) {
	type tc struct {
		name, want string
		setup      func(t *testing.T, f *autoFixture, work string)
		rules      domain.DepartmentAutomation
	}
	codePlan := func(t *testing.T, f *autoFixture, work string) {
		if _, err := f.db.SaveWorkPlan(context.Background(), domain.WorkPlan{ProjectID: f.p.ID, WorkItemID: work, AssignmentID: f.leads[f.dev.ID].ID,
			Instructions: "구현", Outputs: []domain.Output{{Key: "patch", Type: domain.OutCodeChange}}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []tc{
		{name: "category", want: "분류", rules: domain.DepartmentAutomation{AutoStart: true, Categories: []string{"비용"}}},
		{name: "output scope", want: "자동 작업 범위 밖", rules: domain.DepartmentAutomation{AutoStart: true}, setup: codePlan},
		{name: "department tools", want: "허용 범위", rules: domain.DepartmentAutomation{AutoStart: true},
			setup: func(t *testing.T, f *autoFixture, _ string) {
				other, _ := f.db.SaveConnection(context.Background(), domain.ProviderConnection{Name: "other", Provider: "test"})
				f.rules(t, f.dev, domain.DepartmentAutomation{AutoStart: true, ConnectionIDs: []string{other.ID}})
			}},
		{name: "service policy", want: "허용 범위", rules: domain.DepartmentAutomation{AutoStart: true},
			setup: func(t *testing.T, f *autoFixture, _ string) { f.allowConnections(t) }},
		{name: "review before start", want: "시작 전 검토", rules: domain.DepartmentAutomation{AutoStart: true},
			setup: func(t *testing.T, f *autoFixture, work string) {
				w, _ := f.db.WorkItem(context.Background(), f.p.ID, work)
				w.ReviewBeforeStart = true
				if _, err := f.db.UpdateWorkItem(context.Background(), w); err != nil {
					t.Fatal(err)
				}
			}},
		{name: "no budget", want: "예산(토큰 또는 비용)", rules: domain.DepartmentAutomation{AutoStart: true},
			setup: func(t *testing.T, f *autoFixture, _ string) {
				p, _ := f.db.Project(context.Background(), f.p.ID)
				p.Budget = json.RawMessage(`{}`)
				f.db.UpdateProject(context.Background(), p)
			}},
		{name: "budget used up", want: "예산 때문에", rules: domain.DepartmentAutomation{AutoStart: true},
			setup: func(t *testing.T, f *autoFixture, _ string) {
				p, _ := f.db.Project(context.Background(), f.p.ID)
				p.Budget = json.RawMessage(`{"maxTokens": 1}`)
				f.db.UpdateProject(context.Background(), p)
				f.spend(t)
			}},
		{name: "human assignee", want: "사람", rules: domain.DepartmentAutomation{AutoStart: true},
			setup: func(t *testing.T, f *autoFixture, work string) {
				role, _ := f.db.SaveRole(context.Background(), domain.Role{Name: "검토자"})
				me, _ := f.db.SaveAssignment(context.Background(), domain.Assignment{ProjectID: f.p.ID, RoleID: role.ID, ActorKind: domain.ActorHuman, DisplayName: "나"})
				f.db.SaveWorkPlan(context.Background(), domain.WorkPlan{ProjectID: f.p.ID, WorkItemID: work, AssignmentID: me.ID, Instructions: "직접"})
			}},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newAutoFixture(t, domain.ModeAuto)
			ctx := context.Background()
			f.rules(t, f.dev, c.rules)
			plan, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, DepartmentID: f.mkt.ID, Title: "기획", Categories: []string{"개발"}})
			r := f.request(t, plan, f.dev, storage.CollabReference)
			acc, err := f.db.AcceptCollab(ctx, f.p.ID, r.ID, storage.CollabAcceptance{})
			if err != nil {
				t.Fatal(err)
			}
			if c.setup != nil {
				c.setup(t, f, acc.ResultWorkItemID)
			}
			f.settle(t, func() bool { return f.autoStart(t, acc.ResultWorkItemID).Status == storage.AutoReview })
			a := f.autoStart(t, acc.ResultWorkItemID)
			if !strings.Contains(a.Reason, c.want) {
				t.Fatalf("reason = %q, want %q", a.Reason, c.want)
			}
			if runs, _ := f.db.WorkRunIDs(ctx, f.p.ID, acc.ResultWorkItemID); len(runs) != 0 {
				t.Fatal("work that did not fit was run")
			}
			if open, _ := f.db.AutoStarts(ctx, "", storage.AutoStartFilter{Status: storage.AutoReview}); len(open) != 1 {
				t.Fatalf("review list = %+v", open)
			}
		})
	}
}

// spend records usage over the budget on a judgment-free AI job.
func (f *autoFixture) spend(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	w, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, DepartmentID: f.dev.ID, Title: "예산 소진용"})
	job, _, err := f.db.CreateAIJob(ctx, storage.AIJob{ProjectID: f.p.ID, Kind: storage.JobPlan, Lane: storage.LaneInteractive, RequestKey: "plan:spend",
		WorkItemID: w.ID, AssignmentID: f.leads[f.dev.ID].ID, ConnectionID: f.conn.ID, Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := f.db.ClaimAIJob(ctx, job.ID); err != nil || !ok {
		t.Fatal(err)
	}
	job, _ = f.db.AIJob(ctx, f.p.ID, job.ID)
	att, err := f.db.StartAIAttempt(ctx, job)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.db.RecordAIUsage(ctx, att, json.RawMessage(`{"inputTokens":50,"outputTokens":50}`)); err != nil {
		t.Fatal(err)
	}
	f.db.FinishAIAttempt(ctx, att, "succeeded", "")
}

// An automatic start is approved for exactly the work and plan it checked:
// a change before the run starts voids the approval and sends the work to
// the person instead of starting it.
func TestChangeVoidsStartApproval(t *testing.T) {
	f := newAutoFixture(t, domain.ModeAuto)
	ctx := context.Background()
	f.rules(t, f.dev, domain.DepartmentAutomation{AutoStart: true})
	plan, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, DepartmentID: f.mkt.ID, Title: "기획"})
	r := f.request(t, plan, f.dev, storage.CollabReference)
	acc, _ := f.db.AcceptCollab(ctx, f.p.ID, r.ID, storage.CollabAcceptance{})
	f.db.SaveWorkPlan(ctx, domain.WorkPlan{ProjectID: f.p.ID, WorkItemID: acc.ResultWorkItemID, AssignmentID: f.leads[f.dev.ID].ID, Instructions: "검토"})
	if _, err := f.auto.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if a := f.autoStart(t, acc.ResultWorkItemID); a.Status != storage.AutoApproved {
		t.Fatalf("after checks = %+v", a)
	}
	w, _ := f.db.WorkItem(ctx, f.p.ID, acc.ResultWorkItemID)
	w.DoneCriteria = "범위가 바뀌었다"
	f.db.UpdateWorkItem(ctx, w)
	if ap, _ := f.db.WorkStartApproval(ctx, f.p.ID, w.ID); ap.Valid {
		t.Fatal("approval still valid after the done criteria changed")
	}
	f.auto.Drain(ctx)
	if a := f.autoStart(t, w.ID); a.Status != storage.AutoReview || !strings.Contains(a.Reason, "승인이 무효") {
		t.Fatalf("after the change = %+v", a)
	}
	if runs, _ := f.db.WorkRunIDs(ctx, f.p.ID, w.ID); len(runs) != 0 {
		t.Fatal("ran with a void approval")
	}
	// The person can check again once satisfied.
	a := f.autoStart(t, w.ID)
	if err := f.db.RetryAutoStart(ctx, f.p.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	f.settle(t, func() bool { return f.autoStart(t, w.ID).Status == storage.AutoFinished })
}

// The lead AI never decides required requests, a decline and a question
// are recorded as the AI's, an answer lets it decide once more, and the
// person can keep or revert what it decided.
func TestAICollabDecisions(t *testing.T) {
	f := newAutoFixture(t, domain.ModeAuto)
	ctx := context.Background()
	f.rules(t, f.dev, domain.DepartmentAutomation{AutoCollab: true})
	f.rules(t, f.fin, domain.DepartmentAutomation{AutoCollab: true})
	plan, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, DepartmentID: f.mkt.ID, Title: "기획"})
	answers := map[string][]string{
		"개발팀 협업 결정 AI": {`{"decision":"question","reason":"범위를 모른다","question":"어느 화면인가요?"}`,
			`{"decision":"accept","reason":"답을 보니 우리 일이다","title":"랜딩 화면 검토"}`},
		"재무팀 협업 결정 AI": {`{"decision":"decline","reason":"비용이 들지 않는 일이다"}`},
	}
	f.answer = func(what, _ string) (string, error) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(answers[what]) == 0 {
			return "", errors.New("unexpected call: " + what)
		}
		out := answers[what][0]
		answers[what] = answers[what][1:]
		return out, nil
	}
	required := f.request(t, plan, f.dev, storage.CollabRequired)
	toDev := f.request(t, plan, f.dev, storage.CollabReference)
	toFin := f.request(t, plan, f.fin, storage.CollabReference)
	f.settle(t, nil)

	if r, _ := f.db.CollabRequest(ctx, f.p.ID, required.ID); r.Status != storage.CollabRequested || r.DecidedBy != "" {
		t.Fatalf("AI decided a required request: %+v", r)
	}
	fin, _ := f.db.CollabRequest(ctx, f.p.ID, toFin.ID)
	if fin.Status != storage.CollabDeclined || fin.DecidedBy != "ai" || fin.DecisionReason != "비용이 들지 않는 일이다" {
		t.Fatalf("finance = %+v", fin)
	}
	dev, _ := f.db.CollabRequest(ctx, f.p.ID, toDev.ID)
	if dev.Status != storage.CollabQuestion || dev.FollowUp != "어느 화면인가요?" {
		t.Fatalf("development = %+v", dev)
	}
	if unreviewed, _ := f.db.CollabRequests(ctx, "", storage.CollabFilter{AIUnreviewed: true}); len(unreviewed) != 1 || unreviewed[0].ID != fin.ID {
		t.Fatalf("AI decisions to look at = %+v", unreviewed)
	}
	if _, err := f.db.ReviewCollabDecision(ctx, f.p.ID, fin.ID); err != nil {
		t.Fatal(err)
	}
	if unreviewed, _ := f.db.CollabRequests(ctx, "", storage.CollabFilter{AIUnreviewed: true}); len(unreviewed) != 0 {
		t.Fatalf("kept decision still listed: %+v", unreviewed)
	}

	if _, err := f.db.AnswerCollab(ctx, f.p.ID, dev.ID, "첫 화면 랜딩"); err != nil {
		t.Fatal(err)
	}
	f.settle(t, nil)
	dev, _ = f.db.CollabRequest(ctx, f.p.ID, toDev.ID)
	if dev.Status != storage.CollabAccepted || dev.ResultWorkItemID == "" || dev.ResultWorkTitle != "랜딩 화면 검토" {
		t.Fatalf("development after the answer = %+v", dev)
	}
	if _, err := f.db.SetCollabRequirement(ctx, f.p.ID, dev.ID, storage.CollabRequired, ""); err == nil {
		t.Fatal("an AI-decided request was made required")
	}
	if _, err := f.db.RevertCollabDecision(ctx, f.p.ID, dev.ID, ""); err == nil {
		t.Fatal("reverted without a reason")
	}
	back, err := f.db.RevertCollabDecision(ctx, f.p.ID, dev.ID, "이번에는 직접 정하겠다")
	if err != nil {
		t.Fatal(err)
	}
	cancelled, _ := f.db.WorkItem(ctx, f.p.ID, dev.ResultWorkItemID)
	if back.Status != storage.CollabRequested || back.ResultWorkItemID != "" || back.RevertReason == "" || cancelled.Status != domain.WorkCancelled {
		t.Fatalf("after revert: %+v, work %s", back, cancelled.Status)
	}
	if last := back.History[len(back.History)-1]; last.Action != "reverted" || last.Body != "이번에는 직접 정하겠다" {
		t.Fatalf("revert history = %+v", last)
	}
	// The person now decides; nothing is called again.
	if _, err := f.db.AcceptCollab(ctx, f.p.ID, dev.ID, storage.CollabAcceptance{Title: "사람이 정한 업무"}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 {
		t.Fatalf("AI calls = %v", f.calls)
	}
}

// A lead AI decision that keeps failing shows on the request; once the
// person decides, the failed delivery closes and a retry calls nothing.
func TestFailedAICollabDecisionGoesToThePerson(t *testing.T) {
	f := newAutoFixture(t, domain.ModeAuto)
	ctx := context.Background()
	f.rules(t, f.dev, domain.DepartmentAutomation{AutoCollab: true})
	plan, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, DepartmentID: f.mkt.ID, Title: "기획"})
	f.answer = func(string, string) (string, error) { return `{"decision":"accept"}`, nil } // no reason
	req := f.request(t, plan, f.dev, storage.CollabReference)
	f.settle(t, nil)

	r, _ := f.db.CollabRequest(ctx, f.p.ID, req.ID)
	if r.Status != storage.CollabRequested || r.AIStatus != domain.DeliveryFailed || !strings.Contains(r.AIProblem, "이유") {
		t.Fatalf("request after the failed decision = %+v", r)
	}
	calls := len(f.calls)
	r, err := f.db.AcceptCollab(ctx, f.p.ID, req.ID, storage.CollabAcceptance{Title: "사람이 정한 업무"})
	if err != nil {
		t.Fatal(err)
	}
	if r.AIStatus != "" || r.AIProblem != "" {
		t.Fatalf("decided request still shows the AI problem: %+v", r)
	}
	list, _ := f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{DepartmentID: f.dev.ID})
	if len(list) != 1 || list[0].Status != domain.DeliveryDone || list[0].Error != "" || !strings.Contains(list[0].Note, "사람이 결정") {
		t.Fatalf("delivery after the person decided = %+v", list)
	}

	// A delivery left failed by an older version closes on retry, without a call.
	if err := f.db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE event_deliveries SET status = 'failed', error = '협업 결정에 이유가 없습니다' WHERE id = ?`, list[0].ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.db.RetryDelivery(ctx, f.p.ID, list[0].ID); err != nil {
		t.Fatal(err)
	}
	f.settle(t, nil)
	list, _ = f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{DepartmentID: f.dev.ID})
	if list[0].Status != domain.DeliveryDone || len(f.calls) != calls {
		t.Fatalf("retry of a decided request: %+v, calls %d → %d", list[0], calls, len(f.calls))
	}
	if err := f.db.RetryDelivery(ctx, f.p.ID, list[0].ID); err == nil {
		t.Fatal("retried a finished delivery")
	}
}
