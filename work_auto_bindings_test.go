package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

// Stage 7 acceptance through the app: in an auto-mode service the
// development lead AI takes marketing's reference request on, the work
// gets an AI plan, starts on its own and its result is shared as a draft;
// the work then waits for the person. The decision shows in the inbox
// until the person keeps it.
func TestBindingsAutoModeStartsAllowedWork(t *testing.T) {
	a := startApp(t)
	p := need(a.CreateProject(ProjectInput{Name: "캠페인", Mode: "auto"}))
	p.Budget = json.RawMessage(`{"maxTokens": 1000000}`)
	need(a.db.UpdateProject(a.ctx, p))
	conn := need(a.EnsureTestConnection())
	if err := a.SetAutoPolicy(p.ID, []string{conn.ID}); err != nil {
		t.Fatal(err)
	}
	mkt := aiDepartment(t, a, p.ID, "마케팅팀", "auto")
	dev := aiDepartment(t, a, p.ID, "개발팀", "dev-lead")
	// A second development member does the work itself.
	role := need(a.SaveRole(domain.Role{Name: "개발 작업자"}))
	worker := need(a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI, DisplayName: "개발 작업 AI",
		ConnectionID: conn.ID, Model: "auto"}))
	dev.MemberIDs = append(dev.MemberIDs, worker.ID)
	dev = need(a.SaveDepartment(dev))
	need(a.SetDepartmentAutomation(p.ID, dev.ID, domain.DepartmentAutomation{AutoStart: true, AutoShare: true, AutoCollab: true}))
	// The lead's one scripted answer serves both its calls: the decision
	// reads decision/reason/title, the plan reads assignmentId/instructions/outputs.
	scriptJudgment(t, a, "dev-lead", `{"decision":"accept","reason":"개발팀의 일이다","title":"캠페인 페이지 검토",
		"assignmentId":"`+worker.ID+`","instructions":"페이지 구현 범위를 정리한다","outputs":[{"key":"result","type":"markdown"}]}`)

	plan := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, DepartmentID: mkt.ID, Title: "캠페인 기획"}))
	req := need(a.RequestCollab(p.ID, storage.NewCollabRequest{FromWorkItemID: plan.ID, ToDepartmentID: dev.ID, Question: "페이지 구현 검토를 부탁합니다"}))

	deadline := time.Now().Add(e2eWait)
	var start storage.AutoStart
	for {
		r := need(a.ListCollabRequests(p.ID, storage.CollabFilter{WorkItemID: plan.ID}))[0]
		if r.ResultWorkItemID != "" {
			if list := need(a.ListAutoStarts(p.ID, storage.AutoStartFilter{WorkItemID: r.ResultWorkItemID})); len(list) == 1 {
				start = list[0]
				if start.Status == storage.AutoFinished || start.Status == storage.AutoReview {
					break
				}
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("automatic start did not finish: %+v %+v", r, start)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if start.Status != storage.AutoFinished || !strings.Contains(start.Note, "초안으로 공유") {
		t.Fatalf("automatic start = %+v", start)
	}
	w := need(a.GetWorkItem(p.ID, start.WorkItemID)).Item
	if w.Status != domain.WorkWaiting || w.Title != "캠페인 페이지 검토" {
		t.Fatalf("work after its automatic run = %+v", w)
	}
	view := need(a.GetWorkPlan(p.ID, w.ID))
	if view.Plan.AssignmentID != worker.ID || view.Approval.By != "auto" || !view.Approval.Valid || view.AutoStart == nil {
		t.Fatalf("plan view = %+v", view)
	}
	docs := need(a.ListDocuments(p.ID, storage.DocumentFilter{}))
	if len(docs) != 1 || docs[0].WorkItemID != w.ID || docs[0].Versions[0].Stage != "draft" {
		t.Fatalf("shared = %+v", docs)
	}
	pending := need(a.PendingDecisions())
	if len(pending.AIDecisions) != 1 || pending.AIDecisions[0].ID != req.ID {
		t.Fatalf("AI decisions in the inbox = %+v", pending.AIDecisions)
	}
	need(a.ReviewCollabDecision(p.ID, req.ID))
	if pending := need(a.PendingDecisions()); len(pending.AIDecisions) != 0 {
		t.Fatalf("kept decision still in the inbox: %+v", pending.AIDecisions)
	}
	// The finished run's result waits in the inbox, and the overview counts it.
	if pending := need(a.PendingDecisions()); len(pending.Results) != 1 || pending.Results[0].WorkItemID != w.ID || pending.Results[0].RunID == "" {
		t.Fatalf("results to check = %+v", pending.Results)
	}
	ops := need(a.OperationsOverview(p.ID))
	for _, l := range ops.Departments {
		if l.DepartmentID == dev.ID && (l.ResultsToCheck != 1 || l.Waiting != 1) {
			t.Fatalf("development load = %+v", l)
		}
	}
	if ops.Usage.Attempts == 0 {
		t.Fatalf("usage = %+v", ops.Usage)
	}
	if l := need(a.GetWorkLineage(p.ID, w.ID)); len(l.Causes) != 1 || l.Causes[0].Via != "collab" || l.Causes[0].DecidedBy != "ai" {
		t.Fatalf("lineage = %+v", l)
	}
	// The person completes the work; a run never did.
	need(a.SetWorkStatus(p.ID, w.ID, domain.WorkDone, "결과 확인"))
	if pending := need(a.PendingDecisions()); len(pending.Results) != 0 {
		t.Fatalf("completed work still to check: %+v", pending.Results)
	}
}

// A work item can run a confirmed version of one of the service's
// workflows as it is; it takes no change request, and the person's start
// records an approval that a later plan change voids.
func TestBindingsWorkRunsLinkedWorkflow(t *testing.T) {
	a := startApp(t)
	seeded := testenv.ServiceDev(t, a.db, "게임")
	wf := need(a.GetWorkflow(seeded.ID, seeded.WorkflowID))
	confirm := need(a.ConfirmVersion(seeded.ID, wf.ID, wf.Revision))
	choices := need(a.ListWorkflowChoices(seeded.ID))
	if len(choices) != 1 || choices[0].VersionID != confirm.Version.ID {
		t.Fatalf("choices = %+v", choices)
	}
	item := need(a.CreateWorkItem(domain.WorkItem{ProjectID: seeded.ID, Title: "정기 기능 개발"}))
	need(a.SaveWorkPlan(domain.WorkPlan{ProjectID: seeded.ID, WorkItemID: item.ID, WorkflowVersionID: confirm.Version.ID}))
	view := need(a.GetWorkPlan(seeded.ID, item.ID))
	if !strings.Contains(view.Preview, wf.Title) || view.Problem != "" {
		t.Fatalf("plan view = %+v", view)
	}
	if _, err := a.StartWork(seeded.ID, item.ID, "다르게 해 주세요"); err == nil {
		t.Fatal("a linked workflow took a change request")
	}
	res := need(a.StartWork(seeded.ID, item.ID, ""))
	if res.RunID == "" {
		t.Fatalf("start = %+v", res)
	}
	runs := need(a.ListWorkRuns(seeded.ID, item.ID))
	if len(runs) != 1 || runs[0].VersionID != confirm.Version.ID {
		t.Fatalf("runs = %+v", runs)
	}
	if ap := need(a.GetWorkPlan(seeded.ID, item.ID)).Approval; ap.By != "user" || !ap.Valid {
		t.Fatalf("approval = %+v", ap)
	}
	item.DoneCriteria = "새 완료 조건"
	need(a.UpdateWorkItem(item))
	if ap := need(a.GetWorkPlan(seeded.ID, item.ID)).Approval; ap.Valid {
		t.Fatal("approval still valid after the done criteria changed")
	}
	if err := a.CancelRun(seeded.ID, res.RunID); err != nil {
		t.Fatal(err)
	}
}
