package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

func checkReworkRequest(n *domain.Node, targets []string, reason string) error {
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("%w: 수정 요청에는 구체적인 이유가 필요합니다", ErrInvalid)
	}
	if len(targets) == 0 {
		return fmt.Errorf("%w: 돌려보낼 업무를 하나 이상 고르세요", ErrInvalid)
	}
	for _, t := range targets {
		if !contains(n.ReworkTargets, t) {
			return fmt.Errorf("%w: %q는 이 업무의 수정 요청 대상이 아닙니다", ErrInvalid, t)
		}
	}
	return nil
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func (e *Engine) maxRevisions(st *runState, stepID string) int {
	if n := st.graph.Node(stepID); n != nil && n.Limits != nil && n.Limits.MaxRevisions != nil {
		return *n.Limits.MaxRevisions
	}
	if st.run.Limits.MaxRevisions > 0 {
		return st.run.Limits.MaxRevisions
	}
	return e.cfg.DefaultMaxRevisions
}

// reworkTx sends targets and everything downstream of them into a new
// generation (spec §7.4). Old attempts are marked superseded and their
// artifacts stale, but nothing is deleted. Steps outside the scope keep
// their results. It returns attempts still running, which the caller
// must cancel after commit; new attempts for those steps wait until the
// old process has stopped.
func (e *Engine) reworkTx(ctx context.Context, c *storage.Change, st *runState, from *domain.Node, fromAttempt string, targets []string, reason string) ([]string, error) {
	for _, t := range targets {
		if limit := e.maxRevisions(st, t); st.run.Gens[t].R >= limit {
			return nil, fmt.Errorf("%w: %q는 이미 %d회 수정되었습니다. 범위·기준·횟수를 조정하세요", ErrRevisionLimit, t, st.run.Gens[t].R)
		}
	}
	scope := st.graph.Descendants(targets...)
	for _, t := range targets {
		scope[t] = true
	}
	var steps []string
	for s := range scope {
		steps = append(steps, s)
	}
	sort.Strings(steps)

	var live []string
	for _, s := range steps {
		a, ran := st.attempts[s]
		if ran {
			switch {
			case a.Status == StRunning || a.Status == StVerifying || (a.Status == StWaitingApproval || a.Status == StWaitingInput) && st.graph.Node(s).Kind != domain.KindApproval:
				live = append(live, a.ID) // ends as superseded when it stops
			case a.Status != StPending:
				if _, err := setAttemptStatus(ctx, c, st, a, StSuperseded, "", StWaitingHuman, StWaitingApproval, StSucceeded, StSkipped, StFailed, StInterrupted, StCancelled); err != nil {
					return nil, err
				}
			}
		}
		g := st.run.Gens[s]
		g.G++
		if ran {
			g.R++ // a revision round only counts for steps that had a result
		}
		st.run.Gens[s] = g
		delete(st.attempts, s)
	}
	if _, err := c.Tx.ExecContext(ctx, `UPDATE artifacts SET validity = 'stale'
		WHERE run_id = ? AND validity = 'valid' AND step_attempt_id IN (SELECT id FROM step_attempts WHERE run_id = ? AND step_id IN (`+placeholders(len(steps))+`))`,
		append([]any{st.run.ID, st.run.ID}, toAny(steps)...)...); err != nil {
		return nil, err
	}
	if err := saveGens(ctx, c, st); err != nil {
		return nil, err
	}
	for _, t := range targets {
		refs, _ := json.Marshal(map[string]any{"generation": st.run.Gens[t].G, "from": from.ID})
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO messages (id, project_id, run_id, step_attempt_id, sender, recipient, kind, body, artifact_refs, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 'decision', ?, ?, ?)`,
			storage.NewID("msg"), st.run.ProjectID, st.run.ID, fromAttempt, domain.LocalOwner, "step:"+t, reason, string(refs), storage.Now()); err != nil {
			return nil, err
		}
	}
	return live, c.Emit(st.run.ProjectID, st.run.ID, fromAttempt, "rework.requested", map[string]any{
		"from": from.ID, "targets": targets, "scope": steps, "reason": reason,
	})
}

func toAny(xs []string) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// cancelLive asks still-running superseded attempts to stop.
func (e *Engine) cancelLive(ids []string) {
	for _, id := range ids {
		if a := e.lookupActive(id); a != nil {
			a.session.Cancel(context.Background())
		}
	}
}
