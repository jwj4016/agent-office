package engine_test

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

type harness struct {
	t    *testing.T
	ctx  context.Context
	db   *storage.DB
	eng  *engine.Engine
	prov *providers.TestProvider

	mu       sync.Mutex
	override map[string]string // step id -> scenario, applied at start time
	starts   []providers.StartRequest
	// consultScenario, if set, is used by sessions answering another
	// AI's question.
	consultScenario string

	// stopEngine stops the harness engine (as if the app had quit).
	stopEngine func()
}

// started returns the start requests seen for a step, oldest first.
func (h *harness) started(step string) []providers.StartRequest {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []providers.StartRequest
	for _, r := range h.starts {
		if r.StepID == step {
			out = append(out, r)
		}
	}
	return out
}

// scenarioProvider lets a test switch a step's scenario mid-run (the
// frozen version keeps the original model name).
type scenarioProvider struct {
	*providers.TestProvider
	h *harness
}

func (p scenarioProvider) Start(ctx context.Context, req providers.StartRequest) (providers.Session, error) {
	p.h.mu.Lock()
	if s, ok := p.h.override[req.StepID]; ok {
		req.Model = s
	}
	if p.h.consultScenario != "" && strings.HasPrefix(req.Prompt, "# 다른 담당자의 질문에 답하기") {
		req.Model = p.h.consultScenario
	}
	p.h.starts = append(p.h.starts, req)
	p.h.mu.Unlock()
	return p.TestProvider.Start(ctx, req)
}

// useScenario overrides the scenario for a step from now on.
func (h *harness) useScenario(step, scenario string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.override[step] = scenario
}

func newHarness(t *testing.T, mutate ...func(*engine.Config)) *harness {
	t.Helper()
	db := testenv.OpenDB(t)
	prov, err := providers.LoadTestProvider(filepath.Join(testenv.RepoRoot(), "tests", "fixtures", "providers"))
	if err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t, ctx: context.Background(), db: db, prov: prov, override: map[string]string{}}
	cfg := engine.Config{
		DB: db, DataDir: t.TempDir(),
		Providers: func(domain.ProviderConnection) (providers.Provider, error) { return scenarioProvider{prov, h}, nil },
		Logf:      t.Logf,
	}
	for _, m := range mutate {
		m(&cfg)
	}
	h.eng = engine.New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { h.eng.Run(ctx); close(done) }()
	var once sync.Once
	h.stopEngine = func() { once.Do(func() { cancel(); <-done }) }
	t.Cleanup(h.stopEngine)
	return h
}

// start confirms the project's workflow and starts a run.
func (h *harness) start(p testenv.Project) string {
	h.t.Helper()
	w, err := h.db.Workflow(h.ctx, p.ID, p.WorkflowID)
	if err != nil {
		h.t.Fatal(err)
	}
	v, res, err := h.db.ConfirmVersion(h.ctx, p.ID, w.ID, w.Revision)
	if err != nil {
		h.t.Fatalf("confirm: %v %v", err, res.Issues)
	}
	runID, err := h.eng.StartRun(h.ctx, p.ID, v.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return runID
}

func (h *harness) detail(projectID, runID string) engine.RunDetail {
	h.t.Helper()
	d, err := h.eng.RunDetail(h.ctx, projectID, runID)
	if err != nil {
		h.t.Fatal(err)
	}
	return d
}

// waitFor polls until cond holds, failing with the run state on timeout.
func (h *harness) waitFor(projectID, runID, what string, cond func(engine.RunDetail) bool) engine.RunDetail {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for {
		d := h.detail(projectID, runID)
		if cond(d) {
			return d
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("timed out waiting for %s; run=%s\n%s", what, d.Status, describe(d))
		}
		h.eng.Wake()
		time.Sleep(10 * time.Millisecond)
	}
}

func (h *harness) waitStep(projectID, runID, step, status string) engine.StepView {
	h.t.Helper()
	return h.waitFor(projectID, runID, step+"="+status, func(d engine.RunDetail) bool { return d.Step(step).Status == status }).Step(step)
}

func (h *harness) waitRun(projectID, runID, status string) engine.RunDetail {
	h.t.Helper()
	return h.waitFor(projectID, runID, "run="+status, func(d engine.RunDetail) bool { return d.Status == status })
}

func describe(d engine.RunDetail) string {
	var b strings.Builder
	for _, s := range d.Steps {
		b.WriteString("  " + s.ID + ": " + s.Status)
		if s.Error != "" {
			b.WriteString(" (" + s.Error + ")")
		}
		b.WriteString("\n")
	}
	return b.String()
}

// approve approves the step's pending approval.
func (h *harness) approve(projectID, runID, step string) {
	h.t.Helper()
	s := h.waitStep(projectID, runID, step, engine.StWaitingApproval)
	if _, err := h.eng.DecideApproval(h.ctx, projectID, s.ApprovalID, s.Generation, engine.ApprovalApproved, "좋습니다", nil); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) review(projectID, runID, step, decision, comment string, targets ...string) engine.Outcome {
	h.t.Helper()
	s := h.waitStep(projectID, runID, step, engine.StWaitingHuman)
	out, err := h.eng.SubmitReview(h.ctx, projectID, s.AttemptID, s.Generation, decision, comment, targets, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	return out
}

// setModel switches the test scenario used by one fixture assignment.
func (h *harness) setModel(p testenv.Project, fixtureID, scenario string) {
	h.t.Helper()
	list, _ := h.db.Assignments(h.ctx, p.ID)
	for _, a := range list {
		if a.ID == p.Assignments[fixtureID] {
			a.Model = scenario
			if _, err := h.db.SaveAssignment(h.ctx, a); err != nil {
				h.t.Fatal(err)
			}
			return
		}
	}
	h.t.Fatalf("no assignment %s", fixtureID)
}
