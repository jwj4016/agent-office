package storage_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

func mustWork(t *testing.T, c *company, w domain.WorkItem) domain.WorkItem {
	t.Helper()
	w.ProjectID = c.project.ID
	out, err := c.db.CreateWorkItem(context.Background(), w)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func ask(t *testing.T, c *company, from domain.WorkItem, to domain.Department, requirement string, versions ...string) storage.CollabRequest {
	t.Helper()
	r, err := c.db.RequestCollab(context.Background(), c.project.ID, storage.NewCollabRequest{FromWorkItemID: from.ID, ToDepartmentID: to.ID,
		Question: to.Name + "의 검토를 부탁합니다", InputVersionIDs: versions, Requirement: requirement})
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func setStatus(t *testing.T, c *company, w string, status string) error {
	t.Helper()
	_, err := c.db.SetWorkStatus(context.Background(), c.project.ID, w, status, "")
	return err
}

func mustSetStatus(t *testing.T, c *company, w string, status string) {
	t.Helper()
	if err := setStatus(t, c, w, status); err != nil {
		t.Fatalf("%s → %s: %v", w, status, err)
	}
}

func requiredChecks(t *testing.T, c *company, workID string) []storage.ImpactCheck {
	t.Helper()
	all, err := c.db.ImpactChecks(context.Background(), c.project.ID, workID, "open")
	if err != nil {
		t.Fatal(err)
	}
	var out []storage.ImpactCheck
	for _, ch := range all {
		if ch.Cause == storage.CauseRequiredReopened {
			out = append(out, ch)
		}
	}
	return out
}

func blocked(err error) bool {
	return err != nil && errors.Is(err, storage.ErrInvalid) && strings.Contains(err.Error(), "필수 협업")
}

// Stage 6 acceptance at the storage level: marketing asks development
// and finance separately, both departments take the request on and work
// in parallel, and the goal is completed only after the required results
// are done. Reopening a result later flags the finished work and goal
// for review instead of reopening them.
func TestCollaborationGatesTheGoal(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	subscribe(t, c, c.fin, domain.SubscribableEvents) // a subscription does not widen a direct request
	goal := mustWork(t, c, domain.WorkItem{Kind: domain.WorkGoal, Title: "신규 고객 캠페인", DepartmentID: c.mkt.ID})
	plan := mustWork(t, c, domain.WorkItem{ParentID: goal.ID, DepartmentID: c.mkt.ID, Title: "캠페인 기획", Categories: []string{"기획"}})
	_, v1 := share(t, c, storage.NewDocumentVersion{Title: "기획안", OwnerDepartmentID: c.mkt.ID, WorkItemID: plan.ID}, "1판")

	toDev := ask(t, c, plan, c.dev, storage.CollabRequired, v1.ID)
	toFin := ask(t, c, plan, c.fin, storage.CollabRequired)
	if got := strings.Join(inbox(t, c, toDev.EventSequence), ","); got != "dev:collab_request:pending" {
		t.Fatalf("request to development reached %s", got)
	}
	if got := strings.Join(inbox(t, c, toFin.EventSequence), ","); got != "fin:collab_request:pending" {
		t.Fatalf("request to finance reached %s", got)
	}
	if ev := lastEvent(t, c, plan.ID); ev.Kind != domain.EventWorkCollabRequested || ev.Event.CollabRequestID != toFin.ID ||
		ev.Event.TargetDepartmentID != c.fin.ID || ev.Event.DepartmentID != c.mkt.ID {
		t.Fatalf("collaboration event = %+v", ev)
	}
	// Nothing was decided yet: both requests keep the plan and the goal open.
	if err := setStatus(t, c, plan.ID, domain.WorkDone); !blocked(err) || !strings.Contains(err.Error(), "개발팀: 수락 대기") {
		t.Fatalf("complete with requests waiting: %v", err)
	}

	devReq, err := c.db.AcceptCollab(ctx, c.project.ID, toDev.ID, storage.CollabAcceptance{Title: "캠페인 페이지 구현 검토"})
	if err != nil {
		t.Fatal(err)
	}
	finReq, err := c.db.AcceptCollab(ctx, c.project.ID, toFin.ID, storage.CollabAcceptance{})
	if err != nil {
		t.Fatal(err)
	}
	devWork, _ := c.db.WorkItem(ctx, c.project.ID, devReq.ResultWorkItemID)
	finWork, _ := c.db.WorkItem(ctx, c.project.ID, finReq.ResultWorkItemID)
	if devWork.DepartmentID != c.dev.ID || devWork.ParentID != goal.ID || devWork.Title != "캠페인 페이지 구현 검토" {
		t.Fatalf("development work = %+v", devWork)
	}
	if finWork.DepartmentID != c.fin.ID || finWork.ParentID != goal.ID || !strings.Contains(finWork.Title, "캠페인 기획") {
		t.Fatalf("finance work = %+v", finWork)
	}
	if devReq.DecidedBy != "user" || devReq.History[len(devReq.History)-1].AsDepartmentID != c.dev.ID {
		t.Fatalf("decision record = %+v", devReq)
	}
	if docs, _ := c.db.WorkDocuments(ctx, c.project.ID, devWork.ID); len(docs) != 1 || docs[0].Version.ID != v1.ID {
		t.Fatalf("material sent with the request is not linked: %+v", docs)
	}
	if ev := lastEvent(t, c, devWork.ID); ev.Kind != domain.EventWorkCreated || ev.Event.CauseSequence == nil || *ev.Event.CauseSequence != toDev.EventSequence {
		t.Fatalf("development work does not start from the request: %+v", ev)
	}
	if _, err := c.db.AcceptCollab(ctx, c.project.ID, toDev.ID, storage.CollabAcceptance{}); err == nil {
		t.Fatal("a request was accepted twice")
	}

	// The two departments work on their own; one done is not enough.
	mustSetStatus(t, c, finWork.ID, domain.WorkInProgress)
	mustSetStatus(t, c, devWork.ID, domain.WorkDone)
	if err := setStatus(t, c, goal.ID, domain.WorkDone); !blocked(err) || !strings.Contains(err.Error(), "재무팀: 수행 업무") {
		t.Fatalf("goal with finance unfinished: %v", err)
	}
	if blockers, _ := c.db.CompletionBlockers(ctx, c.project.ID, goal.ID); len(blockers) != 1 || blockers[0].ID != toFin.ID {
		t.Fatalf("goal blockers = %+v", blockers)
	}
	mustSetStatus(t, c, finWork.ID, domain.WorkDone)
	mustSetStatus(t, c, plan.ID, domain.WorkDone)
	mustSetStatus(t, c, goal.ID, domain.WorkDone)

	// Development reopens its review: the plan and the goal stay done and
	// get a check each.
	mustSetStatus(t, c, devWork.ID, domain.WorkInProgress)
	for _, id := range []string{plan.ID, goal.ID} {
		w, _ := c.db.WorkItem(ctx, c.project.ID, id)
		checks := requiredChecks(t, c, id)
		if w.Status != domain.WorkDone || len(checks) != 1 || checks[0].RefID != toDev.ID || !strings.Contains(checks[0].Summary, "그대로 두었습니다") {
			t.Fatalf("%s after the required work reopened: %s %+v", w.Title, w.Status, checks)
		}
	}
	// Reopening the goal now needs development's work done again.
	mustSetStatus(t, c, goal.ID, domain.WorkInProgress)
	if err := setStatus(t, c, goal.ID, domain.WorkDone); !blocked(err) {
		t.Fatalf("goal completed with reopened required work: %v", err)
	}
	// A second reopen opens the same check again instead of adding one.
	checks := requiredChecks(t, c, plan.ID)
	if err := c.db.CloseImpactCheck(ctx, c.project.ID, checks[0].ID, "resolved", ""); err != nil {
		t.Fatal(err)
	}
	mustSetStatus(t, c, devWork.ID, domain.WorkDone)
	mustSetStatus(t, c, devWork.ID, domain.WorkCancelled)
	if checks := requiredChecks(t, c, plan.ID); len(checks) != 1 || !strings.Contains(checks[0].Summary, "취소되었습니다") {
		t.Fatalf("checks after cancelling = %+v", checks)
	}
	all, _ := c.db.ImpactChecks(ctx, c.project.ID, plan.ID, "")
	if len(all) != 1 {
		t.Fatalf("impact checks kept = %d", len(all))
	}
}

// A required request blocks completion in every state short of accepted
// with its work done, even when no work was ever made for it. Only an
// explicit release with a reason lifts it; reference requests never
// block.
func TestRequiredRequestWithoutWorkBlocks(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	p := c.project.ID
	work := mustWork(t, c, domain.WorkItem{DepartmentID: c.mkt.ID, Title: "기획"})

	ref := ask(t, c, work, c.fin, storage.CollabReference)
	if _, err := c.db.DeclineCollab(ctx, p, ref.ID, "이번 분기 여력이 없습니다"); err != nil {
		t.Fatal(err)
	}
	req := ask(t, c, work, c.dev, storage.CollabRequired)
	if _, err := c.db.AskCollab(ctx, p, req.ID, "언제까지 필요한가요?"); err != nil {
		t.Fatal(err)
	}
	if err := setStatus(t, c, work.ID, domain.WorkDone); !blocked(err) || !strings.Contains(err.Error(), "질문에 답해야") || strings.Contains(err.Error(), "재무팀") {
		t.Fatalf("question: %v", err)
	}
	if _, err := c.db.AnswerCollab(ctx, p, req.ID, "다음 주 금요일"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.DeclineCollab(ctx, p, req.ID, ""); err == nil {
		t.Fatal("declined without a reason")
	}
	declined, err := c.db.DeclineCollab(ctx, p, req.ID, "인력이 없습니다")
	if err != nil || declined.Status != storage.CollabDeclined {
		t.Fatalf("decline: %+v %v", declined, err)
	}
	if err := setStatus(t, c, work.ID, domain.WorkDone); !blocked(err) || !strings.Contains(err.Error(), "거절됨") {
		t.Fatalf("declined: %v", err)
	}
	if _, err := c.db.CancelCollab(ctx, p, req.ID, "다른 방법을 찾음"); err != nil {
		t.Fatal(err)
	}
	if err := setStatus(t, c, work.ID, domain.WorkDone); !blocked(err) || !strings.Contains(err.Error(), "취소됨") {
		t.Fatalf("cancelled: %v", err)
	}
	if _, err := c.db.SetCollabRequirement(ctx, p, req.ID, storage.CollabReference, ""); err == nil {
		t.Fatal("released without a reason")
	}
	released, err := c.db.SetCollabRequirement(ctx, p, req.ID, storage.CollabReference, "내부에서 처리하기로 함")
	if err != nil || released.RequirementReason != "내부에서 처리하기로 함" {
		t.Fatalf("release: %+v %v", released, err)
	}
	var actions []string
	for _, h := range released.History {
		actions = append(actions, h.Action)
	}
	if got := strings.Join(actions, ","); got != "requested,required,asked,answered,declined,cancelled,reference" {
		t.Fatalf("history = %s", got)
	}
	if released.History[2].AsDepartmentID != c.dev.ID || released.History[3].AsDepartmentID != c.mkt.ID {
		t.Fatalf("who acted = %+v", released.History)
	}
	mustSetStatus(t, c, work.ID, domain.WorkDone)

	// Overdue is shown, never treated as met.
	other := mustWork(t, c, domain.WorkItem{DepartmentID: c.mkt.ID, Title: "이벤트"})
	late, err := c.db.RequestCollab(ctx, p, storage.NewCollabRequest{FromWorkItemID: other.ID, ToDepartmentID: c.dev.ID,
		Question: "검토", Requirement: storage.CollabRequired, DueAt: "2020-01-01"})
	if err != nil || !late.Overdue {
		t.Fatalf("overdue request: %+v %v", late, err)
	}
	if err := setStatus(t, c, other.ID, domain.WorkDone); !blocked(err) || !strings.Contains(err.Error(), "기한 지남") {
		t.Fatalf("overdue: %v", err)
	}
}

// Replacing a required request moves the requirement to a new request in
// one step: the old one becomes reference with the reason, and the new
// one must be met before completion.
func TestReplaceRequiredRequest(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	p := c.project.ID
	work := mustWork(t, c, domain.WorkItem{DepartmentID: c.mkt.ID, Title: "기획"})
	old := ask(t, c, work, c.dev, storage.CollabRequired)
	if _, err := c.db.DeclineCollab(ctx, p, old.ID, "개발 여력 없음"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.ReplaceCollab(ctx, p, old.ID, storage.NewCollabRequest{ToDepartmentID: c.fin.ID, Question: "외주 비용 검토"}, ""); err == nil {
		t.Fatal("replaced without a reason")
	}
	next, err := c.db.ReplaceCollab(ctx, p, old.ID, storage.NewCollabRequest{ToDepartmentID: c.fin.ID, Question: "외주 비용 검토"}, "외주로 돌림")
	if err != nil || next.Requirement != storage.CollabRequired || next.ReplacesID != old.ID || next.FromWorkItemID != work.ID {
		t.Fatalf("replacement: %+v %v", next, err)
	}
	prev, _ := c.db.CollabRequest(ctx, p, old.ID)
	if prev.Requirement != storage.CollabReference || prev.ReplacedByID != next.ID || prev.History[len(prev.History)-1].Action != "replaced" {
		t.Fatalf("replaced request: %+v", prev)
	}
	if _, err := c.db.ReplaceCollab(ctx, p, old.ID, storage.NewCollabRequest{ToDepartmentID: c.fin.ID, Question: "x"}, "다시"); err == nil {
		t.Fatal("a reference request was replaced")
	}
	if err := setStatus(t, c, work.ID, domain.WorkDone); !blocked(err) || !strings.Contains(err.Error(), "재무팀") || strings.Contains(err.Error(), "개발팀") {
		t.Fatalf("after replacement: %v", err)
	}
	acc, err := c.db.AcceptCollab(ctx, p, next.ID, storage.CollabAcceptance{})
	if err != nil {
		t.Fatal(err)
	}
	mustSetStatus(t, c, acc.ResultWorkItemID, domain.WorkDone)
	mustSetStatus(t, c, work.ID, domain.WorkDone)
}

// Required requests may not make two work items wait on each other,
// whether the loop appears on accepting an existing work item or on
// making an accepted request required.
func TestRequiredCycleRefused(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	p := c.project.ID
	plan := mustWork(t, c, domain.WorkItem{DepartmentID: c.mkt.ID, Title: "기획"})
	build := mustWork(t, c, domain.WorkItem{DepartmentID: c.dev.ID, Title: "구현"})

	first := ask(t, c, plan, c.dev, storage.CollabRequired)
	if _, err := c.db.AcceptCollab(ctx, p, first.ID, storage.CollabAcceptance{ExistingWorkItemID: plan.ID}); err == nil {
		t.Fatal("marketing's work accepted for development")
	}
	if _, err := c.db.AcceptCollab(ctx, p, first.ID, storage.CollabAcceptance{ExistingWorkItemID: build.ID}); err != nil {
		t.Fatal(err)
	}
	back := ask(t, c, build, c.mkt, storage.CollabRequired)
	if _, err := c.db.AcceptCollab(ctx, p, back.ID, storage.CollabAcceptance{ExistingWorkItemID: plan.ID}); err == nil || !strings.Contains(err.Error(), "서로의 완료") {
		t.Fatalf("cycle on accept: %v", err)
	}
	// As a reference it may use the plan; making it required is refused.
	if _, err := c.db.SetCollabRequirement(ctx, p, back.ID, storage.CollabReference, "참고만 하면 됨"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.AcceptCollab(ctx, p, back.ID, storage.CollabAcceptance{ExistingWorkItemID: plan.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.SetCollabRequirement(ctx, p, back.ID, storage.CollabRequired, ""); err == nil || !strings.Contains(err.Error(), "서로의 완료") {
		t.Fatalf("cycle on requiring: %v", err)
	}
	if r, _ := c.db.CollabRequest(ctx, p, back.ID); r.Requirement != storage.CollabReference {
		t.Fatalf("requirement after refusal = %s", r.Requirement)
	}
	// The plan is the work of another request already.
	third := ask(t, c, build, c.mkt, storage.CollabReference)
	if _, err := c.db.AcceptCollab(ctx, p, third.ID, storage.CollabAcceptance{ExistingWorkItemID: plan.ID}); err == nil {
		t.Fatal("one work item became the work of two requests")
	}
}

// A request is checked before its event exists: the asking work needs a
// department and must be open, the target must be another active
// department of the service, and material must be visible to it.
func TestCollabRequestChecks(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	p := c.project.ID
	work := mustWork(t, c, domain.WorkItem{DepartmentID: c.mkt.ID, Title: "기획"})
	loose := mustWork(t, c, domain.WorkItem{Title: "부서 없는 일"})
	_, secret := share(t, c, storage.NewDocumentVersion{Title: "내부 자료", OwnerDepartmentID: c.mkt.ID, Scope: "departments"}, "비밀")
	other, _ := c.db.CreateProject(ctx, domain.Project{Name: "다른 회사"})
	otherDept, _ := c.db.SaveDepartment(ctx, domain.Department{ProjectID: other.ID, Name: "개발팀"})
	inactive, _ := c.db.SaveDepartment(ctx, domain.Department{ProjectID: p, Name: "법무팀", Status: domain.StatusInactive})

	for name, n := range map[string]storage.NewCollabRequest{
		"same department":     {FromWorkItemID: work.ID, ToDepartmentID: c.mkt.ID, Question: "x"},
		"inactive":            {FromWorkItemID: work.ID, ToDepartmentID: inactive.ID, Question: "x"},
		"other service":       {FromWorkItemID: work.ID, ToDepartmentID: otherDept.ID, Question: "x"},
		"no question":         {FromWorkItemID: work.ID, ToDepartmentID: c.dev.ID, Question: " "},
		"no department":       {FromWorkItemID: loose.ID, ToDepartmentID: c.dev.ID, Question: "x"},
		"hidden material":     {FromWorkItemID: work.ID, ToDepartmentID: c.dev.ID, Question: "x", InputVersionIDs: []string{secret.ID}},
		"unknown requirement": {FromWorkItemID: work.ID, ToDepartmentID: c.dev.ID, Question: "x", Requirement: "maybe"},
		"bad due date":        {FromWorkItemID: work.ID, ToDepartmentID: c.dev.ID, Question: "x", DueAt: "내일"},
	} {
		if _, err := c.db.RequestCollab(ctx, p, n); err == nil {
			t.Errorf("%s: request accepted", name)
		}
	}
	mustSetStatus(t, c, work.ID, domain.WorkDone)
	if _, err := c.db.RequestCollab(ctx, p, storage.NewCollabRequest{FromWorkItemID: work.ID, ToDepartmentID: c.dev.ID, Question: "x"}); err == nil {
		t.Fatal("a closed work item asked for help")
	}
	if list, _ := c.db.CollabRequests(ctx, p, storage.CollabFilter{}); len(list) != 0 {
		t.Fatalf("refused requests were stored: %+v", list)
	}
	if _, err := c.db.CollabRequests(ctx, other.ID, storage.CollabFilter{WorkItemID: work.ID}); err != nil {
		t.Fatal(err)
	}
	if list, _ := c.db.CollabRequests(ctx, other.ID, storage.CollabFilter{}); len(list) != 0 {
		t.Fatalf("another service sees requests: %+v", list)
	}
}

// What waits on a person: requests to decide, questions to answer and
// declined required requests. A goal lists the requests of its work.
func TestOpenCollabRequests(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	p := c.project.ID
	goal := mustWork(t, c, domain.WorkItem{Kind: domain.WorkGoal, Title: "캠페인", DepartmentID: c.mkt.ID})
	work := mustWork(t, c, domain.WorkItem{ParentID: goal.ID, DepartmentID: c.mkt.ID, Title: "기획"})
	a := ask(t, c, work, c.dev, storage.CollabReference)
	b := ask(t, c, work, c.fin, storage.CollabRequired)
	d := ask(t, c, goal, c.dev, storage.CollabReference)
	c.db.DeclineCollab(ctx, p, a.ID, "안 함")
	c.db.DeclineCollab(ctx, p, b.ID, "안 함")
	c.db.AcceptCollab(ctx, p, d.ID, storage.CollabAcceptance{})
	open, _ := c.db.CollabRequests(ctx, "", storage.CollabFilter{Open: true})
	if len(open) != 1 || open[0].ID != b.ID || open[0].ProjectName != "캠페인" {
		t.Fatalf("open = %+v", open)
	}
	underGoal, _ := c.db.CollabRequests(ctx, p, storage.CollabFilter{WorkItemID: goal.ID})
	if len(underGoal) != 3 {
		t.Fatalf("goal lists %d requests", len(underGoal))
	}
	forDev, _ := c.db.CollabRequests(ctx, p, storage.CollabFilter{DepartmentID: c.dev.ID})
	if len(forDev) != 2 {
		t.Fatalf("development received %d", len(forDev))
	}
	acc, _ := c.db.CollabRequest(ctx, p, d.ID)
	if res, _ := c.db.CollabRequests(ctx, p, storage.CollabFilter{ResultWorkItemID: acc.ResultWorkItemID}); len(res) != 1 || res[0].ID != d.ID {
		t.Fatalf("by result work = %+v", res)
	}
}

// A goal that is already done is not reopened when a required request
// under it appears: it gets a check instead, and moving work with an
// unmet required request under a done goal does the same.
func TestDoneGoalIsFlaggedNotReopened(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	p := c.project.ID
	goal := mustWork(t, c, domain.WorkItem{Kind: domain.WorkGoal, Title: "캠페인", DepartmentID: c.mkt.ID})
	work := mustWork(t, c, domain.WorkItem{ParentID: goal.ID, DepartmentID: c.mkt.ID, Title: "후속 기획"})
	mustSetStatus(t, c, goal.ID, domain.WorkDone)
	r := ask(t, c, work, c.dev, storage.CollabReference)
	if len(requiredChecks(t, c, goal.ID)) != 0 {
		t.Fatal("a reference request flagged the goal")
	}
	if _, err := c.db.SetCollabRequirement(ctx, p, r.ID, storage.CollabRequired, "출시에 꼭 필요"); err != nil {
		t.Fatal(err)
	}
	g, _ := c.db.WorkItem(ctx, p, goal.ID)
	if checks := requiredChecks(t, c, goal.ID); g.Status != domain.WorkDone || len(checks) != 1 || !strings.Contains(checks[0].Summary, "후속 기획") {
		t.Fatalf("goal after a new required request: %s %+v", g.Status, checks)
	}

	other := mustWork(t, c, domain.WorkItem{DepartmentID: c.mkt.ID, Title: "별도 기획"})
	r2 := ask(t, c, other, c.fin, storage.CollabRequired)
	other.ParentID = goal.ID
	if _, err := c.db.UpdateWorkItem(ctx, other); err != nil {
		t.Fatal(err)
	}
	if checks := requiredChecks(t, c, goal.ID); len(checks) != 2 || (checks[0].RefID != r2.ID && checks[1].RefID != r2.ID) {
		t.Fatalf("goal after moving work under it: %+v", checks)
	}
	// A done work item cannot be given new required collaboration.
	mustSetStatus(t, c, other.ID, domain.WorkCancelled)
	ref := mustWork(t, c, domain.WorkItem{DepartmentID: c.mkt.ID, Title: "참고"})
	rr := ask(t, c, ref, c.dev, storage.CollabReference)
	mustSetStatus(t, c, ref.ID, domain.WorkDone)
	if _, err := c.db.SetCollabRequirement(ctx, p, rr.ID, storage.CollabRequired, ""); err == nil {
		t.Fatal("closed work was given a required request")
	}
}
