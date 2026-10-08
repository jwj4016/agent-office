package main

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"

	"github.com/zalando/go-keyring"
)

// e2eWait bounds waits in end-to-end binding tests. The real app keeps
// code in Git worktrees, and on some machines every git command is a
// slow process start.
const e2eWait = 2 * time.Minute

// startApp boots the real App (DB, engine, test provider) in a temp dir.
func startApp(t *testing.T) *App {
	t.Helper()
	keyring.MockInit()
	t.Setenv("AGENT_OFFICE_DATA_DIR", t.TempDir())
	a := NewApp()
	a.emit = func(context.Context, string, ...interface{}) {}
	a.startup(context.Background())
	if st := a.SystemStatus(); st.Error != "" {
		t.Fatal(st.Error)
	}
	t.Cleanup(func() { a.shutdown(context.Background()) })
	return a
}

// need returns v or fails the test by panicking with err (tests only).
func need[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// buildService sets up a service the way a person would through the UI:
// roles, assignments, the example workflow with every step assigned.
func buildService(t *testing.T, a *App, name string) (domain.Project, string) {
	t.Helper()
	conn := need(a.EnsureTestConnection())
	p := need(a.CreateProject(ProjectInput{Name: name, Goal: name + "를 만든다", Mode: "review"}))
	roleIDs := map[string]string{}
	roles := need(a.ListRoles())
	for _, r := range roles {
		roleIDs[r.Name] = r.ID
	}
	for _, name := range []string{"기획", "설계", "백엔드", "프론트엔드", "QA", "리뷰어"} {
		if roleIDs[name] == "" {
			roleIDs[name] = need(a.SaveRole(domain.Role{Name: name, Instructions: name + " 지침"})).ID
		}
	}
	assign := map[string]string{}
	for fixture, role := range map[string]string{"a-planner": "기획", "a-architect": "설계", "a-backend": "백엔드", "a-frontend": "프론트엔드", "a-qa": "QA"} {
		assign[fixture] = need(a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: roleIDs[role], ActorKind: domain.ActorAI,
			DisplayName: role + " AI", ConnectionID: conn.ID, Model: "auto"})).ID
	}
	assign["a-owner"] = need(a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: roleIDs["리뷰어"], ActorKind: domain.ActorHuman, DisplayName: "나"})).ID

	w := need(a.CreateWorkflow(p.ID, "service-dev", ""))
	// The template leaves assignees empty; fill them like the editor does.
	fixture := map[string]string{"plan": "a-planner", "approve": "a-owner", "design": "a-architect", "backend": "a-backend",
		"frontend": "a-frontend", "integrate": "a-architect", "review": "a-owner", "qa": "a-qa", "deliver": "a-planner"}
	var spec domain.WorkflowSpec
	json.Unmarshal(w.Draft, &spec)
	for i := range spec.Nodes {
		spec.Nodes[i].AssignmentID = assign[fixture[spec.Nodes[i].ID]]
	}
	draft, _ := json.Marshal(spec)
	w = need(a.SaveWorkflowDraft(p.ID, w.ID, w.Revision, string(draft)))
	res := need(a.ConfirmVersion(p.ID, w.ID, w.Revision))
	if res.Version == nil || !res.Validation.CanRun {
		t.Fatalf("confirm: %+v", res.Validation.Issues)
	}
	return p, res.Version.ID
}

func waitInbox(t *testing.T, a *App, projectID, kind string) engine.InboxItem {
	t.Helper()
	deadline := time.Now().Add(e2eWait)
	for time.Now().Before(deadline) {
		for _, it := range need(a.Inbox()) {
			if it.ProjectID == projectID && it.Kind == kind {
				return it
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("no %s item for %s", kind, projectID)
	return engine.InboxItem{}
}

// M1 gate through the UI-facing API: two services, manual setup, the
// owner approves and reviews, both finish, nothing crosses services.
func TestBindingsTwoServicesEndToEnd(t *testing.T) {
	a := startApp(t)
	game, gameVersion := buildService(t, a, "게임")
	estate, estateVersion := buildService(t, a, "부동산")

	gameRun := need(a.StartRun(game.ID, gameVersion)).RunID
	estateRun := need(a.StartRun(estate.ID, estateVersion)).RunID

	for _, p := range []domain.Project{game, estate} {
		ap := waitInbox(t, a, p.ID, engine.InboxApproval)
		need(a.DecideApproval(p.ID, ap.ApprovalID, ap.Generation, engine.ApprovalApproved, "진행", nil))
	}
	// Game: request a backend fix first, then pass.
	rv := waitInbox(t, a, game.ID, engine.InboxReview)
	if r := need(a.SubmitReview(game.ID, rv.AttemptID, rv.Generation, engine.ReviewChangesRequested, "테스트 보강", []string{"backend"}, nil)); r.Outcome.Status != engine.StSuperseded {
		t.Fatalf("changes requested: %+v", r)
	}
	for _, p := range []domain.Project{game, estate} {
		deadline := time.Now().Add(e2eWait)
		for {
			it := waitInbox(t, a, p.ID, engine.InboxReview)
			if p.ID != game.ID || it.Generation == rv.Generation+1 {
				need(a.SubmitReview(p.ID, it.AttemptID, it.Generation, engine.ReviewPass, "통과", nil, nil))
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("re-review never arrived")
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	for _, r := range []struct{ project, run string }{{game.ID, gameRun}, {estate.ID, estateRun}} {
		deadline := time.Now().Add(e2eWait)
		for need(a.GetRun(r.project, r.run)).Status != engine.RunSucceeded {
			if time.Now().After(deadline) {
				t.Fatalf("run %s did not finish", r.run)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}

	// Isolation (T07, T23): ids from one service are refused in the other.
	if _, err := a.GetRun(estate.ID, gameRun); err == nil || !strings.Contains(err.Error(), "찾을 수 없는") {
		t.Fatalf("cross-service read: %v", err)
	}
	gameDetail := need(a.GetRun(game.ID, gameRun))
	art := gameDetail.Steps[0].Artifacts[0]
	if _, err := a.ReadArtifact(estate.ID, art.ID); err == nil {
		t.Fatal("read another service's artifact")
	}
	if c := need(a.ReadArtifact(game.ID, art.ID)); !c.HashOK {
		t.Fatal("artifact hash mismatch")
	}
	if len(need(a.Inbox())) != 0 {
		t.Fatal("inbox not empty after both runs finished")
	}
	if evs := need(a.EventsAfter(estate.ID, 0)); len(evs) == 0 {
		t.Fatal("no events")
	} else {
		for _, ev := range evs {
			if ev.ProjectID != estate.ID {
				t.Fatalf("event from another service: %+v", ev)
			}
		}
	}
}

// T04 through the bindings: an unverified connection can be saved and
// versioned, but starting a run is refused with an explanation.
func TestBindingsStartBlockedWithoutConnection(t *testing.T) {
	a := startApp(t)
	p, versionID := buildService(t, a, "게임")
	_ = versionID
	real := need(a.db.SaveConnection(a.ctx, domain.ProviderConnection{Name: "codex", Provider: "codex", ExecutablePath: "/opt/codex"}))
	list := need(a.ListAssignments(p.ID))
	for _, as := range list {
		if as.ActorKind == domain.ActorAI {
			as.ConnectionID = real.ID
			need(a.SaveAssignment(as))
			break
		}
	}
	ws := need(a.ListWorkflows(p.ID))
	res := need(a.ConfirmVersion(p.ID, ws[0].ID, ws[0].Revision))
	if res.Version == nil || res.Validation.CanRun {
		t.Fatalf("confirm: %+v", res)
	}
	start := need(a.StartRun(p.ID, res.Version.ID))
	if start.RunID != "" || len(start.Issues) == 0 || !strings.Contains(start.Issues[0].Message, "연결") {
		t.Fatalf("start = %+v", start)
	}
}

func TestProjectWorkspacePath(t *testing.T) {
	a := startApp(t)
	p := need(a.CreateProject(ProjectInput{Name: "x", Mode: "review"}))
	dir := t.TempDir()
	in := ProjectInput{ID: p.ID, Name: "x", Mode: "review", WorkspacePath: dir}
	if got := need(a.UpdateProject(in)); got.WorkspacePath != dir {
		t.Fatalf("workspace = %q", got.WorkspacePath)
	}
	for _, bad := range []string{"relative/dir", filepath.Join(dir, "missing")} {
		in.WorkspacePath = bad
		if _, err := a.UpdateProject(in); err == nil {
			t.Errorf("accepted workspace %q", bad)
		}
	}
}
