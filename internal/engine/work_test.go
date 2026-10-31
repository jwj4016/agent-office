package engine_test

import (
	"errors"
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

// workSetup is a service with a development department whose AI member
// has a work item and a plan to run.
type workSetup struct {
	p    testenv.Project
	dev  domain.Department
	asg  string
	item domain.WorkItem
}

func newWorkSetup(t *testing.T, h *harness) workSetup {
	t.Helper()
	ctx := h.ctx
	p := testenv.SeedDraft(t, h.db, "캠페인", []byte(`{"schemaVersion":1,"title":"빈 흐름","nodes":[]}`), map[string]string{"a-dev": "개발자"}, nil)
	asg := p.Assignments["a-dev"]
	dev, err := h.db.SaveDepartment(ctx, domain.Department{ProjectID: p.ID, Name: "개발팀", Responsibility: "구현 가능성을 검토한다",
		MemberIDs: []string{asg}, LeadAssignmentID: asg})
	if err != nil {
		t.Fatal(err)
	}
	item, err := h.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: p.ID, DepartmentID: dev.ID, Title: "캠페인 페이지 구현 검토",
		Description: "랜딩 페이지 범위", DoneCriteria: "구현 난이도와 일정이 정리되어 있다"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.db.SaveWorkPlan(ctx, domain.WorkPlan{ProjectID: p.ID, WorkItemID: item.ID, AssignmentID: asg,
		Instructions: "기존 코드 구조를 먼저 본다", Materials: "기획안 초안 1판"}); err != nil {
		t.Fatal(err)
	}
	return workSetup{p: p, dev: dev, asg: asg, item: item}
}

func (s workSetup) work(t *testing.T, h *harness) domain.WorkItem {
	t.Helper()
	w, err := h.db.WorkItem(h.ctx, s.p.ID, s.item.ID)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func TestWorkRunNeedsAPersonToComplete(t *testing.T) {
	h := newHarness(t)
	s := newWorkSetup(t, h)
	runID, err := h.eng.StartWork(h.ctx, s.p.ID, s.item.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	d := h.waitRun(s.p.ID, runID, engine.RunSucceeded)
	if len(d.Step(domain.WorkStepID).Artifacts) == 0 {
		t.Fatalf("no result: %s", describe(d))
	}
	// The step was told the work, its done criteria and the plan; the
	// department responsibility comes through the department layer.
	req := h.started(domain.WorkStepID)[0]
	prompt := req.Instructions + "\n" + req.Prompt
	for _, want := range []string{"캠페인 페이지 구현 검토", "## 완료 조건", "구현 난이도와 일정", "## 작업 지침", "## 참고 자료", "## 부서: 개발팀", "구현 가능성을 검토한다"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, prompt)
		}
	}
	// Success alone does not complete the work.
	w := s.work(t, h)
	if w.Status != domain.WorkWaiting || !strings.Contains(w.StatusReason, "결과를 확인") {
		t.Fatalf("after success = %s (%s)", w.Status, w.StatusReason)
	}
	if _, err := h.db.SetWorkStatus(h.ctx, s.p.ID, s.item.ID, domain.WorkDone, "결과 확인"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.eng.StartWork(h.ctx, s.p.ID, s.item.ID, ""); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("run of a closed item: %v", err)
	}
	// The work's own workflow stays out of the workflow list.
	if list, _ := h.db.Workflows(h.ctx, s.p.ID); len(list) != 1 || list[0].Title != "빈 흐름" {
		t.Fatalf("workflows = %+v", list)
	}
	notes, _ := h.db.WorkNotes(h.ctx, s.p.ID, s.item.ID)
	var trail []string
	for _, n := range notes {
		trail = append(trail, n.ToStatus+"@"+strings.SplitN(n.Author, ":", 2)[0])
	}
	if got := strings.Join(trail, " "); got != "received@local-owner in_progress@run waiting@run done@local-owner" {
		t.Fatalf("history = %s", got)
	}
}

func TestWorkChangeRequestRunsAgainWithTheResult(t *testing.T) {
	h := newHarness(t)
	s := newWorkSetup(t, h)
	first, _ := h.eng.StartWork(h.ctx, s.p.ID, s.item.ID, "")
	h.waitRun(s.p.ID, first, engine.RunSucceeded)
	second, err := h.eng.StartWork(h.ctx, s.p.ID, s.item.ID, "일정 근거를 추가해 주세요")
	if err != nil {
		t.Fatal(err)
	}
	h.waitRun(s.p.ID, second, engine.RunSucceeded)
	req := h.started(domain.WorkStepID)[1]
	prompt := req.Instructions + "\n" + req.Prompt
	if !strings.Contains(prompt, "## 이전 결과에 대한 수정 요청\n일정 근거를 추가해 주세요") || !strings.Contains(prompt, "## 이전 결과\n### result") {
		t.Fatalf("second prompt:\n%s", prompt)
	}
	runs, _ := h.eng.WorkRuns(h.ctx, s.p.ID, s.item.ID)
	if len(runs) != 2 || runs[0].ID != second || runs[0].VersionNumber != 2 || runs[1].VersionNumber != 1 {
		t.Fatalf("runs = %+v", runs)
	}
}

func TestOneActiveRunPerWork(t *testing.T) {
	h := newHarness(t)
	s := newWorkSetup(t, h)
	h.useScenario(domain.WorkStepID, "slow")
	runID, err := h.eng.StartWork(h.ctx, s.p.ID, s.item.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	h.waitStep(s.p.ID, runID, domain.WorkStepID, engine.StRunning)
	if _, err := h.eng.StartWork(h.ctx, s.p.ID, s.item.ID, ""); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("second active run: %v", err)
	}
	if err := h.eng.CancelRun(h.ctx, s.p.ID, runID); err != nil {
		t.Fatal(err)
	}
	if w := s.work(t, h); w.Status != domain.WorkReady {
		t.Fatalf("after cancel = %s", w.Status)
	}
	h.useScenario(domain.WorkStepID, "auto")
	again, err := h.eng.StartWork(h.ctx, s.p.ID, s.item.ID, "")
	if err != nil {
		t.Fatalf("run after cancel: %v", err)
	}
	h.waitRun(s.p.ID, again, engine.RunSucceeded)
}

// A retried older run finishing late must not move the work item: only
// the latest run speaks for it.
func TestOlderRunDoesNotMoveWork(t *testing.T) {
	h := newHarness(t)
	s := newWorkSetup(t, h)
	h.useScenario(domain.WorkStepID, "provider-failure")
	old, _ := h.eng.StartWork(h.ctx, s.p.ID, s.item.ID, "")
	h.waitRun(s.p.ID, old, engine.RunFailed)
	if w := s.work(t, h); w.Status != domain.WorkBlocked {
		t.Fatalf("after failure = %s", w.Status)
	}
	h.useScenario(domain.WorkStepID, "auto")
	latest, err := h.eng.StartWork(h.ctx, s.p.ID, s.item.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	h.waitRun(s.p.ID, latest, engine.RunSucceeded)
	if _, err := h.db.SetWorkStatus(h.ctx, s.p.ID, s.item.ID, domain.WorkInProgress, "결과를 검토 중"); err != nil {
		t.Fatal(err)
	}
	before, _ := h.db.WorkNotes(h.ctx, s.p.ID, s.item.ID)
	if err := h.eng.RetryStep(h.ctx, s.p.ID, old, domain.WorkStepID); err != nil {
		t.Fatal(err)
	}
	h.waitRun(s.p.ID, old, engine.RunSucceeded)
	after, _ := h.db.WorkNotes(h.ctx, s.p.ID, s.item.ID)
	if w := s.work(t, h); w.Status != domain.WorkInProgress || len(after) != len(before) {
		t.Fatalf("older run moved the work: %s, notes %d -> %d", w.Status, len(before), len(after))
	}
}
