package engine_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"agent-office/internal/engine"
	"agent-office/internal/testenv"
)

// T05: the owner's review sends only the backend back. Backend and
// everything after it re-run; frontend keeps its result; old results are
// kept but marked stale; the backend sees the reason.
func TestReviewChangesReworkOnlyAffectedSteps(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	before := h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	d0 := h.detail(p.ID, runID)
	frontend0, backend0 := d0.Step("frontend"), d0.Step("backend")

	out := h.review(p.ID, runID, "review", engine.ReviewChangesRequested, "에러 처리가 빠졌습니다", "backend")
	if out.Status != engine.StSuperseded {
		t.Fatalf("outcome = %+v", out)
	}

	review := h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	d := h.detail(p.ID, runID)
	if review.Generation != before.Generation+1 || review.Round != 1 {
		t.Fatalf("review gen=%d round=%d", review.Generation, review.Round)
	}
	if f := d.Step("frontend"); f.AttemptID != frontend0.AttemptID || f.Generation != 1 || f.Artifacts[0].Validity != "valid" {
		t.Fatalf("frontend was re-run or invalidated: %+v", f)
	}
	if b := d.Step("backend"); b.AttemptID == backend0.AttemptID || b.Generation != 2 {
		t.Fatalf("backend not re-run: %+v", b)
	}
	var validity string
	h.db.Read().QueryRow(`SELECT validity FROM artifacts WHERE id = ?`, backend0.Artifacts[0].ID).Scan(&validity)
	if validity != "stale" {
		t.Fatalf("old backend artifact validity = %s", validity)
	}
	var oldStatus string
	h.db.Read().QueryRow(`SELECT status FROM step_attempts WHERE id = ?`, backend0.AttemptID).Scan(&oldStatus)
	if oldStatus != engine.StSuperseded {
		t.Fatalf("old backend attempt = %s", oldStatus)
	}
	reqs := h.started("backend")
	if len(reqs) != 2 || !strings.Contains(reqs[1].Instructions, "에러 처리가 빠졌습니다") || strings.Contains(reqs[0].Instructions, "에러 처리") {
		t.Fatalf("feedback not delivered to the new backend attempt only")
	}
	if len(h.started("frontend")) != 1 {
		t.Fatal("frontend started again")
	}
	// The new review pins the new integration result.
	if !strings.Contains(string(review.Inputs), d.Step("integrate").Artifacts[0].ID) {
		t.Fatal("review not pinned to the new integration result")
	}

	h.review(p.ID, runID, "review", engine.ReviewPass, "좋습니다")
	h.waitRun(p.ID, runID, engine.RunSucceeded)
}

// independent.json: plan → approve(rework plan) → report, plus a market
// analysis that does not depend on the plan.
var independentWorkflow = []byte(`{
  "schemaVersion": 1, "title": "기획과 독립 분석",
  "nodes": [
    {"id": "plan", "title": "기획", "kind": "task", "assignmentId": "a-planner", "dependsOn": [], "outputs": [{"key": "spec", "type": "markdown"}]},
    {"id": "side", "title": "기획 기반 부가 작업", "kind": "task", "assignmentId": "a-qa", "dependsOn": ["plan"], "outputs": [{"key": "note", "type": "markdown"}]},
    {"id": "approve", "title": "기획 승인", "kind": "approval", "assignmentId": "a-owner", "dependsOn": ["plan"], "reworkTargets": ["plan"]},
    {"id": "market", "title": "시장 분석", "kind": "task", "assignmentId": "a-architect", "dependsOn": [], "outputs": [{"key": "market", "type": "markdown"}]},
    {"id": "report", "title": "보고", "kind": "task", "assignmentId": "a-planner", "dependsOn": ["approve", "market", "side"],
     "inputs": [{"name": "시장", "fromStep": "market", "outputKey": "market"}], "outputs": [{"key": "report", "type": "markdown"}]}
  ]}`)

func seedIndependent(t *testing.T, h *harness) testenv.Project {
	return testenv.SeedDraft(t, h.db, "분석", independentWorkflow,
		map[string]string{"a-planner": "기획", "a-architect": "분석", "a-qa": "보조"}, []string{"a-owner"})
}

// T06: rejecting the plan re-runs the plan and what depends on it, but the
// unrelated market analysis keeps its result.
func TestPlanRejectionKeepsIndependentResults(t *testing.T) {
	h := newHarness(t)
	p := seedIndependent(t, h)
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
	market := h.waitStep(p.ID, runID, "market", engine.StSucceeded)
	h.waitStep(p.ID, runID, "side", engine.StSucceeded)

	if _, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalRejected, "", []string{"plan"}); !errors.Is(err, engine.ErrInvalid) {
		t.Fatalf("rejection without reason: %v", err)
	}
	if _, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalRejected, "타깃 사용자가 불명확", []string{"market"}); !errors.Is(err, engine.ErrInvalid) {
		t.Fatalf("rejection to a non-target: %v", err)
	}
	if _, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalRejected, "타깃 사용자가 불명확", []string{"plan"}); err != nil {
		t.Fatal(err)
	}
	s2 := h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
	d := h.detail(p.ID, runID)
	if s2.Generation != 2 || d.Step("plan").Generation != 2 || d.Step("side").Generation != 2 {
		t.Fatalf("scope not advanced:\n%s", describe(d))
	}
	if m := d.Step("market"); m.AttemptID != market.AttemptID || m.Artifacts[0].Validity != "valid" {
		t.Fatalf("independent result touched: %+v", m)
	}
	// The old approval is stale now.
	if _, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalApproved, "", nil); !errors.Is(err, engine.ErrStale) {
		t.Fatalf("old approval accepted: %v", err)
	}
	h.approve(p.ID, runID, "approve")
	h.waitRun(p.ID, runID, engine.RunSucceeded)
}

// Spec §7.4: after the default 3 revision rounds a person must decide.
func TestRevisionLimit(t *testing.T) {
	h := newHarness(t)
	p := seedIndependent(t, h)
	runID := h.start(p)
	for round := 1; round <= 4; round++ {
		s := h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
		_, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalRejected, "다시", []string{"plan"})
		if round <= 3 && err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if round == 4 {
			if !errors.Is(err, engine.ErrRevisionLimit) {
				t.Fatalf("4th rework: %v", err)
			}
			// Nothing changed: the approval is still open.
			if d := h.detail(p.ID, runID); d.Step("approve").Status != engine.StWaitingApproval || d.Step("plan").Round != 3 {
				t.Fatalf("state changed after refused rework:\n%s", describe(d))
			}
		}
	}
}

// T11: rework while a dependent AI step is still running. The old run's
// late result is recorded but never becomes the step's result, and the
// new attempt starts only after the old process stopped.
func TestLateResultFromSupersededAttemptIsIgnored(t *testing.T) {
	h := newHarness(t)
	p := seedIndependent(t, h)
	h.useScenario("side", "slow")
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
	old := h.waitStep(p.ID, runID, "side", engine.StRunning)

	h.useScenario("side", "auto")
	if _, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalRejected, "다시", []string{"plan"}); err != nil {
		t.Fatal(err)
	}
	h.waitStep(p.ID, runID, "side", engine.StSucceeded)
	var oldStatus string
	var oldArtifacts, late int
	deadline := time.Now().Add(5 * time.Second)
	for oldStatus != engine.StSuperseded && time.Now().Before(deadline) {
		h.db.Read().QueryRow(`SELECT status FROM step_attempts WHERE id = ?`, old.AttemptID).Scan(&oldStatus)
		time.Sleep(10 * time.Millisecond)
	}
	h.db.Read().QueryRow(`SELECT COUNT(*) FROM artifacts WHERE step_attempt_id = ?`, old.AttemptID).Scan(&oldArtifacts)
	h.db.Read().QueryRow(`SELECT COUNT(*) FROM execution_events WHERE step_attempt_id = ? AND kind = 'provider.late'`, old.AttemptID).Scan(&late)
	if oldStatus != engine.StSuperseded || oldArtifacts != 0 || late == 0 {
		t.Fatalf("old attempt: status=%s artifacts=%d lateEvents=%d", oldStatus, oldArtifacts, late)
	}
	// The new side attempt started after the old one had stopped.
	var oldEnded, newStarted string
	h.db.Read().QueryRow(`SELECT ended_at FROM step_attempts WHERE id = ?`, old.AttemptID).Scan(&oldEnded)
	h.db.Read().QueryRow(`SELECT started_at FROM step_attempts WHERE run_id = ? AND step_id = 'side' AND generation = 2`, runID).Scan(&newStarted)
	if newStarted < oldEnded {
		t.Fatalf("new attempt started (%s) before old stopped (%s)", newStarted, oldEnded)
	}
}
