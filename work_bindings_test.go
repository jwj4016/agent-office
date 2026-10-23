package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// Stage 1 acceptance through the UI-facing API: company templates for
// marketing, development and finance are copied into a service, a shared
// goal holds each department's work, and it all survives a restart of
// the app. Another service copies the same template independently.
func TestBindingsDepartmentsAndWork(t *testing.T) {
	a := startApp(t)
	p := need(a.CreateProject(ProjectInput{Name: "캠페인", Mode: "review"}))
	conn := need(a.EnsureTestConnection())

	depts := map[string]domain.Department{}
	templates := map[string]domain.DepartmentTemplate{}
	for _, name := range []string{"마케팅팀", "개발팀", "재무팀"} {
		role := need(a.SaveRole(domain.Role{Name: name + " 담당"}))
		templates[name] = need(a.SaveDepartmentTemplate(domain.DepartmentTemplate{Name: name, Responsibility: name + "의 책임", RoleIDs: []string{role.ID}}))
		asg := need(a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
			DisplayName: name + " AI", ConnectionID: conn.ID, Model: "auto"}))
		d := need(a.ImportDepartmentTemplate(p.ID, templates[name].ID))
		if d.LeadAssignmentID != asg.ID || d.TemplateID != templates[name].ID {
			t.Fatalf("%s from template: %+v", name, d)
		}
		depts[name] = d
		if prev := need(a.InstructionPreview(p.ID, asg.ID)); !strings.Contains(prev.Composed, "## 부서: "+name+"\n"+name+"의 책임") {
			t.Fatalf("%s preview has no department layer:\n%s", name, prev.Composed)
		}
	}

	goal := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, Kind: domain.WorkGoal, Title: "신규 고객 캠페인 준비",
		DepartmentID: depts["마케팅팀"].ID}))
	for name, title := range map[string]string{"마케팅팀": "기획안 작성", "개발팀": "페이지 구현 검토", "재무팀": "예산 검토"} {
		need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, ParentID: goal.ID, DepartmentID: depts[name].ID, Title: title}))
	}
	if _, err := a.SetWorkStatus(p.ID, goal.ID, domain.WorkBlocked, ""); err == nil || strings.Contains(err.Error(), "invalid input") {
		t.Fatalf("blocked without reason: %v", err)
	}

	// Restart the app on the same data folder: everything is still there.
	a.shutdown(a.ctx)
	b := NewApp()
	b.emit = a.emit
	b.startup(t.Context())
	t.Cleanup(func() { b.shutdown(t.Context()) })
	d := need(b.GetWorkItem(p.ID, goal.ID))
	if len(d.Children) != 3 || len(d.Notes) != 1 || d.Item.DepartmentID != depts["마케팅팀"].ID {
		t.Fatalf("goal after restart: %+v", d)
	}
	dev := need(b.ListWorkItems(p.ID, storage.WorkFilter{DepartmentID: depts["개발팀"].ID}))
	if len(dev) != 1 || dev[0].Title != "페이지 구현 검토" {
		t.Fatalf("development work: %+v", dev)
	}
	if got := need(b.ListDepartments(p.ID)); len(got) != 3 {
		t.Fatalf("service departments: %+v", got)
	}

	// Another service copies the same template into its own department and
	// cannot read this service's work.
	other := need(b.CreateProject(ProjectInput{Name: "다른 서비스", Mode: "review"}))
	copied := need(b.ImportDepartmentTemplate(other.ID, templates["개발팀"].ID))
	if copied.ID == depts["개발팀"].ID || copied.ProjectID != other.ID || len(copied.MemberIDs) != 0 {
		t.Fatalf("copy in another service: %+v", copied)
	}
	if _, err := b.GetWorkItem(other.ID, goal.ID); err == nil || !strings.Contains(err.Error(), "이 서비스에서 찾을 수 없는") {
		t.Fatalf("cross-service read: %v", err)
	}
}

func waitDeliveries(t *testing.T, a *App, projectID string, done int) []storage.Delivery {
	t.Helper()
	deadline := time.Now().Add(e2eWait)
	for {
		list := need(a.ListDeliveries(projectID, storage.DeliveryFilter{}))
		n := 0
		for _, d := range list {
			if d.Status == domain.DeliveryDone {
				n++
			}
		}
		if n >= done {
			return list
		}
		if time.Now().After(deadline) {
			t.Fatalf("deliveries done = %d, want %d: %+v", n, done, list)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Stage 2 acceptance: one work event reaches two subscribed departments,
// is handled once, and a restart neither loses nor repeats deliveries.
// A direct check reaches a department that subscribes to nothing.
func TestBindingsWorkEventsReachDepartments(t *testing.T) {
	a := startApp(t)
	p := need(a.CreateProject(ProjectInput{Name: "캠페인", Mode: "review"}))
	dept := map[string]domain.Department{}
	for _, name := range []string{"마케팅팀", "개발팀", "재무팀"} {
		dept[name] = need(a.SaveDepartment(domain.Department{ProjectID: p.ID, Name: name}))
	}
	for _, name := range []string{"개발팀", "재무팀"} {
		need(a.SaveSubscription(domain.Subscription{ProjectID: p.ID, DepartmentID: dept[name].ID,
			EventKinds: []string{domain.EventWorkCreated}, Guidance: name + " 관점", Enabled: true}))
	}
	plan := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, DepartmentID: dept["마케팅팀"].ID, Title: "캠페인 기획안"}))
	got := waitDeliveries(t, a, p.ID, 2)
	if len(got) != 2 {
		t.Fatalf("deliveries = %+v", got)
	}
	evs := need(a.ListWorkEvents(p.ID, plan.ID))
	if len(evs) != 1 || evs[0].Kind != domain.EventWorkCreated || len(evs[0].Deliveries) != 2 {
		t.Fatalf("events = %+v", evs)
	}

	// Restart: nothing is lost or handled again; new events still flow.
	a.shutdown(a.ctx)
	b := NewApp()
	b.emit = a.emit
	b.startup(t.Context())
	t.Cleanup(func() { b.shutdown(t.Context()) })
	if err := b.RequestDepartmentCheck(p.ID, dept["마케팅팀"].ID, "다음 분기 캠페인 아이디어"); err != nil {
		t.Fatal(err)
	}
	got = waitDeliveries(t, b, p.ID, 3)
	if len(got) != 3 {
		t.Fatalf("after restart = %+v", got)
	}
	for _, d := range got {
		if d.EventKind == domain.EventDepartmentCheck && (d.DepartmentID != dept["마케팅팀"].ID || d.Event.Note != "다음 분기 캠페인 아이디어") {
			t.Fatalf("check delivery = %+v", d)
		}
	}
	if err := b.RetryDelivery(p.ID, got[0].ID); err == nil || strings.Contains(err.Error(), "invalid input") {
		t.Fatalf("retry of a done delivery: %v", err)
	}
}

// Stage 3 acceptance: a development work item is planned, run, its
// result read, and only then completed by the person. A finished run
// alone leaves the work waiting for that check.
func TestBindingsRunWorkThenComplete(t *testing.T) {
	a := startApp(t)
	p := need(a.CreateProject(ProjectInput{Name: "캠페인", Mode: "review"}))
	conn := need(a.EnsureTestConnection())
	role := need(a.SaveRole(domain.Role{Name: "개발자"}))
	asg := need(a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
		DisplayName: "개발 AI", ConnectionID: conn.ID, Model: "auto"}))
	dev := need(a.SaveDepartment(domain.Department{ProjectID: p.ID, Name: "개발팀", MemberIDs: []string{asg.ID}, LeadAssignmentID: asg.ID}))
	item := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, DepartmentID: dev.ID, Title: "구현 검토", DoneCriteria: "일정이 정리되어 있다"}))

	// Without a saved plan the lead is suggested and nothing runs.
	view := need(a.GetWorkPlan(p.ID, item.ID))
	if view.Saved || view.Plan.AssignmentID != asg.ID || len(view.Plan.Outputs) != 1 {
		t.Fatalf("starting plan = %+v", view)
	}
	if _, err := a.StartWork(p.ID, item.ID, ""); err == nil {
		t.Fatal("ran without a plan")
	}
	plan := view.Plan
	plan.WorkItemID, plan.Instructions = item.ID, "기존 화면을 먼저 본다"
	need(a.SaveWorkPlan(plan))
	view = need(a.GetWorkPlan(p.ID, item.ID))
	if !view.Saved || !strings.Contains(view.Preview, "## 완료 조건\n일정이 정리되어 있다") || view.Problem != "" {
		t.Fatalf("plan view = %+v", view)
	}

	res := need(a.StartWork(p.ID, item.ID, ""))
	if res.RunID == "" {
		t.Fatalf("start = %+v", res)
	}
	deadline := time.Now().Add(e2eWait)
	var run engine.RunDetail
	for {
		runs := need(a.ListWorkRuns(p.ID, item.ID))
		if len(runs) == 1 && runs[0].Status == "succeeded" {
			run = runs[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("work run did not finish: %+v", runs)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if w := need(a.GetWorkItem(p.ID, item.ID)).Item; w.Status != domain.WorkWaiting {
		t.Fatalf("work after a successful run = %s", w.Status)
	}
	art := run.Steps[0].Artifacts[0]
	if content := need(a.ReadArtifact(p.ID, art.ID)); content.Content == "" {
		t.Fatalf("empty result: %+v", content)
	}
	done := need(a.SetWorkStatus(p.ID, item.ID, domain.WorkDone, "결과 확인함"))
	if done.Status != domain.WorkDone || done.ClosedAt == "" {
		t.Fatalf("completed = %+v", done)
	}
	// The run tab still lists the run; the workflow list does not show
	// the work's own workflow.
	if wfs := need(a.ListWorkflows(p.ID)); len(wfs) != 0 {
		t.Fatalf("workflows = %+v", wfs)
	}
	if runs := need(a.ListRuns(p.ID)); len(runs) != 1 || runs[0].ID != res.RunID {
		t.Fatalf("runs = %+v", runs)
	}
}

// Stage 3-2: the department's AI proposes a plan for a click (a repeated
// click is one request), the proposal is paid from the service budget,
// and applying it saves the plan without starting a run.
func TestBindingsAIPlanProposal(t *testing.T) {
	a := startApp(t)
	p := need(a.CreateProject(ProjectInput{Name: "캠페인", Mode: "review"}))
	conn := need(a.EnsureTestConnection())
	role := need(a.SaveRole(domain.Role{Name: "개발자"}))
	asg := need(a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
		DisplayName: "개발 AI", ConnectionID: conn.ID, Model: "plan-proposal"}))
	proposal, _ := json.Marshal(map[string]any{"assignmentId": asg.ID, "instructions": "화면별로 난이도를 나눈다",
		"outputs": []map[string]string{{"key": "review", "type": "markdown"}}, "notes": "화면 3개 가정"})
	steps := []string{
		`{"emit":"usage","payload":{"inputTokens":50,"outputTokens":20,"costUsd":null}}`,
		`{"write":{"key":"result","content":` + string(need(json.Marshal(string(proposal)))) + `}}`,
		`{"complete":{"status":"succeeded"}}`,
	}
	for _, l := range steps {
		var s providers.Step
		if err := json.Unmarshal([]byte(l), &s); err != nil {
			t.Fatal(err)
		}
		a.testProvider.Scripts["plan-proposal"] = append(a.testProvider.Scripts["plan-proposal"], s)
	}
	dev := need(a.SaveDepartment(domain.Department{ProjectID: p.ID, Name: "개발팀", MemberIDs: []string{asg.ID}, LeadAssignmentID: asg.ID}))
	item := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, DepartmentID: dev.ID, Title: "구현 검토"}))

	first := need(a.RequestWorkPlan(p.ID, item.ID, "click-1"))
	if again := need(a.RequestWorkPlan(p.ID, item.ID, "click-1")); again.ID != first.ID {
		t.Fatal("a repeated click made a second request")
	}
	deadline := time.Now().Add(e2eWait)
	var view PlanJobView
	for {
		jobs := need(a.ListPlanJobs(p.ID, item.ID))
		if len(jobs) == 1 && jobs[0].Job.Status == "ready" {
			view = jobs[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("no proposal: %+v", jobs)
		}
		time.Sleep(20 * time.Millisecond)
	}
	if view.Stale || view.Proposal == nil || view.Proposal.Instructions != "화면별로 난이도를 나눈다" {
		t.Fatalf("proposal = %+v", view)
	}
	if u := need(a.ProjectUsage(p.ID)); u.InputTokens != 50 || u.UnknownCostAttempts != 1 {
		t.Fatalf("usage = %+v", u)
	}
	need(a.ApplyPlanJob(p.ID, view.Job.ID))
	plan := need(a.GetWorkPlan(p.ID, item.ID))
	if !plan.Saved || plan.Plan.Instructions != "화면별로 난이도를 나눈다" || plan.Plan.Outputs[0].Key != "review" {
		t.Fatalf("plan = %+v", plan)
	}
	if runs := need(a.ListWorkRuns(p.ID, item.ID)); len(runs) != 0 {
		t.Fatalf("applying started a run: %+v", runs)
	}
}
