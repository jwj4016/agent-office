package work_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
	"agent-office/internal/work"
)

// judgeFixture is a service with marketing, development, finance and
// legal departments, each led by an AI. Development and finance follow
// shared documents; legal follows nothing.
type judgeFixture struct {
	db                 *storage.DB
	eng                *engine.Engine
	ai                 *work.AI
	proc               *work.Processor
	p                  domain.Project
	mkt, dev, fin, leg domain.Department
	mu                 sync.Mutex
	calls              []string // department of each judgment call
	answer             func(dept, prompt string) (string, error)
}

func newJudgeFixture(t *testing.T) *judgeFixture {
	t.Helper()
	ctx := context.Background()
	f := &judgeFixture{}
	f.db = open(t, filepath.Join(t.TempDir(), "judge.db"))
	t.Cleanup(func() { f.db.Close() })
	f.p, _ = f.db.CreateProject(ctx, domain.Project{Name: "캠페인"})
	conn, _ := f.db.SaveConnection(ctx, domain.ProviderConnection{Name: "test", Provider: "test"})
	dept := func(name string) domain.Department {
		role, _ := f.db.SaveRole(ctx, domain.Role{Name: name + " 담당"})
		a, _ := f.db.SaveAssignment(ctx, domain.Assignment{ProjectID: f.p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
			DisplayName: name + " AI", ConnectionID: conn.ID, Model: "m"})
		d, err := f.db.SaveDepartment(ctx, domain.Department{ProjectID: f.p.ID, Name: name, Responsibility: name + "의 책임",
			MemberIDs: []string{a.ID}, LeadAssignmentID: a.ID})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	f.mkt, f.dev, f.fin, f.leg = dept("마케팅팀"), dept("개발팀"), dept("재무팀"), dept("법무팀")
	for _, d := range []domain.Department{f.dev, f.fin} {
		if _, err := f.db.SaveSubscription(ctx, domain.Subscription{ProjectID: f.p.ID, DepartmentID: d.ID,
			EventKinds: []string{domain.EventDocumentShared, domain.EventDocumentRevised}, Guidance: d.Name + "이 할 일이 있는지 본다", Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	f.eng = engine.New(engine.Config{DB: f.db, DataDir: t.TempDir()})
	f.ai = work.NewAI(work.AIConfig{
		DB:        f.db,
		Providers: func(domain.ProviderConnection) (providers.Provider, error) { return nil, nil },
		Hold: func(ctx context.Context, projectID string) (string, error) {
			u, err := f.eng.ProjectUsage(ctx, projectID)
			return u.HoldReason, err
		},
		Kinds: map[string]work.Kind{storage.JobJudgment: work.JudgeKind{DB: f.db,
			ReadDocument: func(context.Context, string, string) (string, error) {
				return "캠페인 페이지와 광고 예산을 다룬다", nil
			}}},
		Call: func(ctx context.Context, _ providers.Provider, c design.JSONCall) ([]byte, error) {
			dept := strings.TrimSuffix(c.What, " 판단 AI")
			f.mu.Lock()
			f.calls = append(f.calls, dept)
			f.mu.Unlock()
			c.OnEvent(providers.Event{Kind: providers.KindUsage, Payload: json.RawMessage(`{"inputTokens":30,"outputTokens":10,"costUsd":0.002}`)})
			out, err := f.answer(dept, c.Prompt)
			return []byte(out), err
		},
	})
	f.proc = work.New(f.db, work.Judge(f.db, nil))
	return f
}

func propose(titles ...string) string {
	var props []map[string]any
	for _, t := range titles {
		props = append(props, map[string]any{"title": t, "doneCriteria": t + " 완료", "categories": []string{"개발", "없는분류"}, "priority": "high"})
	}
	b, _ := json.Marshal(map[string]any{"outcome": "propose", "reason": "기획안에 해당 내용이 있다", "evidence": []string{"기획안"}, "proposals": props})
	return string(b)
}

const none = `{"outcome":"none","reason":"이 부서가 할 일이 없다"}`

// settle runs deliveries and both AI lanes until nothing is left.
func (f *judgeFixture) settle(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	for i := 0; i < 10; i++ {
		n1, err1 := f.proc.Drain(ctx)
		n2, err2 := f.ai.Drain(ctx, storage.LaneBackground)
		n3, err3 := f.ai.Drain(ctx, storage.LaneInteractive)
		if err := errors.Join(err1, err2, err3); err != nil {
			t.Fatal(err)
		}
		if n1+n2+n3 == 0 {
			return
		}
	}
}

func (f *judgeFixture) share(t *testing.T, title, hash string) storage.DocumentVersion {
	t.Helper()
	_, v, err := f.db.AddDocumentVersion(context.Background(), f.p.ID, storage.NewDocumentVersion{Title: title, OwnerDepartmentID: f.mkt.ID,
		Categories: []string{"기획"}, SourceKind: "workspace", SourcePath: "plan.md", FileName: "plan.md", Path: "p/" + hash, Hash: hash, Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func proposalTitles(t *testing.T, f *judgeFixture, status string) []string {
	t.Helper()
	list, err := f.db.Proposals(context.Background(), f.p.ID, status)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, p := range list {
		out = append(out, p.DepartmentName+":"+p.Title)
	}
	slices.Sort(out)
	return out
}

// Stage 5 acceptance: one shared plan makes development and finance each
// propose work; legal, which follows nothing, is never asked. A plan
// that needs only development leaves finance with nothing, and the
// judgments are paid from the service budget.
func TestDepartmentsProposeFromASharedPlan(t *testing.T) {
	f := newJudgeFixture(t)
	f.answer = func(dept, prompt string) (string, error) {
		switch {
		case dept == "개발팀" && strings.Contains(prompt, "캠페인 기획안"):
			return propose("캠페인 페이지 구현 검토"), nil
		case dept == "개발팀":
			return propose("앱 화면 수정"), nil
		case dept == "재무팀" && strings.Contains(prompt, "캠페인 기획안"):
			return propose("캠페인 예산 검토"), nil
		}
		return none, nil
	}
	f.share(t, "캠페인 기획안", "h1")
	f.settle(t)
	if got := proposalTitles(t, f, "proposed"); !slices.Equal(got, []string{"개발팀:캠페인 페이지 구현 검토", "재무팀:캠페인 예산 검토"}) {
		t.Fatalf("proposals = %v", got)
	}
	slices.Sort(f.calls)
	if !slices.Equal(f.calls, []string{"개발팀", "재무팀"}) {
		t.Fatalf("judged by %v", f.calls)
	}
	// A plan that only needs development: finance judges and proposes nothing.
	f.share(t, "앱 개선안", "h2")
	f.settle(t)
	if got := proposalTitles(t, f, "proposed"); len(got) != 3 || !slices.Contains(got, "개발팀:앱 화면 수정") {
		t.Fatalf("proposals = %v", got)
	}
	all, _ := f.db.Deliveries(context.Background(), f.p.ID, storage.DeliveryFilter{})
	for _, d := range all {
		if d.Status != domain.DeliveryDone {
			t.Fatalf("delivery left %s: %+v", d.Status, d)
		}
		if d.DepartmentID == f.fin.ID && strings.Contains(d.Event.Title, "앱 개선안") && !strings.HasPrefix(d.Note, "대응 불필요") {
			t.Fatalf("finance note = %q", d.Note)
		}
	}
	if u, _ := f.eng.ProjectUsage(context.Background(), f.p.ID); u.Attempts != 4 || u.InputTokens != 120 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestAcceptingAProposalRegistersWorkOnce(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	goal, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, Kind: domain.WorkGoal, Title: "캠페인", DepartmentID: f.mkt.ID})
	plan, _ := f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, ParentID: goal.ID, DepartmentID: f.mkt.ID, Title: "기획안 작성"})
	f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, DepartmentID: f.dev.ID, Title: "캠페인 페이지 구현 검토"}) // already there
	f.answer = func(dept, prompt string) (string, error) {
		if dept == "개발팀" {
			return propose("캠페인 페이지 구현 검토", "추적 코드 추가"), nil
		}
		return none, nil
	}
	_, _, err := f.db.AddDocumentVersion(ctx, f.p.ID, storage.NewDocumentVersion{Title: "기획안", WorkItemID: plan.ID, SourceKind: "workspace",
		SourcePath: "a", FileName: "a", Path: "a", Hash: "h", Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	props, _ := f.db.Proposals(ctx, f.p.ID, "proposed")
	if len(props) != 2 || props[0].DuplicateOf == "" || props[1].DuplicateOf != "" {
		t.Fatalf("duplicate hints = %+v", props)
	}
	if props[0].Categories[0] != "개발" || len(props[0].Categories) != 1 {
		t.Fatalf("categories not filtered: %v", props[0].Categories)
	}
	w, err := f.db.AcceptProposal(ctx, f.p.ID, props[1].ID, storage.ProposalEdit{Title: "추적 코드 추가(GA4)"})
	if err != nil {
		t.Fatal(err)
	}
	if w.DepartmentID != f.dev.ID || w.ParentID != goal.ID || w.Title != "추적 코드 추가(GA4)" || w.Priority != "high" {
		t.Fatalf("work = %+v", w)
	}
	evs, _ := f.db.WorkEvents(ctx, f.p.ID, w.ID)
	if len(evs) != 1 || evs[0].Event.CauseSequence == nil || evs[0].Event.Automated {
		t.Fatalf("work.created = %+v", evs)
	}
	if _, err := f.db.AcceptProposal(ctx, f.p.ID, props[1].ID, storage.ProposalEdit{}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("accepted twice: %v", err)
	}
	if err := f.db.RejectProposal(ctx, f.p.ID, props[0].ID, "이미 있는 업무"); err != nil {
		t.Fatal(err)
	}
	if got := proposalTitles(t, f, "proposed"); len(got) != 0 {
		t.Fatalf("still proposed: %v", got)
	}
}

func TestQuestionThenAnswerThenDecide(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	f.answer = func(dept, prompt string) (string, error) {
		if strings.Contains(prompt, "앞서 한 질문") {
			return propose("예산 한도 확인"), nil
		}
		return `{"outcome":"question","reason":"예산 범위를 모른다","question":"이번 캠페인 예산 한도는 얼마인가요?"}`, nil
	}
	if _, err := f.db.RequestDepartmentCheck(ctx, f.p.ID, f.fin.ID, "이번 달 할 일을 찾아 주세요"); err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	qs, _ := f.db.Judgments(ctx, f.p.ID, "open")
	if len(qs) != 1 || qs[0].Round != 1 || qs[0].Question == "" {
		t.Fatalf("questions = %+v", qs)
	}
	d, _ := f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{})
	if d[0].Status != domain.DeliveryWaitingAnswer {
		t.Fatalf("delivery = %s", d[0].Status)
	}
	job, err := f.db.AnswerJudgment(ctx, f.p.ID, qs[0].ID, "500만 원")
	if err != nil || job.Round != 2 || job.Lane != storage.LaneInteractive {
		t.Fatalf("round 2 = %+v %v", job, err)
	}
	if _, err := f.db.AnswerJudgment(ctx, f.p.ID, qs[0].ID, "다시"); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("answered twice: %v", err)
	}
	f.settle(t)
	if got := proposalTitles(t, f, "proposed"); !slices.Equal(got, []string{"재무팀:예산 한도 확인"}) {
		t.Fatalf("after answer = %v", got)
	}
	if d, _ = f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{}); d[0].Status != domain.DeliveryDone {
		t.Fatalf("delivery after round 2 = %s", d[0].Status)
	}
}

func TestSecondQuestionGoesToAPerson(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	f.answer = func(string, string) (string, error) {
		return `{"outcome":"question","reason":"정보 부족","question":"무엇을 우선할까요?"}`, nil
	}
	f.db.RequestDepartmentCheck(ctx, f.p.ID, f.dev.ID, "")
	f.settle(t)
	qs, _ := f.db.Judgments(ctx, f.p.ID, "open")
	f.db.AnswerJudgment(ctx, f.p.ID, qs[0].ID, "속도")
	f.settle(t)
	manual, _ := f.db.Judgments(ctx, f.p.ID, "manual")
	if len(manual) != 1 || manual[0].Round != 2 {
		t.Fatalf("manual = %+v", manual)
	}
	if err := f.db.CloseJudgmentQuestion(ctx, f.p.ID, manual[0].ID, "직접 업무를 만들었다"); err != nil {
		t.Fatal(err)
	}
	if left, _ := f.db.Judgments(ctx, f.p.ID, "open,manual"); len(left) != 0 {
		t.Fatalf("left = %+v", left)
	}
	if n := len(f.calls); n != 2 {
		t.Fatalf("calls = %d", n)
	}
}

func TestFailedJudgmentIsRetriedFromTheInbox(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	fail := true
	f.answer = func(string, string) (string, error) {
		if fail {
			return `{"outcome":"maybe"}`, nil // malformed: a failed attempt
		}
		return none, nil
	}
	f.db.RequestDepartmentCheck(ctx, f.p.ID, f.dev.ID, "")
	f.settle(t)
	d, _ := f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{})
	if d[0].Status != domain.DeliveryFailed || !strings.Contains(d[0].Error, "이유") || len(f.calls) != 3 {
		t.Fatalf("after failures: %+v calls=%d", d[0], len(f.calls))
	}
	fail = false
	if err := f.db.RetryDelivery(ctx, f.p.ID, d[0].ID); err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	d, _ = f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{})
	u, _ := f.eng.ProjectUsage(ctx, f.p.ID)
	if d[0].Status != domain.DeliveryDone || len(f.calls) != 4 || u.Attempts != 4 {
		t.Fatalf("after retry: %+v calls=%d usage=%+v", d[0], len(f.calls), u)
	}
}

func TestBudgetHoldsJudgment(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	p := f.p
	p.Budget = json.RawMessage(`{"maxTokens":30}`)
	f.db.UpdateProject(ctx, p)
	f.answer = func(string, string) (string, error) { return none, nil }
	f.db.RequestDepartmentCheck(ctx, f.p.ID, f.dev.ID, "")
	f.settle(t) // 40 tokens: over budget afterwards
	f.db.RequestDepartmentCheck(ctx, f.p.ID, f.fin.ID, "")
	f.settle(t)
	d, _ := f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{DepartmentID: f.fin.ID})
	if d[0].Status != domain.DeliveryHeld || d[0].HoldReason == "" || len(f.calls) != 1 {
		t.Fatalf("finance delivery = %+v calls=%d", d[0], len(f.calls))
	}
}

func TestChecksAreInteractiveAndSubscriptionsBackground(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	f.answer = func(string, string) (string, error) { return none, nil }
	f.db.RequestDepartmentCheck(ctx, f.p.ID, f.leg.ID, "")
	f.share(t, "기획안", "h1")
	if _, err := f.proc.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if n, _ := f.ai.Drain(ctx, storage.LaneInteractive); n != 1 {
		t.Fatalf("interactive jobs = %d", n)
	}
	if n, _ := f.ai.Drain(ctx, storage.LaneBackground); n != 2 {
		t.Fatalf("background jobs = %d", n)
	}
}

func TestReviewExistingDocuments(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	f.answer = func(dept, prompt string) (string, error) {
		if strings.Contains(prompt, "기존 자료 검토") && strings.Contains(prompt, "광고 예산") {
			return propose("과거 계약 검토"), nil
		}
		return none, nil
	}
	v := f.share(t, "작년 계약서", "old")
	f.settle(t) // development and finance judged the share
	n, err := f.db.RequestDocumentReview(ctx, f.p.ID, f.leg.ID, []string{v.ID}, "새로 만든 부서입니다")
	if err != nil || n != 1 {
		t.Fatalf("review = %d %v", n, err)
	}
	f.settle(t)
	if got := proposalTitles(t, f, "proposed"); !slices.Contains(got, "법무팀:과거 계약 검토") {
		t.Fatalf("proposals = %v", got)
	}
}

func TestDeepAutomaticChainsAreHeld(t *testing.T) {
	f := newJudgeFixture(t)
	res, err := work.Judge(f.db, nil)(context.Background(), storage.Delivery{Purpose: domain.PurposeJudge,
		Event: storage.WorkEvent{Automated: true, ChainDepth: domain.DefaultChainDepth}})
	if err != nil || res.Status != domain.DeliveryHeld || res.Note == "" {
		t.Fatalf("result = %+v %v", res, err)
	}
}

// A stop during the call ends the attempt and the judgment is asked
// again; a stop after the result was stored never calls again.
func TestJudgmentSurvivesRestarts(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	stop, cancel := context.WithCancel(ctx)
	f.answer = func(string, string) (string, error) {
		cancel() // the app stops while the provider answers
		return none, nil
	}
	f.db.RequestDepartmentCheck(ctx, f.p.ID, f.dev.ID, "")
	if _, err := f.proc.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.ai.Drain(stop, storage.LaneInteractive); !errors.Is(err, context.Canceled) {
		t.Fatalf("stopped drain: %v", err)
	}
	// Next start: deliveries and jobs are recovered, the job is reused.
	if _, err := f.db.RecoverDeliveries(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.db.RecoverAIJobs(ctx); err != nil {
		t.Fatal(err)
	}
	f.answer = func(string, string) (string, error) { return none, nil }
	f.settle(t)
	jobs := func() (n int) {
		f.db.Read().QueryRow(`SELECT COUNT(*) FROM work_ai_jobs WHERE kind = 'judgment'`).Scan(&n)
		return n
	}
	if len(f.calls) != 2 || jobs() != 1 {
		t.Fatalf("calls=%d jobs=%d", len(f.calls), jobs())
	}
	// Stopping after the result was stored: the delivery is done with
	// the job, so a recovery finds nothing to call.
	f.db.RecoverDeliveries(ctx)
	f.db.RecoverAIJobs(ctx)
	f.settle(t)
	d, _ := f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{})
	if len(f.calls) != 2 || jobs() != 1 || d[0].Status != domain.DeliveryDone {
		t.Fatalf("after second restart: calls=%d jobs=%d delivery=%s", len(f.calls), jobs(), d[0].Status)
	}
	if u, _ := f.eng.ProjectUsage(ctx, f.p.ID); u.Attempts != 2 {
		t.Fatalf("usage attempts = %d", u.Attempts)
	}
}

// A delivery handed back to the queue while its job is already ready
// (for example after a crash between the two) finishes without a call.
func TestReadyJudgmentIsNotCalledAgain(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	f.answer = func(string, string) (string, error) { return propose("페이지 검토"), nil }
	f.db.RequestDepartmentCheck(ctx, f.p.ID, f.dev.ID, "")
	f.settle(t)
	f.db.Read().Exec(`UPDATE event_deliveries SET status = 'pending'`)
	f.settle(t)
	d, _ := f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{})
	if len(f.calls) != 1 || d[0].Status != domain.DeliveryDone {
		t.Fatalf("calls=%d delivery=%+v", len(f.calls), d[0])
	}
	if got := proposalTitles(t, f, "proposed"); len(got) != 1 {
		t.Fatalf("proposals = %v", got)
	}
}
