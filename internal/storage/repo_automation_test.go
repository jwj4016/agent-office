package storage_test

import (
	"context"
	"errors"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

// A work plan may run a confirmed version of one of the service's own
// workflows, never another service's or a work item's own; changing it
// voids an earlier start approval.
func TestWorkPlansRunServiceWorkflows(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	p := testenv.ServiceDev(t, db, "게임")
	wf, _ := db.Workflow(ctx, p.ID, p.WorkflowID)
	v, _, err := db.ConfirmVersion(ctx, p.ID, wf.ID, wf.Revision)
	if err != nil {
		t.Fatal(err)
	}
	other := testenv.ServiceDev(t, db, "부동산")
	owf, _ := db.Workflow(ctx, other.ID, other.WorkflowID)
	ov, _, _ := db.ConfirmVersion(ctx, other.ID, owf.ID, owf.Revision)

	choices, err := db.WorkflowChoices(ctx, p.ID)
	if err != nil || len(choices) != 1 || choices[0].VersionID != v.ID || choices[0].Number != 1 {
		t.Fatalf("choices = %+v %v", choices, err)
	}
	w, _ := db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: p.ID, Title: "정기 배포"})
	plan, err := db.SaveWorkPlan(ctx, domain.WorkPlan{ProjectID: p.ID, WorkItemID: w.ID, WorkflowVersionID: v.ID})
	if err != nil || plan.WorkflowVersionID != v.ID {
		t.Fatalf("linked plan = %+v %v", plan, err)
	}
	if _, err := db.SaveWorkPlan(ctx, domain.WorkPlan{ProjectID: p.ID, WorkItemID: w.ID, WorkflowVersionID: ov.ID}); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("another service's version: %v", err)
	}
	own, err := db.WorkVersion(ctx, p.ID, w.ID, domain.WorkflowSpec{SchemaVersion: domain.WorkflowSchemaVersion, Title: "업무: 정기 배포",
		Nodes: []domain.Node{{ID: domain.WorkStepID, Title: "배포", Kind: domain.KindTask, AssignmentID: p.Assignments["a-backend"],
			DependsOn: []string{}, Instructions: "배포", Outputs: domain.DefaultWorkOutputs()}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveWorkPlan(ctx, domain.WorkPlan{ProjectID: p.ID, WorkItemID: w.ID, WorkflowVersionID: own.ID}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("a work item's own workflow: %v", err)
	}
	if choices, _ := db.WorkflowChoices(ctx, p.ID); len(choices) != 1 {
		t.Fatalf("work item workflows listed as choices: %+v", choices)
	}

	hash, _ := db.WorkStartHash(ctx, p.ID, w.ID)
	if err := db.ApproveWorkStart(ctx, p.ID, w.ID, "user", hash); err != nil {
		t.Fatal(err)
	}
	if ap, _ := db.WorkStartApproval(ctx, p.ID, w.ID); ap.By != "user" || !ap.Valid {
		t.Fatalf("approval = %+v", ap)
	}
	db.SaveWorkPlan(ctx, domain.WorkPlan{ProjectID: p.ID, WorkItemID: w.ID, AssignmentID: p.Assignments["a-backend"], Instructions: "직접 배포"})
	if ap, _ := db.WorkStartApproval(ctx, p.ID, w.ID); ap.Valid {
		t.Fatal("approval survived a plan change")
	}
}

// A department's automation rules start all off, and only known result
// types, existing connections and the service's own workflow versions
// can be allowed.
func TestDepartmentAutomationRules(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	off := domain.ParseAutomation(c.dev.Automation)
	if off.AutoStart || off.AutoShare || off.AutoCollab || !off.AllowsOutput(domain.OutMarkdown) || off.AllowsOutput(domain.OutCodeChange) {
		t.Fatalf("new department rules = %+v", off)
	}
	conn, _ := c.db.SaveConnection(ctx, domain.ProviderConnection{Name: "test", Provider: "test"})
	d, err := c.db.SetDepartmentAutomation(ctx, c.project.ID, c.dev.ID, domain.DepartmentAutomation{AutoStart: true, Categories: []string{"개발", "개발"},
		ConnectionIDs: []string{conn.ID}, OutputTypes: []domain.OutputType{domain.OutCodeChange}, AutoShare: true})
	if err != nil {
		t.Fatal(err)
	}
	rules := domain.ParseAutomation(d.Automation)
	if !rules.AutoStart || !rules.AutoShare || rules.AutoCollab || len(rules.Categories) != 1 || !rules.AllowsOutput(domain.OutCodeChange) ||
		rules.AllowsOutput(domain.OutMarkdown) || !rules.AllowsConnection(conn.ID) || rules.AllowsConnection("other") {
		t.Fatalf("saved rules = %+v", rules)
	}
	if !rules.AllowsCategories([]string{"개발"}) || rules.AllowsCategories([]string{"개발", "비용"}) || rules.AllowsCategories(nil) {
		t.Fatal("category rule")
	}
	for name, bad := range map[string]domain.DepartmentAutomation{
		"output type": {OutputTypes: []domain.OutputType{"video"}},
		"connection":  {ConnectionIDs: []string{"conn-nope"}},
		"version":     {WorkflowVersionIDs: []string{"wv-nope"}},
	} {
		if _, err := c.db.SetDepartmentAutomation(ctx, c.project.ID, c.dev.ID, bad); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	// Saving the department itself keeps its rules.
	d.Name = "개발본부"
	if d, err = c.db.SaveDepartment(ctx, d); err != nil || !domain.ParseAutomation(d.Automation).AutoStart {
		t.Fatalf("rules after saving the department: %+v %v", d, err)
	}
}

// In review mode, or for a department that does not start work on its
// own, new work gets no automatic start; manually created work never
// does.
func TestAutoStartsQueuedOnlyWhenAllowed(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	work := mustWork(t, c, domain.WorkItem{DepartmentID: c.mkt.ID, Title: "기획"})
	accept := func() string {
		t.Helper()
		r := ask(t, c, work, c.dev, storage.CollabReference)
		acc, err := c.db.AcceptCollab(ctx, c.project.ID, r.ID, storage.CollabAcceptance{})
		if err != nil {
			t.Fatal(err)
		}
		return acc.ResultWorkItemID
	}
	c.db.SetDepartmentAutomation(ctx, c.project.ID, c.dev.ID, domain.DepartmentAutomation{AutoStart: true})
	accept() // review mode
	p, _ := c.db.Project(ctx, c.project.ID)
	p.Mode = domain.ModeAuto
	if _, err := c.db.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	c.db.SetDepartmentAutomation(ctx, c.project.ID, c.dev.ID, domain.DepartmentAutomation{})
	accept() // the department does not start on its own
	if list, _ := c.db.AutoStarts(ctx, c.project.ID, storage.AutoStartFilter{}); len(list) != 0 {
		t.Fatalf("queued = %+v", list)
	}
	c.db.SetDepartmentAutomation(ctx, c.project.ID, c.dev.ID, domain.DepartmentAutomation{AutoStart: true})
	id := accept()
	list, _ := c.db.AutoStarts(ctx, c.project.ID, storage.AutoStartFilter{})
	if len(list) != 1 || list[0].WorkItemID != id || list[0].Status != storage.AutoPending {
		t.Fatalf("queued = %+v", list)
	}
	mustWork(t, c, domain.WorkItem{DepartmentID: c.dev.ID, Title: "직접 등록"})
	if list, _ := c.db.AutoStarts(ctx, c.project.ID, storage.AutoStartFilter{}); len(list) != 1 {
		t.Fatalf("manual work was queued: %+v", list)
	}
	// A person starting it takes over.
	if err := c.db.SettleAutoStart(ctx, c.project.ID, id, "사람이 직접 시작했습니다"); err != nil {
		t.Fatal(err)
	}
	if list, _ := c.db.AutoStarts(ctx, c.project.ID, storage.AutoStartFilter{}); list[0].Status != storage.AutoCancelled {
		t.Fatalf("after a person took over = %+v", list[0])
	}
}
