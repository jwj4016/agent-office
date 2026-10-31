package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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

// scriptJudgment makes the test provider answer model with result as its
// structured output, reporting 30 input and 10 output tokens.
func scriptJudgment(t *testing.T, a *App, model, result string) {
	t.Helper()
	for _, l := range []string{
		`{"emit":"usage","payload":{"inputTokens":30,"outputTokens":10,"costUsd":0.001}}`,
		`{"write":{"key":"result","content":` + string(need(json.Marshal(result))) + `}}`,
		`{"complete":{"status":"succeeded"}}`,
	} {
		var s providers.Step
		if err := json.Unmarshal([]byte(l), &s); err != nil {
			t.Fatal(err)
		}
		a.testProvider.Scripts[model] = append(a.testProvider.Scripts[model], s)
	}
}

// aiDepartment is a department led by an AI that answers with model.
func aiDepartment(t *testing.T, a *App, projectID, name, model string) domain.Department {
	t.Helper()
	conn := need(a.EnsureTestConnection())
	role := need(a.SaveRole(domain.Role{Name: name + " 담당 " + projectID}))
	asg := need(a.SaveAssignment(domain.Assignment{ProjectID: projectID, RoleID: role.ID, ActorKind: domain.ActorAI,
		DisplayName: name + " AI", ConnectionID: conn.ID, Model: model}))
	return need(a.SaveDepartment(domain.Department{ProjectID: projectID, Name: name, Responsibility: name + "의 책임",
		MemberIDs: []string{asg.ID}, LeadAssignmentID: asg.ID}))
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
	scriptJudgment(t, a, "judge-none", `{"outcome":"none","reason":"할 일이 없다"}`)
	dept := map[string]domain.Department{}
	for _, name := range []string{"마케팅팀", "개발팀", "재무팀"} {
		dept[name] = aiDepartment(t, a, p.ID, name, "judge-none")
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
	scriptJudgment(t, b, "judge-none", `{"outcome":"none","reason":"할 일이 없다"}`)
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

// Stage 4 through the UI-facing API: a running campaign's draft result
// is shared, another department links it and sees it in its plan, the
// draft is confirmed without changing, and reading it checks the copy.
func TestBindingsShareDraftWithAnotherDepartment(t *testing.T) {
	a := startApp(t)
	p := need(a.CreateProject(ProjectInput{Name: "캠페인", Mode: "review"}))
	conn := need(a.EnsureTestConnection())
	dept := func(name string) (domain.Department, domain.Assignment) {
		role := need(a.SaveRole(domain.Role{Name: name + " 담당"}))
		asg := need(a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
			DisplayName: name + " AI", ConnectionID: conn.ID, Model: "auto"}))
		return need(a.SaveDepartment(domain.Department{ProjectID: p.ID, Name: name, MemberIDs: []string{asg.ID}, LeadAssignmentID: asg.ID})), asg
	}
	mkt, mktAsg := dept("마케팅팀")
	dev, devAsg := dept("개발팀")
	plan := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, DepartmentID: mkt.ID, Title: "기획안 작성"}))
	review := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, DepartmentID: dev.ID, Title: "구현 검토"}))
	need(a.SaveWorkPlan(domain.WorkPlan{ProjectID: p.ID, WorkItemID: plan.ID, AssignmentID: mktAsg.ID}))
	need(a.SaveWorkPlan(domain.WorkPlan{ProjectID: p.ID, WorkItemID: review.ID, AssignmentID: devAsg.ID}))
	run := need(a.StartWork(p.ID, plan.ID, ""))
	deadline := time.Now().Add(e2eWait)
	var art engine.ArtifactView
	for {
		d := need(a.GetRun(p.ID, run.RunID))
		if d.Status == "succeeded" {
			art = d.Steps[0].Artifacts[0]
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run not finished: %s", d.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
	v1 := need(a.ShareArtifact(p.ID, art.ID, storage.NewDocumentVersion{Title: "캠페인 기획안", WorkItemID: plan.ID}))
	docs := need(a.ListDocuments(p.ID, storage.DocumentFilter{VisibleTo: dev.ID}))
	if len(docs) != 1 || docs[0].OwnerDepartmentID != mkt.ID || len(docs[0].Versions) != 1 {
		t.Fatalf("documents = %+v", docs)
	}
	if err := a.LinkDocument(p.ID, review.ID, v1.ID); err != nil {
		t.Fatal(err)
	}
	if view := need(a.GetWorkPlan(p.ID, review.ID)); !strings.Contains(view.Preview, "캠페인 기획안 1판 (초안)") {
		t.Fatalf("preview lacks the draft:\n%s", view.Preview)
	}
	final := need(a.ConfirmDocumentVersion(p.ID, v1.ID, "확정"))
	if final.Stage != "final" || final.Hash != v1.Hash {
		t.Fatalf("confirmed = %+v", final)
	}
	if c := need(a.ReadDocumentVersion(p.ID, v1.ID)); !c.HashOK || c.Content == "" {
		t.Fatalf("read = %+v", c)
	}
	if w := need(a.GetWorkItem(p.ID, plan.ID)).Item; w.Status == domain.WorkDone {
		t.Fatal("sharing completed the work")
	}
}

// Stage 5 acceptance through the UI-facing API: a shared plan makes
// development and finance each propose work while legal, which follows
// nothing, is not asked; accepting a proposal registers the work under
// the department with its cause, and the judgments show in the usage.
func TestBindingsDepartmentsProposeWork(t *testing.T) {
	a := startApp(t)
	ws := t.TempDir()
	p := need(a.CreateProject(ProjectInput{Name: "캠페인", Mode: "review"}))
	p = need(a.UpdateProject(ProjectInput{ID: p.ID, Name: p.Name, Mode: "review", WorkspacePath: ws}))
	propose := func(title string) string {
		return string(need(json.Marshal(map[string]any{"outcome": "propose", "reason": "기획안에 필요한 일이 있다",
			"proposals": []map[string]any{{"title": title, "doneCriteria": title + " 완료", "priority": "high"}}})))
	}
	scriptJudgment(t, a, "judge-dev", propose("캠페인 페이지 구현 검토"))
	scriptJudgment(t, a, "judge-fin", propose("캠페인 예산 검토"))
	scriptJudgment(t, a, "judge-law", `{"outcome":"none","reason":"계약이 없다"}`)
	mkt := need(a.SaveDepartment(domain.Department{ProjectID: p.ID, Name: "마케팅팀"}))
	dev := aiDepartment(t, a, p.ID, "개발팀", "judge-dev")
	fin := aiDepartment(t, a, p.ID, "재무팀", "judge-fin")
	law := aiDepartment(t, a, p.ID, "법무팀", "judge-law")
	for _, d := range []domain.Department{dev, fin} {
		need(a.SaveSubscription(domain.Subscription{ProjectID: p.ID, DepartmentID: d.ID,
			EventKinds: []string{domain.EventDocumentShared}, Guidance: d.Name + " 관점", Enabled: true}))
	}
	goal := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, Kind: domain.WorkGoal, Title: "신규 고객 캠페인", DepartmentID: mkt.ID}))
	plan := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, ParentID: goal.ID, DepartmentID: mkt.ID, Title: "기획안 작성"}))
	if err := os.WriteFile(filepath.Join(ws, "plan.md"), []byte("# 캠페인 기획안\n새 페이지와 광고 예산"), 0o644); err != nil {
		t.Fatal(err)
	}
	need(a.ShareWorkspaceFile(p.ID, "plan.md", storage.NewDocumentVersion{Title: "캠페인 기획안", WorkItemID: plan.ID}))

	deadline := time.Now().Add(e2eWait)
	var props []storage.Proposal
	for {
		props = need(a.ListProposals(p.ID, "proposed"))
		if len(props) == 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	byDept := map[string]storage.Proposal{}
	for _, pr := range props {
		byDept[pr.DepartmentID] = pr
	}
	if len(props) != 2 || byDept[dev.ID].Title != "캠페인 페이지 구현 검토" || byDept[fin.ID].Title != "캠페인 예산 검토" {
		t.Fatalf("proposals = %+v", props)
	}
	for _, d := range need(a.ListDeliveries(p.ID, storage.DeliveryFilter{})) {
		if d.DepartmentID == law.ID {
			t.Fatalf("legal was asked: %+v", d)
		}
	}
	if dec := need(a.PendingDecisions()); len(dec.Proposals) != 2 || dec.Proposals[0].ProjectName != "캠페인" {
		t.Fatalf("inbox decisions = %+v", dec)
	}
	if u := need(a.ProjectUsage(p.ID)); u.Attempts != 2 || u.InputTokens != 60 {
		t.Fatalf("usage = %+v", u)
	}

	w := need(a.AcceptProposal(p.ID, byDept[dev.ID].ID, storage.ProposalEdit{}))
	if w.DepartmentID != dev.ID || w.ParentID != goal.ID || w.Priority != "high" {
		t.Fatalf("work = %+v", w)
	}
	if batch := need(a.DecideProposals(p.ID, []string{byDept[dev.ID].ID, byDept[fin.ID].ID}, false, "나중에")); batch.Done != 1 || len(batch.Errors) != 1 {
		t.Fatalf("batch = %+v", batch)
	}
	if left := need(a.ListProposals(p.ID, "proposed")); len(left) != 0 {
		t.Fatalf("left = %+v", left)
	}

	// A department created later reviews the shared plan on request.
	versions := need(a.GetDocument(p.ID, need(a.ListDocuments(p.ID, storage.DocumentFilter{}))[0].ID)).Versions
	if n := need(a.RequestDocumentReview(p.ID, law.ID, []string{versions[0].ID}, "")); n != 1 {
		t.Fatalf("review requests = %d", n)
	}
	deadline = time.Now().Add(e2eWait)
	for {
		list := need(a.ListDeliveries(p.ID, storage.DeliveryFilter{DepartmentID: law.ID}))
		if len(list) == 1 && list[0].Status == domain.DeliveryDone {
			if !strings.Contains(list[0].Note, "계약이 없다") {
				t.Fatalf("legal note = %q", list[0].Note)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("legal review = %+v", list)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// startPlannedWork saves the suggested plan (the department lead) and
// starts it.
func startPlannedWork(t *testing.T, a *App, projectID, workItemID string) {
	t.Helper()
	view := need(a.GetWorkPlan(projectID, workItemID))
	plan := view.Plan
	plan.WorkItemID = workItemID
	need(a.SaveWorkPlan(plan))
	if res := need(a.StartWork(projectID, workItemID, "")); res.RunID == "" {
		t.Fatalf("start %s = %+v", workItemID, res)
	}
}

func waitWorkStatus(t *testing.T, a *App, projectID, workItemID, status string) {
	t.Helper()
	deadline := time.Now().Add(e2eWait)
	for need(a.GetWorkItem(projectID, workItemID)).Item.Status != status {
		if time.Now().After(deadline) {
			t.Fatalf("%s did not reach %s", workItemID, status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Stage 6 acceptance: marketing asks development and finance for help
// separately, each department takes its request on as its own work and
// runs it in parallel, and the goal is completed only after the person
// checked both required results. Reopening a result afterwards flags the
// finished goal in the inbox instead of reopening it.
func TestBindingsCollaborationCompletesGoal(t *testing.T) {
	a := startApp(t)
	p := need(a.CreateProject(ProjectInput{Name: "캠페인", Mode: "review"}))
	mkt := aiDepartment(t, a, p.ID, "마케팅팀", "auto")
	dev := aiDepartment(t, a, p.ID, "개발팀", "auto")
	fin := aiDepartment(t, a, p.ID, "재무팀", "auto")
	goal := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, Kind: domain.WorkGoal, Title: "신규 고객 캠페인 준비", DepartmentID: mkt.ID}))
	plan := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, ParentID: goal.ID, DepartmentID: mkt.ID, Title: "캠페인 기획"}))

	toDev := need(a.RequestCollab(p.ID, storage.NewCollabRequest{FromWorkItemID: plan.ID, ToDepartmentID: dev.ID,
		Question: "캠페인 페이지를 만들 수 있는지 검토해 주세요", Requirement: storage.CollabRequired}))
	toFin := need(a.RequestCollab(p.ID, storage.NewCollabRequest{FromWorkItemID: plan.ID, ToDepartmentID: fin.ID,
		Question: "예산 범위를 확인해 주세요", Requirement: storage.CollabRequired}))
	for _, d := range waitDeliveries(t, a, p.ID, 2) {
		if d.Purpose != domain.PurposeCollabRequest || !strings.Contains(d.Note, "협업 요청") {
			t.Fatalf("delivery = %+v", d)
		}
	}
	if pending := need(a.PendingDecisions()); len(pending.Collabs) != 2 {
		t.Fatalf("inbox collaboration = %+v", pending.Collabs)
	}
	if _, err := a.SetWorkStatus(p.ID, goal.ID, domain.WorkDone, ""); err == nil || !strings.Contains(err.Error(), "필수 협업") ||
		strings.Contains(err.Error(), "invalid input") {
		t.Fatalf("goal completed before anyone helped: %v", err)
	}

	devReq := need(a.AcceptCollab(p.ID, toDev.ID, storage.CollabAcceptance{Title: "캠페인 페이지 구현 검토"}))
	finReq := need(a.AcceptCollab(p.ID, toFin.ID, storage.CollabAcceptance{Title: "캠페인 예산 검토"}))
	if pending := need(a.PendingDecisions()); len(pending.Collabs) != 0 {
		t.Fatalf("decided requests still in the inbox: %+v", pending.Collabs)
	}
	// Both departments run their own work at the same time.
	startPlannedWork(t, a, p.ID, devReq.ResultWorkItemID)
	startPlannedWork(t, a, p.ID, finReq.ResultWorkItemID)
	for _, id := range []string{devReq.ResultWorkItemID, finReq.ResultWorkItemID} {
		waitWorkStatus(t, a, p.ID, id, domain.WorkWaiting)
	}
	collab := need(a.GetWorkCollab(p.ID, goal.ID))
	if len(collab.Requests) != 2 || len(collab.Blockers) != 2 {
		t.Fatalf("goal collaboration = %+v", collab)
	}
	if in := need(a.GetWorkCollab(p.ID, devReq.ResultWorkItemID)).Incoming; len(in) != 1 || in[0].FromWorkItemID != plan.ID {
		t.Fatalf("development work does not lead back to the plan: %+v", in)
	}

	// The person checks each result, then completes the plan and the goal.
	need(a.SetWorkStatus(p.ID, devReq.ResultWorkItemID, domain.WorkDone, "검토 결과 확인"))
	if _, err := a.SetWorkStatus(p.ID, plan.ID, domain.WorkDone, ""); err == nil || !strings.Contains(err.Error(), "재무팀") {
		t.Fatalf("plan completed with finance unchecked: %v", err)
	}
	need(a.SetWorkStatus(p.ID, finReq.ResultWorkItemID, domain.WorkDone, "예산 확인"))
	need(a.SetWorkStatus(p.ID, plan.ID, domain.WorkDone, ""))
	if g := need(a.SetWorkStatus(p.ID, goal.ID, domain.WorkDone, "필수 협업 결과 확인")); g.Status != domain.WorkDone {
		t.Fatalf("goal = %+v", g)
	}

	need(a.SetWorkStatus(p.ID, devReq.ResultWorkItemID, domain.WorkInProgress, "추가 수정"))
	if g := need(a.GetWorkItem(p.ID, goal.ID)).Item; g.Status != domain.WorkDone {
		t.Fatalf("goal reopened behind the person's back: %s", g.Status)
	}
	notices := need(a.PendingDecisions()).Notices
	if len(notices) != 2 {
		t.Fatalf("notices = %+v", notices)
	}
	if checks := need(a.GetWorkCollab(p.ID, goal.ID)).Checks; len(checks) != 1 || checks[0].RefID != toDev.ID {
		t.Fatalf("goal checks = %+v", checks)
	}
}

// Stage 8: a department's regular check is saved through the app with the
// PC's time zone, lists its next slot, and refuses what cannot run.
func TestBindingsDepartmentSchedule(t *testing.T) {
	a := startApp(t)
	p := need(a.CreateProject(ProjectInput{Name: "캠페인"}))
	mkt := aiDepartment(t, a, p.ID, "마케팅팀", "auto")
	friday := 5
	saved := need(a.SaveDepartmentSchedule(domain.DepartmentSchedule{ProjectID: p.ID, DepartmentID: mkt.ID, Frequency: domain.ScheduleWeekly,
		TimeOfDay: "10:30", Weekday: &friday, Timezone: "Asia/Seoul", Note: "이번 주 자료를 본다", Enabled: true}))
	list := need(a.ListDepartmentSchedules(p.ID))
	if len(list) != 1 || list[0].ID != saved.ID || list[0].NextDueAt == "" || list[0].DepartmentName != "마케팅팀" {
		t.Fatalf("schedules = %+v", list)
	}
	next, _ := time.Parse(time.RFC3339, list[0].NextDueAt)
	seoul, _ := time.LoadLocation("Asia/Seoul")
	if l := next.In(seoul); l.Weekday() != time.Friday || l.Format("15:04") != "10:30" {
		t.Fatalf("next slot = %s", l)
	}
	if _, err := a.SaveDepartmentSchedule(domain.DepartmentSchedule{ProjectID: p.ID, DepartmentID: mkt.ID, Frequency: domain.ScheduleDaily,
		TimeOfDay: "10:30", Timezone: "Mars/Base"}); err == nil || !strings.Contains(err.Error(), "시간대") {
		t.Fatalf("unknown zone: %v", err)
	}
	if err := a.SetProjectArchived(p.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SaveDepartmentSchedule(saved); err == nil {
		t.Fatal("saved a schedule in an archived service")
	}
}
