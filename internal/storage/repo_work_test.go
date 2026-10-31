package storage_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

// company is a service with marketing, development and finance
// departments, each with one AI member who leads it.
type company struct {
	db                        *storage.DB
	project                   domain.Project
	mktRole, devRole, finRole domain.Role
	mktAsg, devAsg, finAsg    domain.Assignment
	mkt, dev, fin             domain.Department
}

func newCompany(t *testing.T) *company {
	t.Helper()
	db := testenv.OpenDB(t)
	ctx := context.Background()
	c := &company{db: db}
	role := func(name string) domain.Role {
		r, err := db.SaveRole(ctx, domain.Role{Name: name})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	c.mktRole, c.devRole, c.finRole = role("마케터"), role("개발자"), role("재무 담당")
	p, err := db.CreateProject(ctx, domain.Project{Name: "캠페인"})
	if err != nil {
		t.Fatal(err)
	}
	c.project = p
	asg := func(r domain.Role) domain.Assignment {
		a, err := db.SaveAssignment(ctx, domain.Assignment{ProjectID: p.ID, RoleID: r.ID, ActorKind: domain.ActorAI, DisplayName: r.Name + " AI"})
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	c.mktAsg, c.devAsg, c.finAsg = asg(c.mktRole), asg(c.devRole), asg(c.finRole)
	dept := func(name string, a domain.Assignment, resp string) domain.Department {
		d, err := db.SaveDepartment(ctx, domain.Department{ProjectID: p.ID, Name: name, Responsibility: resp,
			MemberIDs: []string{a.ID}, LeadAssignmentID: a.ID})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	c.mkt = dept("마케팅팀", c.mktAsg, "캠페인 기획")
	c.dev = dept("개발팀", c.devAsg, "구현 검토")
	c.fin = dept("재무팀", c.finAsg, "비용 검토")
	return c
}

func TestDepartmentsBelongToTheirService(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	other, _ := c.db.CreateProject(ctx, domain.Project{Name: "다른 회사"})
	// The same name is fine in another service, not twice in one.
	if _, err := c.db.SaveDepartment(ctx, domain.Department{ProjectID: other.ID, Name: "개발팀"}); err != nil {
		t.Fatalf("same name in another service: %v", err)
	}
	if _, err := c.db.SaveDepartment(ctx, domain.Department{ProjectID: c.project.ID, Name: "개발팀"}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("duplicate name: %v", err)
	}
	// Members and the lead come from this service only.
	if _, err := c.db.SaveDepartment(ctx, domain.Department{ProjectID: other.ID, Name: "x", MemberIDs: []string{c.devAsg.ID}}); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("another service's member: %v", err)
	}
	if _, err := c.db.Department(ctx, other.ID, c.dev.ID); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("read across services: %v", err)
	}
	list, _ := c.db.Departments(ctx, c.project.ID)
	if len(list) != 3 || list[0].Name != "개발팀" || list[0].LeadAssignmentID != c.devAsg.ID || len(list[0].MemberIDs) != 1 {
		t.Fatalf("list = %+v", list)
	}
}

func TestDepartmentMembership(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	// The lead must be a member.
	d := c.fin
	d.LeadAssignmentID = c.devAsg.ID
	if _, err := c.db.SaveDepartment(ctx, d); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("lead outside the department: %v", err)
	}
	// Moving the developer into finance takes them out of development and
	// development loses its lead.
	d.MemberIDs = []string{c.finAsg.ID, c.devAsg.ID}
	d.LeadAssignmentID = c.finAsg.ID
	if _, err := c.db.SaveDepartment(ctx, d); err != nil {
		t.Fatal(err)
	}
	dev, _ := c.db.Department(ctx, c.project.ID, c.dev.ID)
	if len(dev.MemberIDs) != 0 || dev.LeadAssignmentID != "" {
		t.Fatalf("development after move: %+v", dev)
	}
	// Saving without a member list keeps the members.
	got, err := c.db.SaveDepartment(ctx, domain.Department{ID: c.fin.ID, ProjectID: c.project.ID, Name: "재무팀", Responsibility: "돈", LeadAssignmentID: c.finAsg.ID})
	if err != nil || len(got.MemberIDs) != 2 || got.Responsibility != "돈" {
		t.Fatalf("after edit: %+v %v", got, err)
	}
}

func TestDepartmentTemplates(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	designer, _ := c.db.SaveRole(ctx, domain.Role{Name: "디자이너"})
	tpl, err := c.db.SaveDepartmentTemplate(ctx, domain.DepartmentTemplate{Name: "디자인팀", Responsibility: "화면과 브랜드",
		RoleIDs: []string{designer.ID, c.devRole.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.SaveDepartmentTemplate(ctx, domain.DepartmentTemplate{Name: "디자인팀"}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("duplicate template: %v", err)
	}
	// Designers join the service; the developer is already in a department.
	des, _ := c.db.SaveAssignment(ctx, domain.Assignment{ProjectID: c.project.ID, RoleID: designer.ID, ActorKind: domain.ActorAI, DisplayName: "디자인 AI"})
	me, _ := c.db.SaveAssignment(ctx, domain.Assignment{ProjectID: c.project.ID, RoleID: designer.ID, ActorKind: domain.ActorHuman, DisplayName: "가 나"})
	got, err := c.db.ImportDepartmentTemplate(ctx, c.project.ID, tpl.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "디자인팀" || got.Responsibility != "화면과 브랜드" || got.TemplateID != tpl.ID ||
		strings.Join(got.MemberIDs, ",") != me.ID+","+des.ID || got.LeadAssignmentID != des.ID {
		t.Fatalf("imported = %+v (designer %s, me %s)", got, des.ID, me.ID)
	}
	// Importing again makes a second, numbered copy with no free members.
	again, err := c.db.ImportDepartmentTemplate(ctx, c.project.ID, tpl.ID)
	if err != nil || again.Name != "디자인팀 2" || len(again.MemberIDs) != 0 || again.LeadAssignmentID != "" {
		t.Fatalf("second import = %+v %v", again, err)
	}
	// Editing the template does not change the copies; deleting it only
	// drops the link.
	tpl.Responsibility = "바뀐 책임"
	c.db.SaveDepartmentTemplate(ctx, tpl)
	if err := c.db.DeleteDepartmentTemplate(ctx, tpl.ID); err != nil {
		t.Fatal(err)
	}
	after, _ := c.db.Department(ctx, c.project.ID, got.ID)
	if after.Responsibility != "화면과 브랜드" || after.TemplateID != "" {
		t.Fatalf("copy after template change: %+v", after)
	}
	// A service department can become a template with its members' roles.
	saved, err := c.db.SaveDepartmentAsTemplate(ctx, c.project.ID, c.dev.ID, "개발팀 표준")
	if err != nil || saved.Responsibility != "구현 검토" || strings.Join(saved.RoleIDs, ",") != c.devRole.ID {
		t.Fatalf("saved as template = %+v %v", saved, err)
	}
}

func TestDeletingRoleLeavesTemplate(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	r, _ := db.SaveRole(ctx, domain.Role{Name: "임시"})
	tpl, err := db.SaveDepartmentTemplate(ctx, domain.DepartmentTemplate{Name: "임시팀", RoleIDs: []string{r.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.DeleteRole(ctx, r.ID); err != nil {
		t.Fatalf("delete role in a template: %v", err)
	}
	if got, _ := db.DepartmentTemplate(ctx, tpl.ID); len(got.RoleIDs) != 0 {
		t.Fatalf("roles = %v", got.RoleIDs)
	}
}

func TestInstructionLayersIncludeDepartment(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	layers, err := c.db.InstructionLayers(ctx, c.project.ID, c.devAsg.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for _, l := range layers {
		sources = append(sources, l.Source)
	}
	if got := strings.Join(sources, ","); got != "organization,role,project,department" {
		t.Fatalf("sources = %s", got)
	}
	if l := layers[3]; l.Name != "부서: 개발팀" || l.Text != "구현 검토" {
		t.Fatalf("department layer = %+v", l)
	}
	// An inactive department adds no layer.
	d := c.dev
	d.Status = domain.StatusInactive
	if _, err := c.db.SaveDepartment(ctx, d); err != nil {
		t.Fatal(err)
	}
	layers, _ = c.db.InstructionLayers(ctx, c.project.ID, c.devAsg.ID)
	for _, l := range layers {
		if l.Source == "department" {
			t.Fatal("inactive department still adds a layer")
		}
	}
}

func TestProjectCategories(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	cats, err := c.db.ProjectCategories(ctx, c.project.ID)
	if err != nil || strings.Join(cats, ",") != strings.Join(domain.DefaultCategories, ",") {
		t.Fatalf("defaults = %v %v", cats, err)
	}
	cats, err = c.db.SetProjectCategories(ctx, c.project.ID, []string{"개발", " 광고 ", "개발", ""})
	if err != nil || strings.Join(cats, ",") != "개발,광고" {
		t.Fatalf("set = %v %v", cats, err)
	}
}

func TestWorkItemsUnderGoal(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	pid := c.project.ID
	if _, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, Kind: domain.WorkGoal, Title: "신규 고객 캠페인 준비"}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("goal without department: %v", err)
	}
	goal, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, Kind: domain.WorkGoal, Title: "신규 고객 캠페인 준비", DepartmentID: c.mkt.ID})
	if err != nil {
		t.Fatal(err)
	}
	mk := func(d domain.Department, title, priority, due string, cats ...string) domain.WorkItem {
		w, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, ParentID: goal.ID, DepartmentID: d.ID, Title: title,
			Priority: priority, DueAt: due, Categories: cats, DoneCriteria: "결과를 공유한다"})
		if err != nil {
			t.Fatal(err)
		}
		return w
	}
	plan := mk(c.mkt, "캠페인 기획안 작성", "high", "2026-11-01", "기획")
	devWork := mk(c.dev, "캠페인 페이지 구현 검토", "normal", "", "개발")
	finWork := mk(c.fin, "캠페인 예산 검토", "urgent", "2026-10-30", "비용", "기획")
	if plan.Status != domain.WorkReceived || plan.Kind != domain.WorkTask || plan.ParentID != goal.ID {
		t.Fatalf("plan = %+v", plan)
	}

	// Work cannot hang under ordinary work or a goal under anything.
	if _, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, ParentID: plan.ID, Title: "하위"}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("parent is not a goal: %v", err)
	}
	if _, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, Kind: domain.WorkGoal, ParentID: goal.ID, DepartmentID: c.mkt.ID, Title: "x"}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("nested goal: %v", err)
	}
	if _, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, Title: "x", Priority: "asap"}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("bad priority: %v", err)
	}
	if _, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, Title: "x", DueAt: "11/01"}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("bad due date: %v", err)
	}

	titles := func(list []domain.WorkItem) string {
		var out []string
		for _, w := range list {
			out = append(out, w.Title)
		}
		return strings.Join(out, "|")
	}
	children, _ := c.db.WorkItems(ctx, pid, storage.WorkFilter{ParentID: goal.ID})
	if got := titles(children); got != "캠페인 예산 검토|캠페인 기획안 작성|캠페인 페이지 구현 검토" {
		t.Fatalf("children by priority = %s", got)
	}
	byDept, _ := c.db.WorkItems(ctx, pid, storage.WorkFilter{DepartmentID: c.dev.ID})
	byCat, _ := c.db.WorkItems(ctx, pid, storage.WorkFilter{Category: "기획"})
	due, _ := c.db.WorkItems(ctx, pid, storage.WorkFilter{DueBefore: "2026-10-31"})
	goals, _ := c.db.WorkItems(ctx, pid, storage.WorkFilter{Kind: domain.WorkGoal})
	if titles(byDept) != devWork.Title || titles(byCat) != finWork.Title+"|"+plan.Title || titles(due) != finWork.Title || titles(goals) != goal.Title {
		t.Fatalf("filters: dept=%s cat=%s due=%s goals=%s", titles(byDept), titles(byCat), titles(due), titles(goals))
	}

	// Edits keep the kind and status.
	devWork.Description = "랜딩 페이지 범위"
	devWork.Kind = domain.WorkGoal
	devWork.Status = domain.WorkDone
	got, err := c.db.UpdateWorkItem(ctx, devWork)
	if err != nil || got.Kind != domain.WorkTask || got.Status != domain.WorkReceived || got.Description != "랜딩 페이지 범위" {
		t.Fatalf("update = %+v %v", got, err)
	}
}

func TestWorkStatusHistory(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	pid := c.project.ID
	w, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, DepartmentID: c.dev.ID, Title: "구현 검토"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.SetWorkStatus(ctx, pid, w.ID, domain.WorkBlocked, " "); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("blocked without reason: %v", err)
	}
	step := func(status, reason string) domain.WorkItem {
		t.Helper()
		got, err := c.db.SetWorkStatus(ctx, pid, w.ID, status, reason)
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	if got := step(domain.WorkWaiting, "기획안 초안을 기다림"); got.StatusReason != "기획안 초안을 기다림" || got.ClosedAt != "" {
		t.Fatalf("waiting = %+v", got)
	}
	step(domain.WorkInProgress, "")
	done := step(domain.WorkDone, "검토 완료")
	if done.ClosedAt == "" || done.StatusReason != "" {
		t.Fatalf("done = %+v", done)
	}
	if _, err := c.db.AddWorkNote(ctx, pid, w.ID, "결과는 공유 자료에 있음"); err != nil {
		t.Fatal(err)
	}
	reopened := step(domain.WorkInProgress, "기획안이 바뀜")
	if reopened.ClosedAt != "" {
		t.Fatalf("reopened still closed: %+v", reopened)
	}
	notes, err := c.db.WorkNotes(ctx, pid, w.ID)
	if err != nil {
		t.Fatal(err)
	}
	var trail []string
	for _, n := range notes {
		if n.Kind == domain.NoteStatus {
			trail = append(trail, n.FromStatus+">"+n.ToStatus)
		} else {
			trail = append(trail, "note:"+n.Body)
		}
	}
	want := ">received|received>waiting|waiting>in_progress|in_progress>done|note:결과는 공유 자료에 있음|done>in_progress"
	if got := strings.Join(trail, "|"); got != want {
		t.Fatalf("history =\n%s\nwant\n%s", got, want)
	}
}

func TestWorkScopedToService(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	other, _ := c.db.CreateProject(ctx, domain.Project{Name: "다른 서비스"})
	w, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, Title: "내 업무"})
	if _, err := c.db.WorkItem(ctx, other.ID, w.ID); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("read from another service: %v", err)
	}
	if _, err := c.db.SetWorkStatus(ctx, other.ID, w.ID, domain.WorkDone, ""); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("status from another service: %v", err)
	}
	if _, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: other.ID, Title: "x", DepartmentID: c.dev.ID}); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("another service's department: %v", err)
	}
	if _, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: other.ID, Title: "x", AssigneeAssignmentID: c.devAsg.ID}); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("another service's assignee: %v", err)
	}
	if err := c.db.CheckScope(ctx, other.ID, storage.Ref{Kind: storage.RefWorkItem, ID: w.ID}); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("scope: %v", err)
	}
	if err := c.db.CheckScope(ctx, c.project.ID, storage.Ref{Kind: storage.RefDepartment, ID: c.dev.ID}, storage.Ref{Kind: storage.RefWorkItem, ID: w.ID}); err != nil {
		t.Fatalf("own scope: %v", err)
	}
	// Archived services keep their work but take no changes.
	if err := c.db.SetProjectArchived(ctx, c.project.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.SetWorkStatus(ctx, c.project.ID, w.ID, domain.WorkDone, ""); !errors.Is(err, storage.ErrArchived) {
		t.Fatalf("archived change: %v", err)
	}
	if _, err := c.db.SaveDepartment(ctx, domain.Department{ProjectID: c.project.ID, Name: "새 부서"}); !errors.Is(err, storage.ErrArchived) {
		t.Fatalf("archived department: %v", err)
	}
	if _, err := c.db.WorkItem(ctx, c.project.ID, w.ID); err != nil {
		t.Fatalf("archived read: %v", err)
	}
}

func TestInactiveDepartmentKeepsExistingWork(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	pid := c.project.ID
	w, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, DepartmentID: c.fin.ID, Title: "예산 검토"})
	d := c.fin
	d.Status = domain.StatusInactive
	if _, err := c.db.SaveDepartment(ctx, d); err != nil {
		t.Fatal(err)
	}
	if _, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: pid, DepartmentID: c.fin.ID, Title: "새 업무"}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("new work for inactive department: %v", err)
	}
	w.Title = "예산 검토(수정)"
	if _, err := c.db.UpdateWorkItem(ctx, w); err != nil {
		t.Fatalf("editing existing work of inactive department: %v", err)
	}
}
