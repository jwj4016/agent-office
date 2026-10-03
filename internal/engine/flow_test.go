package engine_test

import (
	"encoding/json"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/testenv"
)

// T01 + the first user journey: plan → owner approval → design →
// backend/frontend → integrate → owner review → QA → deliver.
func TestServiceDevFlowEndToEnd(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	runID := h.start(p)

	h.waitStep(p.ID, runID, "plan", engine.StSucceeded)
	// Approval waits for the person; nothing downstream starts meanwhile.
	d := h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
	if run := h.detail(p.ID, runID); run.Status != engine.RunWaiting || run.Step("design").Status != engine.StPending {
		t.Fatalf("not waiting for approval: %s\n%s", run.Status, describe(run))
	}
	// The approval is pinned to the plan's artifact.
	var pinned []engine.ManifestEntry
	json.Unmarshal(d.Inputs, &pinned)
	if len(pinned) != 1 || pinned[0].ArtifactID == "" || pinned[0].Hash == "" {
		t.Fatalf("approval not pinned to plan output: %s", d.Inputs)
	}
	h.approve(p.ID, runID, "approve")

	h.review(p.ID, runID, "review", engine.ReviewPass, "코드 확인 완료")
	run := h.waitRun(p.ID, runID, engine.RunSucceeded)
	for _, s := range run.Steps {
		if s.Status != engine.StSucceeded {
			t.Fatalf("step %s = %s", s.ID, s.Status)
		}
	}

	// The human review is a report artifact QA received as input.
	review := run.Step("review").Artifacts
	if len(review) != 1 || review[0].Type != "report" {
		t.Fatalf("review artifact = %+v", review)
	}
	var qaInputs []engine.ManifestEntry
	json.Unmarshal(run.Step("qa").Inputs, &qaInputs)
	if qaInputs[0].ArtifactID != review[0].ID {
		t.Fatalf("QA did not receive the review report: %+v", qaInputs)
	}
	// Integration ran only after both developers finished.
	if run.Step("integrate").Attempt != 1 || len(run.Step("integrate").Artifacts) != 1 {
		t.Fatalf("integrate = %+v", run.Step("integrate"))
	}
}

// T15: the model says it is done, but its required output is missing.
func TestClaimedDoneWithoutOutputFails(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.setModel(p, "a-planner", "claims-done-no-complete")
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "plan", engine.StFailed)
	if s.Error == "" {
		t.Fatal("no failure reason recorded")
	}
	h.waitRun(p.ID, runID, engine.RunFailed)
}

// T15: provider reports success and writes the file, but the completion
// command (tests) fails.
func TestFailingVerificationCommandFails(t *testing.T) {
	h := newHarness(t)
	draft := testenv.EditFixture(t, "workflows/service-dev.json", func(n map[string]map[string]any) {
		n["plan"]["completion"] = map[string]any{"commands": []any{map[string]any{"executable": "false"}}}
	})
	p := testenv.SeedDraft(t, h.db, "게임", draft, testenv.ServiceDevRoles, []string{"a-owner"})
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "plan", engine.StFailed)
	if len(s.Artifacts) != 0 {
		t.Fatal("artifacts stored for a failed verification")
	}
}
