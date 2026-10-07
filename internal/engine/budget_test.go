package engine_test

import (
	"encoding/json"
	"testing"
	"time"

	"agent-office/internal/engine"
	"agent-office/internal/testenv"
)

// T19: when a service reaches its budget, new AI work is held (not failed),
// the person sees why, and raising the limit lets the run continue.
func TestBudgetHoldsNewAIWork(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	proj, _ := h.db.Project(h.ctx, p.ID)
	proj.Budget = json.RawMessage(`{"maxTokens": 500}`)
	if _, err := h.db.UpdateProject(h.ctx, proj); err != nil {
		t.Fatal(err)
	}
	h.useScenario("plan", "plan-heavy")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")

	// 600 tokens used ≥ 500: design is ready but must not start.
	d := h.waitRun(p.ID, runID, engine.RunWaiting)
	time.Sleep(100 * time.Millisecond)
	if d = h.detail(p.ID, runID); d.Step("design").Status != engine.StPending || d.Status != engine.RunWaiting {
		t.Fatalf("budget not enforced:\n%s", describe(d))
	}
	u, err := h.eng.ProjectUsage(h.ctx, p.ID)
	if err != nil || u.InputTokens != 400 || u.OutputTokens != 200 || u.CostUSD != 0.05 || u.HoldReason == "" {
		t.Fatalf("usage = %+v, %v", u, err)
	}
	items, _ := h.eng.Inbox(h.ctx)
	if len(items) != 1 || items[0].Kind != engine.InboxBudget || items[0].Detail == "" {
		t.Fatalf("inbox = %+v", items)
	}

	proj, _ = h.db.Project(h.ctx, p.ID)
	proj.Budget = json.RawMessage(`{"maxTokens": 100000}`)
	h.db.UpdateProject(h.ctx, proj)
	h.eng.Wake()
	h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
}

// Unknown costs are never counted as zero: they are reported separately.
func TestUsageUnknownCost(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.useScenario("plan", "plan-success") // reports usage with costUsd null
	runID := h.start(p)
	h.waitStep(p.ID, runID, "plan", engine.StSucceeded)
	u, _ := h.eng.ProjectUsage(h.ctx, p.ID)
	if u.UnknownCostAttempts != 1 || u.CostUSD != 0 || u.InputTokens != 120 {
		t.Fatalf("usage = %+v", u)
	}
}
