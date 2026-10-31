package work_test

import (
	"context"
	"encoding/json"
	"errors"
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

// intakeFixture is a company with two services, 빌라노트 (a development
// department) and 블로그 (no departments). Desks answer through answer,
// keyed by the desk's name.
type intakeFixture struct {
	db          *storage.DB
	eng         *engine.Engine
	in          *work.Intake
	villa, blog domain.Project
	dev         domain.Department
	villaAI     domain.Assignment
	blogAI      domain.Assignment
	conn        domain.ProviderConnection
	mu          sync.Mutex
	calls       []string // desk of each call
	prompts     []string
	answer      func(desk, prompt string) (string, error)
}

func newIntakeFixture(t *testing.T) *intakeFixture {
	t.Helper()
	ctx := context.Background()
	f := &intakeFixture{}
	f.db = open(t, filepath.Join(t.TempDir(), "intake.db"))
	t.Cleanup(func() { f.db.Close() })
	f.conn, _ = f.db.SaveConnection(ctx, domain.ProviderConnection{Name: "test", Provider: "test"})
	f.villa, _ = f.db.CreateProject(ctx, domain.Project{Name: "빌라노트", Goal: "빌라 물건을 분석한다"})
	f.blog, _ = f.db.CreateProject(ctx, domain.Project{Name: "블로그", Goal: "회사 블로그를 운영한다"})
	role, _ := f.db.SaveRole(ctx, domain.Role{Name: "기획"})
	ai := func(p domain.Project, name string) domain.Assignment {
		a, err := f.db.SaveAssignment(ctx, domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
			DisplayName: name, ConnectionID: f.conn.ID, Model: "m"})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	f.villaAI, f.blogAI = ai(f.villa, "빌라 접수"), ai(f.blog, "블로그 접수")
	var err error
	if f.dev, err = f.db.SaveDepartment(ctx, domain.Department{ProjectID: f.villa.ID, Name: "개발팀", Responsibility: "코드를 만든다",
		MemberIDs: []string{f.villaAI.ID}, LeadAssignmentID: f.villaAI.ID}); err != nil {
		t.Fatal(err)
	}
	f.eng = engine.New(engine.Config{DB: f.db, DataDir: t.TempDir()})
	f.in = work.NewIntake(work.IntakeConfig{
		DB: f.db, Engine: f.eng,
		Providers: func(domain.ProviderConnection) (providers.Provider, error) { return nil, nil },
		Hold: func(ctx context.Context, projectID string) (string, error) {
			u, err := f.eng.ProjectUsage(ctx, projectID)
			return u.HoldReason, err
		},
		Call: func(ctx context.Context, _ providers.Provider, c design.JSONCall) ([]byte, error) {
			desk := strings.TrimSuffix(c.What, " 접수 AI")
			f.mu.Lock()
			f.calls = append(f.calls, desk)
			f.prompts = append(f.prompts, c.Prompt)
			f.mu.Unlock()
			c.OnEvent(providers.Event{Kind: providers.KindUsage, Payload: json.RawMessage(`{"inputTokens":40,"outputTokens":10,"costUsd":0.01}`)})
			out, err := f.answer(desk, c.Prompt)
			return []byte(out), err
		},
	})
	return f
}

func (f *intakeFixture) desks(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.db.SaveIntakeDesk(ctx, domain.IntakeDesk{ConnectionID: f.conn.ID, Model: "m", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	for _, d := range []domain.IntakeDesk{{ProjectID: f.villa.ID, AssignmentID: f.villaAI.ID, Enabled: true},
		{ProjectID: f.blog.ID, AssignmentID: f.blogAI.ID, Enabled: true}} {
		if _, err := f.db.SaveIntakeDesk(ctx, d); err != nil {
			t.Fatal(err)
		}
	}
}

func (f *intakeFixture) drain(t *testing.T) {
	t.Helper()
	if _, err := f.in.Drain(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func (f *intakeFixture) submit(t *testing.T, projectID, body string) domain.Request {
	t.Helper()
	r, err := f.db.SubmitRequest(context.Background(), domain.Request{ProjectID: projectID, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func (f *intakeFixture) get(t *testing.T, projectID, id string) domain.Request {
	t.Helper()
	r, err := f.db.Request(context.Background(), projectID, id)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func asJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// A service desk answers, asks back, proposes work and declines; the
// person's answer starts round 2, and accepted work names its request.
func TestServiceDeskDecides(t *testing.T) {
	f := newIntakeFixture(t)
	f.desks(t)
	ctx := context.Background()
	f.answer = func(desk, prompt string) (string, error) {
		switch {
		case strings.HasSuffix(desk, "보고 AI"):
			if !strings.Contains(prompt, "역전세 위험 지표 추가") || !strings.Contains(prompt, "완료") {
				return "", errors.New("the report prompt lacks the work")
			}
			return `{"summary":"역전세 지표를 추가했습니다.","details":"- 역전세 위험 지표 추가: 완료"}`, nil
		case strings.Contains(prompt, "목표가 뭐야"):
			return `{"outcome":"answer","reason":"서비스 정보","answer":"빌라 물건을 분석합니다"}`, nil
		case strings.Contains(prompt, "앞서 한 질문과 사람의 답"):
			return asJSON(map[string]any{"outcome": "work", "reason": "기준 보완", "work": []map[string]any{
				{"title": "역전세 지표 추가", "doneCriteria": "지표가 리포트에 나온다", "departmentId": f.dev.ID, "priority": "high"}}}), nil
		case strings.Contains(prompt, "기준 고쳐"):
			return `{"outcome":"question","reason":"범위가 필요","question":"어느 기준을 고칠까요?"}`, nil
		default:
			return `{"outcome":"decline","reason":"이 앱으로 송금할 수 없습니다. 은행 앱을 쓰세요"}`, nil
		}
	}
	answered := f.submit(t, f.villa.ID, "빌라노트 목표가 뭐야?")
	asked := f.submit(t, f.villa.ID, "검토에서 나온 기준 고쳐 줘")
	declined := f.submit(t, f.villa.ID, "계약금을 송금해 줘")
	f.drain(t)

	if r := f.get(t, f.villa.ID, answered.ID); r.Status != domain.ReqDone || r.Answer != "빌라 물건을 분석합니다" || r.ClosedAt == "" {
		t.Fatalf("answered = %+v", r)
	}
	if r := f.get(t, f.villa.ID, declined.ID); r.Status != domain.ReqDeclined || !strings.Contains(r.Reason, "은행 앱") {
		t.Fatalf("declined = %+v", r)
	}
	r := f.get(t, f.villa.ID, asked.ID)
	if r.Status != domain.ReqQuestion || r.Question != "어느 기준을 고칠까요?" {
		t.Fatalf("asked = %+v", r)
	}
	if !strings.Contains(f.prompts[1], f.dev.ID) || !strings.Contains(f.prompts[1], "개발팀") {
		t.Fatal("the desk was not told about the departments")
	}
	if err := f.db.AnswerRequest(ctx, f.villa.ID, asked.ID, "역전세 위험 지표"); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	r = f.get(t, f.villa.ID, asked.ID)
	if r.Status != domain.ReqProposed || r.Round != 2 || len(r.Items) != 1 || r.Items[0].Status != domain.ItemProposed {
		t.Fatalf("after the answer = %+v", r)
	}
	w, err := f.db.AcceptRequestWork(ctx, f.villa.ID, r.Items[0].ID, domain.WorkProposalPayload{Title: "역전세 위험 지표 추가"})
	if err != nil {
		t.Fatal(err)
	}
	if w.Title != "역전세 위험 지표 추가" || w.DepartmentID != f.dev.ID || w.Priority != "high" || w.OriginRequestID != asked.ID {
		t.Fatalf("work = %+v", w)
	}
	if _, err := f.db.AcceptRequestWork(ctx, f.villa.ID, r.Items[0].ID, domain.WorkProposalPayload{}); err == nil {
		t.Fatal("the same proposal was registered twice")
	}
	// The request follows its work: in progress until the work is done,
	// then the desk's completion report ends it and waits to be read.
	if r := f.get(t, f.villa.ID, asked.ID); r.Status != domain.ReqInProgress || r.Items[0].WorkTitle != w.Title {
		t.Fatalf("after accepting = %+v", r)
	}
	if _, err := f.db.SetWorkStatus(ctx, f.villa.ID, w.ID, domain.WorkDone, ""); err != nil {
		t.Fatal(err)
	}
	if r := f.get(t, f.villa.ID, asked.ID); r.Status != domain.ReqInProgress || r.Report != "" {
		t.Fatalf("before the report = %+v", r)
	}
	f.drain(t)
	r = f.get(t, f.villa.ID, asked.ID)
	if r.Status != domain.ReqDone || !strings.HasPrefix(r.Report, "## 요약\n역전세 지표를 추가했습니다.") || r.ReportReadAt != "" {
		t.Fatalf("after the report = %+v", r)
	}
	todos, _ := f.db.RequestTodos(ctx)
	if len(todos) != 1 || todos[0].ID != asked.ID {
		t.Fatalf("unread report to-dos = %+v", todos)
	}
	if err := f.db.MarkReportRead(ctx, f.villa.ID, asked.ID); err != nil {
		t.Fatal(err)
	}
	if todos, _ := f.db.RequestTodos(ctx); len(todos) != 0 {
		t.Fatalf("read report still a to-do: %+v", todos)
	}
	l, err := f.db.WorkLineage(ctx, f.villa.ID, w.ID)
	if err != nil || len(l.Causes) != 1 || l.Causes[0].Via != "request" || l.Causes[0].RequestBody != "검토에서 나온 기준 고쳐 줘" || l.Causes[0].DecidedBy != "user" {
		t.Fatalf("lineage = %+v %v", l, err)
	}
	// Every call is paid from the service budget; requests are not work
	// events, so no department got a delivery.
	if u, _ := f.eng.ProjectUsage(ctx, f.villa.ID); u.Attempts != 5 || u.InputTokens != 200 {
		t.Fatalf("usage = %+v", u)
	}
	if ds, _ := f.db.Deliveries(ctx, f.villa.ID, storage.DeliveryFilter{}); len(ds) != 0 {
		t.Fatalf("request events reached departments: %+v", ds)
	}
	// A request of one service is not another's.
	if _, err := f.db.Request(ctx, f.blog.ID, asked.ID); err == nil {
		t.Fatal("read another service's request")
	}
	if _, err := f.db.Request(ctx, "", asked.ID); err == nil {
		t.Fatal("read a service request as a company request")
	}
}

// The company desk passes a request to the services it concerns; the
// company request is done once both service requests are. A request
// sent to a chosen service skips the company desk.
func TestCompanyDeskRoutesToServices(t *testing.T) {
	f := newIntakeFixture(t)
	f.desks(t)
	f.answer = func(desk, prompt string) (string, error) {
		if desk == "비서실" {
			if !strings.Contains(prompt, f.villa.ID) || !strings.Contains(prompt, "블로그") {
				return "", errors.New("services missing from the company prompt")
			}
			return asJSON(map[string]any{"outcome": "route", "reason": "두 서비스의 일", "routes": []map[string]any{
				{"projectId": f.villa.ID, "body": "빌라노트 소개 자료를 정리해 줘"},
				{"projectId": f.blog.ID, "body": "빌라노트 소개 글을 써 줘"},
				{"projectId": f.blog.ID, "body": "중복"}}}), nil
		}
		return `{"outcome":"answer","reason":"바로 답함","answer":"알겠습니다"}`, nil
	}
	r := f.submit(t, "", "빌라노트를 블로그에 소개해 줘")
	f.drain(t)
	r = f.get(t, "", r.ID)
	if r.Status != domain.ReqDone || len(r.Children) != 2 || len(r.Items) != 2 {
		t.Fatalf("company request = %+v", r)
	}
	if r.Children[0].ProjectID != f.villa.ID || r.Children[0].Body != "빌라노트 소개 자료를 정리해 줘" || r.Children[1].Status != domain.ReqDone {
		t.Fatalf("children = %+v", r.Children)
	}
	if len(f.calls) != 3 || f.calls[0] != "비서실" {
		t.Fatalf("calls = %v", f.calls)
	}
	// Choosing the service skips the company desk.
	f.calls = nil
	f.submit(t, f.blog.ID, "새 글 주제 알려 줘")
	f.drain(t)
	if len(f.calls) != 1 || f.calls[0] != "블로그 접수" {
		t.Fatalf("direct request calls = %v", f.calls)
	}
}

// Without a desk a request waits and goes ahead once one is set up; a
// used-up budget holds it; the company desk has a monthly budget.
func TestRequestsWaitForDeskAndBudget(t *testing.T) {
	f := newIntakeFixture(t)
	ctx := context.Background()
	f.answer = func(string, string) (string, error) { return `{"outcome":"answer","reason":"r","answer":"네"}`, nil }
	r := f.submit(t, f.villa.ID, "안녕")
	c := f.submit(t, "", "회사 소개해 줘")
	f.drain(t)
	// The AI could not staff a desk (it answered something else): after two
	// tries per request they wait for the person, and no desk decided.
	if got := f.get(t, f.villa.ID, r.ID); got.Status != domain.ReqWaitingDesk || !strings.Contains(got.HoldReason, "부서·담당자") ||
		!strings.Contains(got.HoldReason, "AI가 접수 담당을 배치하지 못했습니다") {
		t.Fatalf("no desk = %+v", got)
	}
	if got := f.get(t, "", c.ID); got.Status != domain.ReqWaitingDesk {
		t.Fatalf("no company desk = %+v", got)
	}
	if len(f.calls) != 4 || f.calls[0] != "접수 담당 배치 AI" || f.calls[3] != "접수 담당 배치 AI" {
		t.Fatalf("calls without a desk = %v", f.calls)
	}
	f.drain(t)
	if len(f.calls) != 4 {
		t.Fatal("staffing was tried again")
	}
	// A desk that is a person is refused.
	human, _ := f.db.SaveAssignment(ctx, domain.Assignment{ProjectID: f.villa.ID, RoleID: f.villaAI.RoleID, ActorKind: domain.ActorHuman,
		ActorID: domain.LocalOwner, DisplayName: "나"})
	if _, err := f.db.SaveIntakeDesk(ctx, domain.IntakeDesk{ProjectID: f.villa.ID, AssignmentID: human.ID, Enabled: true}); err == nil {
		t.Fatal("a person became a desk")
	}
	f.desks(t)
	f.drain(t)
	if got := f.get(t, f.villa.ID, r.ID); got.Status != domain.ReqDone {
		t.Fatalf("after the desk = %+v", got)
	}

	limit := 0.01
	p := f.villa
	p.Budget = json.RawMessage(`{"maxCostUsd":0.01}`)
	if _, err := f.db.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	held := f.submit(t, f.villa.ID, "또 안녕")
	f.drain(t)
	if got := f.get(t, f.villa.ID, held.ID); got.Status != domain.ReqHeld || !strings.Contains(got.HoldReason, "예산") {
		t.Fatalf("over budget = %+v", got)
	}
	if _, err := f.db.SaveIntakeDesk(ctx, domain.IntakeDesk{ConnectionID: f.conn.ID, Model: "m", Enabled: true,
		Budget: domain.DeskBudget{MonthlyCostUSD: &limit}}); err != nil {
		t.Fatal(err)
	}
	c2 := f.submit(t, "", "회사 소개 한 번 더")
	f.drain(t)
	if got := f.get(t, "", c2.ID); got.Status != domain.ReqHeld || !strings.Contains(got.HoldReason, "이번 달 예산") {
		t.Fatalf("company over budget = %+v", got)
	}
	// Two failed staffing calls and one decision, all counted.
	if u, _ := f.db.CompanyIntakeUsage(ctx, testMonthStart()); u.Attempts != 3 {
		t.Fatalf("company usage = %+v", u)
	}
}

// A decision naming something the desk was not shown fails the attempt;
// it is retried twice, then the request fails until the person retries.
func TestBadDecisionsAreRetriedThenFail(t *testing.T) {
	f := newIntakeFixture(t)
	f.desks(t)
	ctx := context.Background()
	f.answer = func(string, string) (string, error) {
		return `{"outcome":"work","reason":"r","work":[{"title":"t","departmentId":"dep-없음"}]}`, nil
	}
	r := f.submit(t, f.villa.ID, "업무 만들어 줘")
	for i := 0; i < 3; i++ {
		f.drain(t)
	}
	got := f.get(t, f.villa.ID, r.ID)
	if got.Status != domain.ReqFailed || !strings.Contains(got.Error, "목록에 없는 부서") || got.Attempts != 3 {
		t.Fatalf("bad decision = %+v", got)
	}
	f.answer = func(string, string) (string, error) {
		return `{"outcome":"answer","reason":"r","answer":"됐습니다"}`, nil
	}
	if err := f.db.RetryRequest(ctx, f.villa.ID, r.ID); err != nil {
		t.Fatal(err)
	}
	f.drain(t)
	if got := f.get(t, f.villa.ID, r.ID); got.Status != domain.ReqDone || got.Attempts != 4 {
		t.Fatalf("after retry = %+v", got)
	}
}

// In auto mode the desk's work is registered at once; actions still wait
// for the person, run once, and an action cut off by a stop is left for
// the person to check.
func TestAutoWorkAndActions(t *testing.T) {
	f := newIntakeFixture(t)
	f.desks(t)
	ctx := context.Background()
	p := f.villa
	p.Mode = domain.ModeAuto
	if _, err := f.db.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	f.answer = func(string, string) (string, error) {
		return asJSON(map[string]any{"outcome": "action", "reason": "내보낸 뒤 정리", "actions": []map[string]any{{"action": "export_service", "reason": "백업"}},
			"work": []map[string]any{{"title": "내보낸 자료 정리", "departmentId": f.dev.ID}}}), nil
	}
	r := f.submit(t, f.villa.ID, "서비스 백업하고 정리해 줘")
	f.drain(t)
	r = f.get(t, f.villa.ID, r.ID)
	if r.Status != domain.ReqProposed || len(r.Items) != 2 {
		t.Fatalf("decision = %+v", r)
	}
	action, workItem := r.Items[0], r.Items[1]
	if action.Kind != domain.ItemAction || action.Status != domain.ItemProposed || workItem.Status != domain.ItemAccepted || workItem.DecidedBy != "auto" || workItem.WorkItemID == "" {
		t.Fatalf("items = %+v", r.Items)
	}
	w, _ := f.db.WorkItem(ctx, f.villa.ID, workItem.WorkItemID)
	if w.OriginRequestID != r.ID {
		t.Fatalf("auto work = %+v", w)
	}
	if _, err := f.db.StartRequestAction(ctx, f.villa.ID, action.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.StartRequestAction(ctx, f.villa.ID, action.ID, ""); err == nil {
		t.Fatal("an action started twice")
	}
	if got := f.get(t, f.villa.ID, r.ID); got.Status != domain.ReqInProgress {
		t.Fatalf("running = %+v", got)
	}
	// The app stops while the action runs: the person checks it.
	if err := f.db.RecoverRequests(ctx); err != nil {
		t.Fatal(err)
	}
	got := f.get(t, f.villa.ID, r.ID)
	if got.Status != domain.ReqProposed || got.Items[0].Status != domain.ItemCheck {
		t.Fatalf("after a stop = %+v", got)
	}
	if _, err := f.db.StartRequestAction(ctx, f.villa.ID, action.ID, "다시"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.FinishRequestAction(ctx, action.ID, map[string]string{"path": "/tmp/x.zip"}, ""); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, f.villa.ID, r.ID); got.Status != domain.ReqInProgress || got.Items[0].Status != domain.ItemDone {
		t.Fatalf("after the action = %+v", got)
	}
	// Its work cancelled, nothing was done: the request ends without a report.
	if _, err := f.db.SetWorkStatus(ctx, f.villa.ID, workItem.WorkItemID, domain.WorkCancelled, ""); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, f.villa.ID, r.ID); got.Status != domain.ReqDone || got.Report != "" {
		t.Fatalf("after cancelling the work = %+v", got)
	}
}

// A call cut off by a stop goes back to the queue; cancelling a routed
// company request cancels its open service requests.
func TestRequestRecoveryAndCancel(t *testing.T) {
	f := newIntakeFixture(t)
	f.desks(t)
	ctx := context.Background()
	r := f.submit(t, f.villa.ID, "안녕")
	if ok, err := f.db.ClaimRequest(ctx, r.ID); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if _, err := f.db.StartRequestAttempt(ctx, f.get(t, f.villa.ID, r.ID), f.conn.ID, "m"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.CancelRequest(ctx, f.villa.ID, r.ID); err == nil {
		t.Fatal("cancelled while the desk decides")
	}
	if err := f.db.RecoverRequests(ctx); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, f.villa.ID, r.ID); got.Status != domain.ReqReceived || got.Attempts != 1 {
		t.Fatalf("after a stop = %+v", got)
	}

	// The blog desk is off, so its part waits; cancelling the company
	// request cancels it.
	off, _, _ := f.db.IntakeDesk(ctx, f.blog.ID)
	off.Enabled = false
	if _, err := f.db.SaveIntakeDesk(ctx, off); err != nil {
		t.Fatal(err)
	}
	f.answer = func(desk, prompt string) (string, error) {
		if desk == "비서실" {
			return asJSON(map[string]any{"outcome": "route", "reason": "블로그", "routes": []map[string]any{{"projectId": f.blog.ID}}}), nil
		}
		return `{"outcome":"answer","reason":"r","answer":"네"}`, nil
	}
	c := f.submit(t, "", "블로그에 글 써 줘")
	f.drain(t)
	c = f.get(t, "", c.ID)
	if c.Status != domain.ReqRouted || len(c.Children) != 1 || c.Children[0].Status != domain.ReqWaitingDesk || c.Children[0].Body != "블로그에 글 써 줘" {
		t.Fatalf("routed = %+v", c)
	}
	if err := f.db.CancelRequest(ctx, "", c.ID); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t, "", c.ID); got.Status != domain.ReqCancelled || got.Children[0].Status != domain.ReqCancelled {
		t.Fatalf("cancelled = %+v", got)
	}
}

func testMonthStart() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
}

// An archived service is not offered to the company desk, and a route to
// it is refused like any unknown service; its own requests are held.
func TestArchivedServicesAreNotRoutedTo(t *testing.T) {
	f := newIntakeFixture(t)
	f.desks(t)
	ctx := context.Background()
	held := f.submit(t, f.blog.ID, "글 써 줘")
	if err := f.db.SetProjectArchived(ctx, f.blog.ID, true); err != nil {
		t.Fatal(err)
	}
	f.answer = func(desk, prompt string) (string, error) {
		if strings.Contains(prompt, f.blog.ID) {
			return "", errors.New("the archived service was offered")
		}
		return asJSON(map[string]any{"outcome": "route", "reason": "블로그", "routes": []map[string]any{{"projectId": f.blog.ID}}}), nil
	}
	r := f.submit(t, "", "블로그에 글 써 줘")
	f.drain(t)
	got := f.get(t, "", r.ID)
	if got.Status != domain.ReqFailed || got.Attempts != 3 || len(got.Children) != 0 || !strings.Contains(got.Error, "서비스 목록에 없는") {
		t.Fatalf("route to an archived service = %+v", got)
	}
	if got := f.get(t, f.blog.ID, held.ID); got.Status != domain.ReqHeld || !strings.Contains(got.HoldReason, "보관된 서비스") {
		t.Fatalf("archived service's request = %+v", got)
	}
}

// A request to a service without a desk makes an AI look at the service
// and set up a new desk: an AI assignee in the one shared desk role, with
// instructions written for the service. The request is then decided, and
// shows which desk was set up for it. The company desk is set up alike.
func TestMissingDeskIsStaffedByAI(t *testing.T) {
	f := newIntakeFixture(t)
	ctx := context.Background()
	f.answer = func(desk, prompt string) (string, error) {
		switch {
		case desk == "접수 담당 배치 AI" && strings.Contains(prompt, "회사 접수 담당 배치"):
			return `{"name":"비서실","instructions":"빌라노트와 블로그 중 어디의 일인지 가린다."}`, nil
		case desk == "접수 담당 배치 AI" && strings.Contains(prompt, "## 지금 지침"):
			return `{"name":"빌라노트 접수 담당","instructions":"새 지침: 실제 자료 연결 여부를 먼저 본다."}`, nil
		case desk == "접수 담당 배치 AI":
			if !strings.Contains(prompt, "빌라 물건을 분석한다") && !strings.Contains(prompt, "회사 블로그") {
				return "", errors.New("the service was not described")
			}
			name := "빌라노트 접수 담당"
			if strings.Contains(prompt, "회사 블로그") {
				name = "블로그 접수 담당"
			}
			return asJSON(map[string]string{"name": name, "instructions": name + " 지침: 개발팀이 코드를 맡는다."}), nil
		case desk == "비서실":
			return asJSON(map[string]any{"outcome": "route", "reason": "블로그", "routes": []map[string]any{{"projectId": f.blog.ID}}}), nil
		default:
			return `{"outcome":"answer","reason":"r","answer":"네"}`, nil
		}
	}
	r := f.submit(t, f.villa.ID, "안녕")
	f.drain(t)
	got := f.get(t, f.villa.ID, r.ID)
	if got.Status != domain.ReqDone || got.StaffedDesk != "빌라노트 접수 담당" {
		t.Fatalf("request = %+v", got)
	}
	desk, ok, _ := f.db.IntakeDesk(ctx, f.villa.ID)
	people, _ := f.db.Assignments(ctx, f.villa.ID)
	var a domain.Assignment
	for _, x := range people {
		if x.ID == desk.AssignmentID {
			a = x
		}
	}
	if !ok || !desk.Enabled || a.DisplayName != "빌라노트 접수 담당" || a.ConnectionID != f.conn.ID || a.Model != "m" ||
		!strings.Contains(a.Overrides.Instructions, "개발팀이 코드를 맡는다") {
		t.Fatalf("desk = %+v, assignee = %+v", desk, a)
	}
	if !slicesContain(f.calls, "빌라노트 접수 담당") {
		t.Fatalf("the new desk did not decide: %v", f.calls)
	}
	if last := f.prompts[len(f.prompts)-1]; !strings.Contains(last, "너(빌라노트 접수 담당)를 방금 배치했다") {
		t.Fatalf("the new desk was not told it was just set up:\n%s", last)
	}
	// Staffing is paid from the service budget like any desk call.
	if u, _ := f.eng.ProjectUsage(ctx, f.villa.ID); u.Attempts != 2 {
		t.Fatalf("usage = %+v", u)
	}

	// The company desk is set up too, and its route to 블로그 sets up
	// 블로그's desk in the same shared role.
	c := f.submit(t, "", "블로그에 글 써 줘")
	f.drain(t)
	if got := f.get(t, "", c.ID); got.Status != domain.ReqDone || got.StaffedDesk != "비서실" || len(got.Children) != 1 || got.Children[0].StaffedDesk != "블로그 접수 담당" {
		t.Fatalf("company request = %+v", got)
	}
	cd, _, _ := f.db.IntakeDesk(ctx, "")
	if cd.ConnectionID != f.conn.ID || cd.Instructions == "" {
		t.Fatalf("company desk = %+v", cd)
	}
	roles, _ := f.db.Roles(ctx)
	n := 0
	for _, r := range roles {
		if r.Name == storage.DeskRoleName {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("desk roles = %d", n)
	}

	// The person can have the instructions written again.
	if _, err := f.in.RewriteDesk(ctx, f.villa.ID); err != nil {
		t.Fatal(err)
	}
	people, _ = f.db.Assignments(ctx, f.villa.ID)
	for _, x := range people {
		if x.ID == desk.AssignmentID && !strings.Contains(x.Overrides.Instructions, "실제 자료 연결") {
			t.Fatalf("rewritten = %+v", x)
		}
	}
	if s, ok, _ := f.db.LatestStaffing(ctx, f.villa.ID); !ok || s.Kind != "rewrite" || s.Status != "succeeded" {
		t.Fatalf("latest staffing = %+v", s)
	}
}

func slicesContain(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// An existing service without departments (like 빌라노트 was) gets its
// organization proposed by an AI and applied by the person: departments
// of its AI assignees with auto start in auto mode, a desk, the chosen
// connection allowed for auto mode, and its waiting request work moved
// into the assignee's new department and queued for the auto starter.
func TestOrganizeExistingService(t *testing.T) {
	f := newIntakeFixture(t)
	ctx := context.Background()
	p, _ := f.db.CreateProject(ctx, domain.Project{Name: "검증 서비스", Goal: "리포트를 검증한다", Mode: domain.ModeAuto})
	role, _ := f.db.SaveRole(ctx, domain.Role{Name: "분석"})
	ai := func(name string) domain.Assignment {
		a, _ := f.db.SaveAssignment(ctx, domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI, DisplayName: name, ConnectionID: f.conn.ID, Model: "m"})
		return a
	}
	planner, dev, qa := ai("기획 AI"), ai("개발 AI"), ai("검증 AI")
	// Work a desk registered before there were departments: it waits.
	if _, err := f.db.SaveIntakeDesk(ctx, domain.IntakeDesk{ProjectID: p.ID, AssignmentID: planner.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.answer = func(desk, prompt string) (string, error) {
		switch desk {
		case "조직 정비 AI":
			if !strings.Contains(prompt, qa.ID) || strings.Contains(prompt, "intakeDesk.instructions에는") {
				return "", errors.New("unexpected organize prompt")
			}
			return asJSON(map[string]any{"intakeDesk": map[string]string{"name": "", "instructions": ""}, "departments": []map[string]any{
				{"name": "기획팀", "responsibility": "방향", "members": []string{planner.ID}, "lead": planner.ID, "watches": []any{}},
				{"name": "개발팀", "responsibility": "개발과 검증", "members": []string{dev.ID, qa.ID}, "lead": dev.ID,
					"watches": []map[string]any{{"eventKinds": []string{"document.shared"}, "guidance": "기획안을 본다"}}}}}), nil
		}
		return asJSON(map[string]any{"outcome": "work", "reason": "검증", "work": []map[string]any{{"title": "리포트 검증", "assigneeAssignmentId": qa.ID}}}), nil
	}
	r := f.submit(t, p.ID, "리포트 검증해 줘")
	f.drain(t)
	r = f.get(t, p.ID, r.ID)
	if len(r.Items) != 1 || r.Items[0].WorkItemID == "" {
		t.Fatalf("request = %+v", r)
	}
	if starts, _ := f.db.AutoStarts(ctx, p.ID, storage.AutoStartFilter{}); len(starts) != 0 {
		t.Fatalf("work without a department was queued: %+v", starts)
	}

	prop, err := f.in.ProposeOrg(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if prop.Status != "ready" || len(prop.Plan.Departments) != 2 || prop.Plan.IntakeDesk != nil {
		t.Fatalf("proposal = %+v", prop)
	}
	applied, err := f.in.ApplyOrgProposal(ctx, p.ID, prop.ID, []string{f.conn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(applied.Departments) != 2 || !applied.AutoStart || applied.AdoptedWork != 1 || len(applied.Problems) != 0 {
		t.Fatalf("applied = %+v", applied)
	}
	depts, _ := f.db.Departments(ctx, p.ID)
	for _, d := range depts {
		if !domain.ParseAutomation(d.Automation).AutoStart {
			t.Fatalf("department %s does not start work on its own", d.Name)
		}
	}
	w, _ := f.db.WorkItem(ctx, p.ID, r.Items[0].WorkItemID)
	if dev2 := depts[1]; w.DepartmentID == "" || (w.DepartmentID != dev2.ID && w.DepartmentID != depts[0].ID) {
		t.Fatalf("work = %+v", w)
	}
	if starts, _ := f.db.AutoStarts(ctx, p.ID, storage.AutoStartFilter{}); len(starts) != 1 || starts[0].WorkItemID != w.ID {
		t.Fatalf("auto starts = %+v", starts)
	}
	proj, _ := f.db.Project(ctx, p.ID)
	if !strings.Contains(string(proj.AutoPolicy), f.conn.ID) {
		t.Fatalf("auto policy = %s", proj.AutoPolicy)
	}
	if _, err := f.in.ApplyOrgProposal(ctx, p.ID, prop.ID, nil); err == nil {
		t.Fatal("a proposal was applied twice")
	}
	if u, _ := f.eng.ProjectUsage(ctx, p.ID); u.Attempts != 2 {
		t.Fatalf("usage = %+v", u)
	}

	// Nobody is left to place and the desk exists: nothing to propose.
	if _, err := f.in.ProposeOrg(ctx, p.ID); err == nil {
		t.Fatal("proposed an organization for a fully organized service")
	}
}

// A proposal naming someone who is not an eligible assignee is refused
// and kept as failed.
func TestOrganizeRefusesUnknownMembers(t *testing.T) {
	f := newIntakeFixture(t)
	ctx := context.Background()
	f.answer = func(desk, prompt string) (string, error) {
		return asJSON(map[string]any{"intakeDesk": map[string]string{"name": "블로그 접수 담당", "instructions": "글을 다룬다"},
			"departments": []map[string]any{{"name": "편집팀", "responsibility": "편집", "members": []string{f.villaAI.ID}, "lead": f.villaAI.ID, "watches": []any{}}}}), nil
	}
	if _, err := f.in.ProposeOrg(ctx, f.blog.ID); err == nil || !strings.Contains(err.Error(), "찾을 수 없습니다") {
		t.Fatalf("propose: %v", err)
	}
	if p, ok, _ := f.db.LatestOrgProposal(ctx, f.blog.ID); !ok || p.Status != "failed" {
		t.Fatalf("latest = %+v", p)
	}
}

// Work of several departments: the desk's shared goal holds it, the later
// work waits for the earlier (the auto starter holds it with a note), and
// when the person registers the work out of order the link still holds.
func TestGoalAndOrderedWork(t *testing.T) {
	f := newIntakeFixture(t)
	f.desks(t)
	ctx := context.Background()
	role, _ := f.db.SaveRole(ctx, domain.Role{Name: "검증"})
	qaAI, _ := f.db.SaveAssignment(ctx, domain.Assignment{ProjectID: f.villa.ID, RoleID: role.ID, ActorKind: domain.ActorAI, DisplayName: "검증 AI", ConnectionID: f.conn.ID, Model: "m"})
	qa, err := f.db.SaveDepartment(ctx, domain.Department{ProjectID: f.villa.ID, Name: "검증팀", Responsibility: "검증", MemberIDs: []string{qaAI.ID}, LeadAssignmentID: qaAI.ID})
	if err != nil {
		t.Fatal(err)
	}
	decision := asJSON(map[string]any{"outcome": "work", "reason": "개발 뒤 검증",
		"goal": map[string]any{"title": "역전세 지표 출시", "doneCriteria": "검증까지 끝남", "departmentId": f.dev.ID},
		"work": []map[string]any{
			{"title": "지표 구현", "departmentId": f.dev.ID},
			{"title": "지표 검증", "departmentId": qa.ID, "after": []int{1, 2, 7}},
		}})
	f.answer = func(string, string) (string, error) { return decision, nil }

	// Review mode, registered out of order: the check first, then the build.
	r := f.submit(t, f.villa.ID, "역전세 지표 만들고 검증해 줘")
	f.drain(t)
	r = f.get(t, f.villa.ID, r.ID)
	if r.GoalPlan == nil || r.GoalPlan.Title != "역전세 지표 출시" || len(r.Items) != 2 {
		t.Fatalf("decision = %+v", r)
	}
	check, err := f.db.AcceptRequestWork(ctx, f.villa.ID, r.Items[1].ID, domain.WorkProposalPayload{})
	if err != nil {
		t.Fatal(err)
	}
	build, err := f.db.AcceptRequestWork(ctx, f.villa.ID, r.Items[0].ID, domain.WorkProposalPayload{})
	if err != nil {
		t.Fatal(err)
	}
	r = f.get(t, f.villa.ID, r.ID)
	goal, _ := f.db.WorkItem(ctx, f.villa.ID, r.GoalID)
	check, _ = f.db.WorkItem(ctx, f.villa.ID, check.ID)
	if goal.Kind != domain.WorkGoal || goal.DepartmentID != f.dev.ID || r.GoalTitle != "역전세 지표 출시" ||
		check.ParentID != goal.ID || build.ParentID != goal.ID || len(check.AfterIDs) != 1 || check.AfterIDs[0] != build.ID {
		t.Fatalf("goal = %+v, check = %+v, build = %+v", goal, check, build)
	}

	// Auto mode: the check waits for the build before it may start.
	p := f.villa
	p.Mode = domain.ModeAuto
	if _, err := f.db.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{f.dev.ID, qa.ID} {
		if _, err := f.db.SetDepartmentAutomation(ctx, f.villa.ID, d, domain.DepartmentAutomation{AutoStart: true}); err != nil {
			t.Fatal(err)
		}
	}
	r2 := f.submit(t, f.villa.ID, "한 번 더")
	f.drain(t)
	r2 = f.get(t, f.villa.ID, r2.ID)
	if r2.GoalID == "" || r2.GoalID == r.GoalID || len(r2.Items) != 2 || r2.Items[1].WorkItemID == "" {
		t.Fatalf("auto decision = %+v", r2)
	}
	auto := work.NewAutoStarter(work.AutoConfig{DB: f.db,
		Hold:  func(ctx context.Context, projectID string) (string, error) { return "", nil },
		Start: func(context.Context, string, string) (string, error) { return "", errors.New("not in this test") }})
	auto.Drain(ctx)
	r2 = f.get(t, f.villa.ID, r2.ID)
	if r2.Items[1].AutoStatus != storage.AutoPending || !strings.Contains(r2.Items[1].AutoNote, "‘지표 구현’") {
		t.Fatalf("the check did not wait: %+v", r2.Items[1])
	}
	if r2.Items[0].AutoStatus == storage.AutoPending && r2.Items[0].AutoNote != "" {
		t.Fatalf("the build waited: %+v", r2.Items[0])
	}
	if _, err := f.db.SetWorkStatus(ctx, f.villa.ID, r2.Items[0].WorkItemID, domain.WorkDone, ""); err != nil {
		t.Fatal(err)
	}
	auto.Drain(ctx)
	if got := f.get(t, f.villa.ID, r2.ID); got.Items[1].AutoStatus == storage.AutoPending {
		t.Fatalf("the check still waits after the build: %+v", got.Items[1])
	}
}
