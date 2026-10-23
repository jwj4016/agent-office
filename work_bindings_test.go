package main

import (
	"strings"
	"testing"
	"time"

	"agent-office/internal/domain"
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
