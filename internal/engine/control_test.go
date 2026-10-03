package engine_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"agent-office/internal/engine"
	"agent-office/internal/testenv"
)

// The app-wide limit (2) holds across projects; human waits use no slot.
func TestConcurrencyLimits(t *testing.T) {
	h := newHarness(t)
	var projects []testenv.Project
	var runs []string
	for _, name := range []string{"게임", "부동산"} {
		p := testenv.ServiceDev(t, h.db, name)
		h.setModel(p, "a-planner", "slow")
		projects = append(projects, p)
	}
	third := testenv.ServiceDev(t, h.db, "세번째")
	h.setModel(third, "a-planner", "slow")
	projects = append(projects, third)
	for _, p := range projects {
		runs = append(runs, h.start(p))
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		running := 0
		for i, p := range projects {
			if h.detail(p.ID, runs[i]).Step("plan").Status == engine.StRunning {
				running++
			}
		}
		if running > 2 {
			t.Fatalf("%d AI steps running, limit is 2", running)
		}
		if running == 2 {
			// The third run is ready but waits for a slot.
			pending := 0
			for i, p := range projects {
				if h.detail(p.ID, runs[i]).Step("plan").Status == engine.StPending {
					pending++
				}
			}
			if pending != 1 {
				t.Fatalf("expected one queued step, got %d", pending)
			}
			// Cancelling one frees a slot for the queued run.
			for i, p := range projects {
				if h.detail(p.ID, runs[i]).Step("plan").Status == engine.StRunning {
					h.eng.CancelRun(h.ctx, p.ID, runs[i])
					break
				}
			}
			for i, p := range projects {
				if h.detail(p.ID, runs[i]).Status != engine.RunCancelled {
					h.waitStep(p.ID, runs[i], "plan", engine.StRunning)
				}
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("never reached two running steps")
}

// T08: pausing one service's run holds its new steps; another service
// keeps going. Submissions to the paused run are still stored.
func TestPauseOneRunOthersContinue(t *testing.T) {
	h := newHarness(t)
	game := testenv.ServiceDev(t, h.db, "게임")
	estate := testenv.ServiceDev(t, h.db, "부동산")
	gameRun, estateRun := h.start(game), h.start(estate)

	h.waitStep(game.ID, gameRun, "approve", engine.StWaitingApproval)
	if err := h.eng.PauseRun(h.ctx, game.ID, gameRun); err != nil {
		t.Fatal(err)
	}
	if d := h.detail(game.ID, gameRun); d.Status != engine.RunPaused {
		t.Fatalf("status = %s", d.Status)
	}
	h.approve(game.ID, gameRun, "approve") // accepted while paused
	time.Sleep(150 * time.Millisecond)
	if d := h.detail(game.ID, gameRun); d.Step("design").Status != engine.StPending || d.Step("approve").Status != engine.StSucceeded {
		t.Fatalf("paused run advanced:\n%s", describe(d))
	}

	// The other service is unaffected.
	h.approve(estate.ID, estateRun, "approve")
	h.waitStep(estate.ID, estateRun, "review", engine.StWaitingHuman)

	if err := h.eng.ResumeRun(h.ctx, game.ID, gameRun); err != nil {
		t.Fatal(err)
	}
	h.waitStep(game.ID, gameRun, "review", engine.StWaitingHuman)
}

// T10: a failed required step blocks its dependents; independent steps
// still run, and the run reports failed only when nothing can move.
func TestFailureBlocksOnlyDependents(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.setModel(p, "a-backend", "provider-failure")
	h.setModel(p, "a-frontend", "slow")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "backend", engine.StFailed)
	// The failure must not stop the independent frontend from running.
	h.waitStep(p.ID, runID, "frontend", engine.StRunning)
	d := h.detail(p.ID, runID)
	if d.Status != engine.RunRunning || d.Step("backend").Status != engine.StFailed {
		t.Fatalf("independent work stopped:\n%s", describe(d))
	}
	if d.Step("integrate").Status != engine.StPending {
		t.Fatal("dependent of a failed step started")
	}

	// Retry with a working scenario; the run continues.
	h.useScenario("backend", "auto")
	if err := h.eng.RetryStep(h.ctx, p.ID, runID, "backend"); err != nil {
		t.Fatal(err)
	}
	s := h.waitStep(p.ID, runID, "backend", engine.StSucceeded)
	if s.Attempt != 2 {
		t.Fatalf("retry attempt = %d", s.Attempt)
	}
}

// T10 continued: when the only remaining work depends on a failure, the
// run is failed (not silently waiting forever).
func TestRunFailsWhenBlocked(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.setModel(p, "a-planner", "provider-failure")
	runID := h.start(p)
	h.waitRun(p.ID, runID, engine.RunFailed)
}

// Cancel stops live work, marks waiting human steps cancelled and makes
// later decisions stale.
func TestCancelRun(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
	if err := h.eng.CancelRun(h.ctx, p.ID, runID); err != nil {
		t.Fatal(err)
	}
	d := h.waitRun(p.ID, runID, engine.RunCancelled)
	if d.Step("approve").Status != engine.StCancelled {
		t.Fatalf("approve = %s", d.Step("approve").Status)
	}
	if _, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalApproved, "", nil); !errors.Is(err, engine.ErrStale) {
		t.Fatalf("decision after cancel: %v", err)
	}
}

func TestCancelInterruptsRunningProvider(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.setModel(p, "a-planner", "slow")
	runID := h.start(p)
	h.waitStep(p.ID, runID, "plan", engine.StRunning)
	start := time.Now()
	h.eng.CancelRun(h.ctx, p.ID, runID)
	h.waitStep(p.ID, runID, "plan", engine.StCancelled)
	if time.Since(start) > 3*time.Second {
		t.Fatal("cancel took too long")
	}
}

// T24: concurrent duplicate clicks apply once; the rest see "already".
func TestDuplicateApprovalClicks(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
	var wg sync.WaitGroup
	var mu sync.Mutex
	applied, already := 0, 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalApproved, "ok", nil)
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err != nil:
				t.Errorf("click failed: %v", err)
			case out.Already:
				already++
			default:
				applied++
			}
		}()
	}
	wg.Wait()
	if applied != 1 || already != 7 {
		t.Fatalf("applied=%d already=%d", applied, already)
	}
	// A conflicting late decision is refused, not applied.
	if _, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalRejected, "늦은 반려", []string{"plan"}); !errors.Is(err, engine.ErrStale) {
		t.Fatalf("conflicting decision: %v", err)
	}
	var events int
	h.db.Read().QueryRow(`SELECT COUNT(*) FROM execution_events WHERE run_id = ? AND kind = 'approval.decided'`, runID).Scan(&events)
	if events != 1 {
		t.Fatalf("approval.decided recorded %d times", events)
	}
}

// T23 at the engine boundary: ids from another project are refused.
func TestEngineRejectsCrossProjectIDs(t *testing.T) {
	h := newHarness(t)
	game := testenv.ServiceDev(t, h.db, "게임")
	estate := testenv.ServiceDev(t, h.db, "부동산")
	gameRun := h.start(game)
	s := h.waitStep(game.ID, gameRun, "approve", engine.StWaitingApproval)
	if _, err := h.eng.DecideApproval(h.ctx, estate.ID, s.ApprovalID, s.Generation, engine.ApprovalApproved, "", nil); err == nil {
		t.Fatal("approved another project's step")
	}
	if err := h.eng.CancelRun(h.ctx, estate.ID, gameRun); err == nil {
		t.Fatal("cancelled another project's run")
	}
	if _, err := h.eng.RunDetail(h.ctx, estate.ID, gameRun); err == nil {
		t.Fatal("read another project's run")
	}
}
