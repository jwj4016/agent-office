package engine_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"agent-office/internal/engine"
	"agent-office/internal/testenv"
)

// Review finding 1: a provider that fails to start (e.g. a wrong
// executable path) must fail the step, not crash the app.
func TestProviderStartFailureFailsStep(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.useScenario("plan", "no-such-scenario")
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "plan", engine.StFailed)
	if !strings.Contains(s.Error, "공급자 시작 실패") || !strings.Contains(s.Error, "no-such-scenario") {
		t.Fatalf("error = %q", s.Error)
	}
	// The engine is still alive: a retry with a working scenario runs.
	h.useScenario("plan", "auto")
	if err := h.eng.RetryStep(h.ctx, p.ID, runID, "plan"); err != nil {
		t.Fatal(err)
	}
	h.waitStep(p.ID, runID, "plan", engine.StSucceeded)
}

// review-gate: a human review whose node also requires another output
// and a verification command.
var strictReviewWorkflow = []byte(`{
  "schemaVersion": 1, "title": "엄격한 리뷰",
  "nodes": [
    {"id": "dev", "title": "개발", "kind": "task", "assignmentId": "a-backend", "dependsOn": [], "outputs": [{"key": "change", "type": "code_change"}]},
    {"id": "review", "title": "리뷰", "kind": "review", "assignmentId": "a-owner", "dependsOn": ["dev"],
     "inputs": [{"name": "변경", "fromStep": "dev", "outputKey": "change"}], "reworkTargets": ["dev"],
     "outputs": [{"key": "review", "type": "report"}, {"key": "checklist", "type": "markdown"}],
     "completion": {"commands": [{"executable": "COMMAND"}]}}
  ]}`)

// Review finding 2: passing a review must meet the node's full completion
// criteria (all required outputs and verification commands).
func TestReviewPassMeetsCompletionCriteria(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		outputs       map[string]string
		wantProblem   string
	}{
		{"missing required output", "true", nil, `"checklist"`},
		{"failing verification command", "false", map[string]string{"checklist": "- 확인"}, "검증 명령"},
		{"all criteria met", "true", map[string]string{"checklist": "- 확인"}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t)
			wf := []byte(strings.Replace(string(strictReviewWorkflow), "COMMAND", tc.command, 1))
			p := testenv.SeedDraft(t, h.db, "리뷰", wf, map[string]string{"a-backend": "백엔드"}, []string{"a-owner"})
			runID := h.start(p)
			s := h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
			out, err := h.eng.SubmitReview(h.ctx, p.ID, s.AttemptID, s.Generation, engine.ReviewPass, "확인", nil, tc.outputs)
			var verr *engine.VerificationError
			if tc.wantProblem == "" {
				if err != nil || out.Status != engine.StSucceeded {
					t.Fatalf("pass refused: %+v %v", out, err)
				}
				h.waitRun(p.ID, runID, engine.RunSucceeded)
				return
			}
			if !errors.As(err, &verr) || !strings.Contains(verr.Problem, tc.wantProblem) {
				t.Fatalf("want problem %q, got %+v %v", tc.wantProblem, out, err)
			}
			if st := h.detail(p.ID, runID).Step("review"); st.Status != engine.StWaitingHuman || len(st.Artifacts) != 0 {
				t.Fatalf("refused review changed the step: %+v", st)
			}
		})
	}
}

// Requesting changes is not completing the step, so it is not blocked by
// the step's own completion criteria.
func TestReviewChangesNotBlockedByCompletionCriteria(t *testing.T) {
	h := newHarness(t)
	wf := []byte(strings.Replace(string(strictReviewWorkflow), "COMMAND", "false", 1))
	p := testenv.SeedDraft(t, h.db, "리뷰", wf, map[string]string{"a-backend": "백엔드"}, []string{"a-owner"})
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	if _, err := h.eng.SubmitReview(h.ctx, p.ID, s.AttemptID, s.Generation, engine.ReviewChangesRequested, "다시", []string{"dev"}, nil); err != nil {
		t.Fatal(err)
	}
	if again := h.waitStep(p.ID, runID, "review", engine.StWaitingHuman); again.Generation != 2 {
		t.Fatalf("review generation = %d", again.Generation)
	}
}

// Review finding 4: a second engine on the same database must not start
// (and so must not mark the first engine's live work interrupted).
func TestSecondEngineRefusedWhileFirstRuns(t *testing.T) {
	h := newHarness(t)
	p := seedIndependent(t, h)
	h.useScenario("market", "slow")
	runID := h.start(p)
	h.waitStep(p.ID, runID, "market", engine.StRunning)

	second := engine.New(engine.Config{DB: h.db, DataDir: t.TempDir(), Logf: t.Logf})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := second.Run(ctx); !errors.Is(err, engine.ErrEngineRunning) {
		t.Fatalf("second engine Run = %v", err)
	}
	if s := h.detail(p.ID, runID).Step("market"); s.Status != engine.StRunning {
		t.Fatalf("first engine's live step became %s", s.Status)
	}
}

// Recovery after a real crash: the first engine is gone, then a new one
// starts and marks the orphaned attempt interrupted.
func TestRecoverAfterEngineStopped(t *testing.T) {
	h := newHarness(t)
	p := seedIndependent(t, h)
	runID := h.start(p)
	h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
	h.waitStep(p.ID, runID, "market", engine.StSucceeded)
	h.stopEngine()

	// Simulate a crash mid-step: an attempt left "running" with no process.
	var marketID string
	h.db.Read().QueryRow(`SELECT id FROM step_attempts WHERE run_id = ? AND step_id = 'market'`, runID).Scan(&marketID)
	h.db.Write(h.ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec(`UPDATE step_attempts SET status = 'running' WHERE id = ?`, marketID)
		return err
	})

	second := engine.New(engine.Config{DB: h.db, DataDir: t.TempDir(), Logf: t.Logf})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- second.Run(ctx) }()
	defer func() { cancel(); <-done }()
	h.eng = second

	h.waitStep(p.ID, runID, "market", engine.StInterrupted)
	if d := h.detail(p.ID, runID); d.Step("approve").Status != engine.StWaitingApproval {
		t.Fatalf("human wait lost: %s", d.Step("approve").Status)
	}
	var attempts int
	h.db.Read().QueryRow(`SELECT COUNT(*) FROM step_attempts WHERE run_id = ? AND step_id = 'market'`, runID).Scan(&attempts)
	if attempts != 1 {
		t.Fatalf("interrupted step was re-run automatically (%d attempts)", attempts)
	}
}
