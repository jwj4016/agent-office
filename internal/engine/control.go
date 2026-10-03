package engine

import (
	"context"
	"fmt"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

func (e *Engine) runInProject(ctx context.Context, projectID, runID string) error {
	return e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefRun, ID: runID})
}

// PauseRun stops new steps from starting in one run. Running steps go on;
// human submissions are still accepted (spec §7.5, T08).
func (e *Engine) PauseRun(ctx context.Context, projectID, runID string) error {
	return e.setPaused(ctx, projectID, runID, true)
}

// ResumeRun lets a paused run start new steps again.
func (e *Engine) ResumeRun(ctx context.Context, projectID, runID string) error {
	return e.setPaused(ctx, projectID, runID, false)
}

func (e *Engine) setPaused(ctx context.Context, projectID, runID string, paused bool) error {
	if err := e.runInProject(ctx, projectID, runID); err != nil {
		return err
	}
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, runID)
		if err != nil {
			return err
		}
		if st.run.Status == RunCancelled || st.run.Status == RunSucceeded {
			return fmt.Errorf("%w: 끝난 실행입니다", ErrInvalid)
		}
		if st.run.Paused == paused {
			return nil
		}
		v := 0
		kind := "run.resumed"
		if paused {
			v, kind = 1, "run.paused"
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE runs SET paused = ? WHERE id = ?`, v, runID); err != nil {
			return err
		}
		st.run.Paused = paused
		if err := c.Emit(projectID, runID, "", kind, nil); err != nil {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	e.Wake()
	return err
}

// CancelRun stops a run: waiting steps are cancelled and live provider
// sessions are asked to stop. Files and external effects already made
// are not rolled back, and the UI must not claim they were.
func (e *Engine) CancelRun(ctx context.Context, projectID, runID string) error {
	if err := e.runInProject(ctx, projectID, runID); err != nil {
		return err
	}
	var live []string
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, runID)
		if err != nil {
			return err
		}
		if st.run.Status == RunCancelled || st.run.Status == RunSucceeded {
			return nil
		}
		for _, id := range st.graph.Order() {
			a, ok := st.attempts[id]
			if !ok {
				continue
			}
			switch {
			case a.Status == StWaitingHuman || a.Status == StWaitingApproval && st.graph.Node(id).Kind == domain.KindApproval:
				if _, err := setAttemptStatus(ctx, c, st, a, StCancelled, "", a.Status); err != nil {
					return err
				}
			case isLive(a.Status):
				live = append(live, a.ID)
			}
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE runs SET status = 'cancelled', ended_at = ? WHERE id = ?`, storage.Now(), runID); err != nil {
			return err
		}
		return c.Emit(projectID, runID, "", "run.status", map[string]string{"from": st.run.Status, "to": RunCancelled})
	})
	e.cancelLive(live)
	return err
}

// RetryStep queues a new attempt of a failed, interrupted or cancelled
// step at the same generation.
func (e *Engine) RetryStep(ctx context.Context, projectID, runID, stepID string) error {
	if err := e.runInProject(ctx, projectID, runID); err != nil {
		return err
	}
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, runID)
		if err != nil {
			return err
		}
		if st.run.Status == RunCancelled {
			return fmt.Errorf("%w: 취소된 실행입니다", ErrInvalid)
		}
		a, ok := st.attempts[stepID]
		if !ok || !isBlocking(a.Status) {
			return fmt.Errorf("%w: 실패·중단된 업무만 다시 시도할 수 있습니다", ErrInvalid)
		}
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO step_attempts (id, project_id, run_id, step_id, revision_round, attempt, generation, status, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'pending', ?)`, storage.NewID("att"), projectID, runID, stepID, a.Round, a.Attempt+1, a.Generation, storage.Now()); err != nil {
			return err
		}
		return c.Emit(projectID, runID, a.ID, "step.retry_requested", map[string]any{"stepId": stepID, "attempt": a.Attempt + 1})
	})
	e.Wake()
	return err
}

// recover runs at startup: provider processes from a previous app run are
// gone, so their attempts are marked interrupted instead of being resumed
// or silently re-run (spec §7.5). Human waits are kept.
func (e *Engine) recover(ctx context.Context) error {
	ids, err := e.activeRunIDs(ctx)
	if err != nil {
		return err
	}
	for _, runID := range ids {
		_, err := e.db.Change(ctx, func(c *storage.Change) error {
			st, err := loadState(ctx, c.Tx, runID)
			if err != nil {
				return err
			}
			for _, id := range st.graph.Order() {
				a, ok := st.attempts[id]
				if !ok {
					continue
				}
				aiWait := (a.Status == StWaitingApproval || a.Status == StWaitingInput) && st.graph.Node(id).Kind != domain.KindApproval
				if a.Status == StRunning || a.Status == StVerifying || aiWait {
					if _, err := setAttemptStatus(ctx, c, st, a, StInterrupted, "앱이 다시 시작되어 중단되었습니다. 결과를 확인한 뒤 다시 시도하세요", a.Status); err != nil {
						return err
					}
				}
			}
			return saveRunStatus(ctx, c, st)
		})
		if err != nil {
			return err
		}
	}
	return nil
}
