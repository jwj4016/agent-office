package storage_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

func TestRoleHierarchyRejectsCycles(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	cto, _ := db.SaveRole(ctx, domain.Role{Name: "CTO"})
	dev, _ := db.SaveRole(ctx, domain.Role{Name: "개발자", ParentRoleID: cto.ID})
	be, err := db.SaveRole(ctx, domain.Role{Name: "백엔드", ParentRoleID: dev.ID})
	if err != nil {
		t.Fatal(err)
	}
	cto.ParentRoleID = be.ID
	if _, err := db.SaveRole(ctx, cto); !errors.Is(err, storage.ErrRoleCycle) {
		t.Fatalf("cycle accepted: %v", err)
	}
	if _, err := db.SaveRole(ctx, domain.Role{Name: "x", ParentRoleID: "role-nope"}); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("unknown parent accepted: %v", err)
	}
	// Deleting the middle role re-parents its child.
	if err := db.DeleteRole(ctx, dev.ID); err != nil {
		t.Fatal(err)
	}
	if got, _ := db.Role(ctx, be.ID); got.ParentRoleID != cto.ID {
		t.Fatalf("child not re-parented: %+v", got)
	}
}

func TestInstructionLayersOrder(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	db.UpdateOrganization(ctx, "우리 회사", "회사 지침")
	parent, _ := db.SaveRole(ctx, domain.Role{Name: "개발", Instructions: "개발 공통"})
	child, _ := db.SaveRole(ctx, domain.Role{Name: "백엔드", ParentRoleID: parent.ID, Mission: "API", Instructions: "백엔드 지침"})
	p, _ := db.CreateProject(ctx, domain.Project{Name: "게임", Instructions: "게임 지침"})
	a, err := db.SaveAssignment(ctx, domain.Assignment{ProjectID: p.ID, RoleID: child.ID, ActorKind: domain.ActorAI, DisplayName: "BE",
		Overrides: domain.AssignmentOverrides{Instructions: "이번 서비스 추가 지침"}})
	if err != nil {
		t.Fatal(err)
	}
	layers, err := db.InstructionLayers(ctx, p.ID, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, l := range layers {
		got = append(got, l.Source+":"+l.Text)
	}
	want := "organization:회사 지침|role:개발 공통|role:책임: API\n백엔드 지침|project:게임 지침|assignment:이번 서비스 추가 지침"
	if strings.Join(got, "|") != want {
		t.Fatalf("layers =\n%s\nwant\n%s", strings.Join(got, "|"), want)
	}
	if text := domain.ComposeInstructions(layers); !strings.HasPrefix(text, "## 회사: 우리 회사\n회사 지침") {
		t.Fatalf("composed = %q", text)
	}
}

func TestHumanAssignmentIsLocalOwner(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	r, _ := db.SaveRole(ctx, domain.Role{Name: "리뷰어"})
	p, _ := db.CreateProject(ctx, domain.Project{Name: "x"})
	a, err := db.SaveAssignment(ctx, domain.Assignment{ProjectID: p.ID, RoleID: r.ID, ActorKind: domain.ActorHuman, DisplayName: "나", ConnectionID: "ignored", Model: "ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if a.ActorID != domain.LocalOwner || a.ConnectionID != "" || a.Model != "" {
		t.Fatalf("human assignment = %+v", a)
	}
}

func TestAssignmentCannotMoveAcrossProjects(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	game := testenv.ServiceDev(t, db, "게임")
	estate := testenv.ServiceDev(t, db, "부동산")
	assignments, _ := db.Assignments(ctx, game.ID)
	a := assignments[0]
	a.ProjectID = estate.ID // try to re-home game's assignment via update
	if _, err := db.SaveAssignment(ctx, a); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("cross-project update accepted: %v", err)
	}
}

func TestDraftSaveDetectsConflicts(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	p := testenv.ServiceDev(t, db, "게임")
	w, _ := db.Workflow(ctx, p.ID, p.WorkflowID)
	if _, err := db.SaveWorkflowDraft(ctx, p.ID, w.ID, w.Revision, w.Draft); err != nil {
		t.Fatal(err)
	}
	if _, err := db.SaveWorkflowDraft(ctx, p.ID, w.ID, w.Revision, w.Draft); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("stale revision accepted: %v", err)
	}
	// Half-finished drafts still save; validation reports the problems.
	half := json.RawMessage(`{"schemaVersion":1,"title":"작성 중","nodes":[{"id":"a","title":"A","kind":"task","dependsOn":["b"]}]}`)
	if _, err := db.SaveWorkflowDraft(ctx, p.ID, w.ID, w.Revision+1, half); err != nil {
		t.Fatalf("draft save rejected: %v", err)
	}
	res, _ := db.ValidateWorkflow(ctx, p.ID, w.ID)
	if res.CanVersion || len(res.Issues) == 0 {
		t.Fatalf("invalid draft passes: %+v", res)
	}
}

// T13: a confirmed version is frozen. Editing the draft, a shared role or
// an assignment afterwards must not change it.
func TestVersionSnapshotIsFrozen(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	p := testenv.ServiceDev(t, db, "게임")
	w, _ := db.Workflow(ctx, p.ID, p.WorkflowID)
	v1, res, err := db.ConfirmVersion(ctx, p.ID, w.ID, w.Revision)
	if err != nil {
		t.Fatalf("confirm: %v %+v", err, res.Issues)
	}
	if !res.CanRun || v1.Number != 1 || len(v1.Spec.Nodes) != 9 || len(v1.Assignments) != 6 {
		t.Fatalf("version = %+v res = %+v", v1, res)
	}
	planner := v1.Assignments[p.Assignments["a-planner"]]
	if planner.RoleName != "기획" || !strings.Contains(domain.ComposeInstructions(planner.Instructions), "기획 지침") {
		t.Fatalf("snapshot instructions = %+v", planner)
	}

	// Change everything the version depends on.
	edited := strings.Replace(string(w.Draft), `"title": "서비스 개발"`, `"title": "바뀐 흐름"`, 1)
	if _, err := db.SaveWorkflowDraft(ctx, p.ID, w.ID, w.Revision, json.RawMessage(edited)); err != nil {
		t.Fatal(err)
	}
	role, _ := db.Role(ctx, planner.RoleID)
	role.Instructions = "완전히 새로운 지침"
	db.SaveRole(ctx, role)
	a := planner.Assignment
	a.DisplayName = "새 이름"
	db.SaveAssignment(ctx, a)

	again, err := db.WorkflowVersion(ctx, p.ID, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	snap := again.Assignments[a.ID]
	if again.Spec.Title != "서비스 개발" || snap.DisplayName != "기획 AI" ||
		strings.Contains(domain.ComposeInstructions(snap.Instructions), "완전히 새로운") {
		t.Fatalf("version changed after edits: title=%q name=%q", again.Spec.Title, snap.DisplayName)
	}

	// A new version picks the changes up and gets the next number.
	cur, _ := db.Workflow(ctx, p.ID, w.ID)
	v2, _, err := db.ConfirmVersion(ctx, p.ID, w.ID, cur.Revision)
	if err != nil || v2.Number != 2 || v2.Spec.Title != "바뀐 흐름" ||
		!strings.Contains(domain.ComposeInstructions(v2.Assignments[a.ID].Instructions), "완전히 새로운") {
		t.Fatalf("v2 = %+v, %v", v2, err)
	}
	// Confirming from a stale editor view is rejected.
	if _, _, err := db.ConfirmVersion(ctx, p.ID, w.ID, w.Revision); !errors.Is(err, storage.ErrConflict) {
		t.Fatalf("stale confirm accepted: %v", err)
	}
}

func TestConfirmRejectsInvalidAndAllowsUnconnected(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	p := testenv.ServiceDev(t, db, "게임")

	// T04: an AI assignment without a verified connection: version ok, run blocked.
	real, _ := db.SaveConnection(ctx, domain.ProviderConnection{Name: "codex", Provider: "codex", ExecutablePath: "/usr/bin/codex"})
	list, _ := db.Assignments(ctx, p.ID)
	for _, a := range list {
		if a.ID == p.Assignments["a-qa"] {
			a.ConnectionID = real.ID
			db.SaveAssignment(ctx, a)
		}
	}
	w, _ := db.Workflow(ctx, p.ID, p.WorkflowID)
	v, res, err := db.ConfirmVersion(ctx, p.ID, w.ID, w.Revision)
	if err != nil || res.CanRun || v.Assignments[p.Assignments["a-qa"]].Connected {
		t.Fatalf("unconnected AI: err=%v canRun=%v", err, res.CanRun)
	}

	// Structural errors block the version entirely.
	broken := strings.Replace(string(w.Draft), `"dependsOn": ["qa"]`, `"dependsOn": ["nope"]`, 1)
	w, _ = db.SaveWorkflowDraft(ctx, p.ID, w.ID, w.Revision, json.RawMessage(broken))
	_, _, err = db.ConfirmVersion(ctx, p.ID, w.ID, w.Revision)
	var nv *storage.ErrNotVersionable
	if !errors.As(err, &nv) || len(nv.Issues) == 0 {
		t.Fatalf("broken workflow versioned: %v", err)
	}
}

func TestArchivedProjectIsReadOnly(t *testing.T) {
	db := testenv.OpenDB(t)
	ctx := context.Background()
	p := testenv.ServiceDev(t, db, "게임")
	if err := db.SetProjectArchived(ctx, p.ID, true); err != nil {
		t.Fatal(err)
	}
	if list, _ := db.Projects(ctx, false); len(list) != 0 {
		t.Fatal("archived project listed")
	}
	if _, err := db.CreateWorkflow(ctx, p.ID, json.RawMessage(`{"title":"x"}`)); !errors.Is(err, storage.ErrArchived) {
		t.Fatalf("write to archived project: %v", err)
	}
	if err := db.SetProjectArchived(ctx, p.ID, false); err != nil {
		t.Fatal(err)
	}
}
