package design_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

type env struct {
	t    *testing.T
	ctx  context.Context
	db   *storage.DB
	eng  *engine.Engine
	svc  *design.Service
	prov *providers.TestProvider
	conn domain.ProviderConnection
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testenv.OpenDB(t)
	ctx := context.Background()
	conn, err := db.SaveConnection(ctx, domain.ProviderConnection{Name: "테스트", Provider: "test"})
	if err != nil {
		t.Fatal(err)
	}
	prov := &providers.TestProvider{Scripts: map[string][]providers.Step{}}
	eng := engine.New(engine.Config{DB: db, DataDir: t.TempDir(), Logf: t.Logf,
		Providers: func(domain.ProviderConnection) (providers.Provider, error) { return prov, nil }})
	ectx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { eng.Run(ectx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })
	db.SaveRole(ctx, domain.Role{Name: "기획", Instructions: "기존 기획 지침"})
	e := &env{t: t, ctx: ctx, db: db, eng: eng, prov: prov, conn: conn}
	e.svc = &design.Service{DB: db, Engine: eng, Logf: t.Logf,
		Providers: func(domain.ProviderConnection) (providers.Provider, error) { return prov, nil },
		Connections: func(context.Context) ([]design.ConnectionInfo, error) {
			return []design.ConnectionInfo{{ID: conn.ID, Name: conn.Name, Provider: "test", Usable: true, Coding: true}}, nil
		},
	}
	return e
}

// designer installs a test scenario that writes the given proposal.
func (e *env) designer(name string, edit func(string) string) {
	data, err := os.ReadFile(filepath.Join(testenv.RepoRoot(), "tests", "fixtures", "design", "game-launch.json"))
	if err != nil {
		e.t.Fatal(err)
	}
	s := strings.NewReplacer("CONN_AI", e.conn.ID, "CONN_CODE", e.conn.ID).Replace(string(data))
	if edit != nil {
		s = edit(s)
	}
	e.prov.Scripts[name] = []providers.Step{
		{Write: &providers.WriteStep{Key: "design", Content: s}},
		{Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded}},
	}
}

func (e *env) draft(in design.StoredInput) storage.DesignRecord {
	e.t.Helper()
	rec, err := e.svc.Start(e.ctx, in, e.conn.ID)
	if err != nil {
		e.t.Fatal(err)
	}
	e.svc.Wait()
	rec, _ = e.db.Design(e.ctx, rec.ID)
	return rec
}

func ptr[T any](v T) *T { return &v }

func gameRequest(mode string) design.StoredInput {
	return design.StoredInput{Request: design.Request{Goal: "작은 웹 게임을 출시하고 싶어. 코드 리뷰는 내가 할게.", HumanTasks: []string{"코드 리뷰"}, Mode: mode}, Model: "designer"}
}

// T02: "게임 출시, 리뷰는 내가" → roles, assignments and workflow, with the
// review assigned to local-owner; review mode leaves it for editing.
func TestDesignReviewModeNewService(t *testing.T) {
	e := newEnv(t)
	e.designer("designer", nil)
	rec := e.draft(gameRequest("review"))
	if rec.Status != design.StatusReady {
		t.Fatalf("status = %s error = %s", rec.Status, rec.Error)
	}
	out, err := e.svc.Apply(e.ctx, rec.ID, false)
	if err != nil || out.Applied == nil || len(out.Issues) != 0 || out.RunID != "" {
		t.Fatalf("apply: %+v %v", out, err)
	}
	p, _ := e.db.Project(e.ctx, out.Applied.ProjectID)
	if p.Name != "작은 웹 게임" || p.Mode != domain.ModeReview {
		t.Fatalf("project = %+v", p)
	}
	roles, _ := e.db.Roles(e.ctx)
	plans := 0
	for _, r := range roles {
		if r.Name == "기획" {
			plans++
		}
	}
	if plans != 1 || len(roles) != 6 {
		t.Fatalf("roles = %d (기획 %d)", len(roles), plans)
	}
	list, _ := e.db.Assignments(e.ctx, p.ID)
	var owner domain.Assignment
	for _, a := range list {
		if a.ActorKind == domain.ActorHuman {
			owner = a
		}
	}
	if owner.ActorID != domain.LocalOwner {
		t.Fatalf("human assignment = %+v", owner)
	}
	w, _ := e.db.Workflow(e.ctx, p.ID, out.Applied.WorkflowID)
	spec, _ := domain.ParseWorkflow(w.Draft)
	if spec.Node("review").AssignmentID != owner.ID {
		t.Fatal("code review not assigned to the person")
	}
	// The draft is a normal workflow: it versions and can run.
	v, res, err := e.db.ConfirmVersion(e.ctx, p.ID, w.ID, w.Revision)
	if err != nil || !res.CanRun || v.Number != 1 {
		t.Fatalf("confirm: %v %+v", err, res.Issues)
	}
	// Applying twice is refused.
	if _, err := e.svc.Apply(e.ctx, rec.ID, false); err == nil {
		t.Fatal("design applied twice")
	}
}

// T03: auto mode inside the pre-approved scope applies, versions and
// starts, but still waits for the person's approvals.
func TestDesignAutoModeStartsAndWaitsForPeople(t *testing.T) {
	e := newEnv(t)
	// The test provider needs a scenario name for the AI steps.
	e.designer("designer", func(s string) string { return strings.ReplaceAll(s, `"model": ""`, `"model": "auto"`) })
	in := gameRequest("auto")
	in.Model = "designer"
	in.AutoPolicy = design.AutoPolicy{AllowedConnectionIDs: []string{e.conn.ID}}
	in.Budget = engine.Budget{MaxTokens: ptr(int64(100000))}
	rec := e.draft(in)
	out, err := e.svc.Apply(e.ctx, rec.ID, true)
	if err != nil || out.RunID == "" || out.Version != 1 {
		t.Fatalf("auto apply: %+v %v", out, err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		d, _ := e.eng.RunDetail(e.ctx, out.Applied.ProjectID, out.RunID)
		if d.Step("direction").Status == engine.StWaitingApproval {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("run did not reach the human approval: %+v", d.Steps)
		}
		time.Sleep(20 * time.Millisecond)
	}
	d, _ := e.eng.RunDetail(e.ctx, out.Applied.ProjectID, out.RunID)
	if d.Status != engine.RunWaiting || d.Step("dev").Status != engine.StPending {
		t.Fatalf("auto mode skipped the approval: %s", d.Status)
	}
}

func TestDesignAutoModeRefusedOutsideScope(t *testing.T) {
	cases := map[string]func(*design.StoredInput, string){
		"no budget":              func(in *design.StoredInput, conn string) { in.AutoPolicy.AllowedConnectionIDs = []string{conn} },
		"connection not allowed": func(in *design.StoredInput, conn string) { in.Budget.MaxTokens = ptr(int64(1000)) },
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			e := newEnv(t)
			e.designer("designer", nil)
			in := gameRequest("auto")
			setup(&in, e.conn.ID)
			rec := e.draft(in)
			out, err := e.svc.Apply(e.ctx, rec.ID, true)
			if err != nil || out.Applied != nil || len(out.Issues) == 0 {
				t.Fatalf("auto outside scope: %+v %v", out, err)
			}
			if ps, _ := e.db.Projects(e.ctx, true); len(ps) != 0 {
				t.Fatal("something was created although auto was refused")
			}
			// The same proposal can still be applied for review.
			if out, err := e.svc.Apply(e.ctx, rec.ID, false); err != nil || out.Applied == nil {
				t.Fatalf("review apply after refusal: %+v %v", out, err)
			}
		})
	}
}

func TestDesignFailures(t *testing.T) {
	e := newEnv(t)
	e.designer("tools", func(s string) string { return strings.Replace(s, `"notes":`, `"tools": ["Bash"], "notes":`, 1) })
	e.designer("review-to-ai", func(s string) string {
		return strings.Replace(s, `"kind": "review", "assignmentId": "a-owner"`, `"kind": "review", "assignmentId": "a-qa"`, 1)
	})
	e.prov.Scripts["garbage"] = []providers.Step{{Write: &providers.WriteStep{Key: "design", Content: "not json"}}, {Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded}}}

	for _, model := range []string{"tools", "garbage"} {
		in := gameRequest("review")
		in.Model = model
		if rec := e.draft(in); rec.Status != design.StatusFailed || rec.Error == "" {
			t.Errorf("%s: status %s", model, rec.Status)
		}
	}
	in := gameRequest("review")
	in.Model = "review-to-ai"
	rec := e.draft(in)
	out, err := e.svc.Apply(e.ctx, rec.ID, false)
	if err != nil || out.Applied != nil || len(out.Issues) == 0 {
		t.Fatalf("proposal ignoring the person's review applied: %+v %v", out, err)
	}
}

func TestDesignIntoExistingService(t *testing.T) {
	e := newEnv(t)
	e.designer("designer", nil)
	p, _ := e.db.CreateProject(e.ctx, domain.Project{Name: "기존 게임", Goal: "업데이트"})
	in := gameRequest("review")
	in.ProjectID = p.ID
	rec := e.draft(in)
	out, err := e.svc.Apply(e.ctx, rec.ID, false)
	if err != nil || out.Applied == nil || out.Applied.ProjectID != p.ID || out.Applied.NewProject {
		t.Fatalf("apply into existing: %+v %v", out, err)
	}
	if ps, _ := e.db.Projects(e.ctx, true); len(ps) != 1 {
		t.Fatalf("projects = %d", len(ps))
	}
}
