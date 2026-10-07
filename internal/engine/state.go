package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// Attempt statuses (mirrors the step_attempts CHECK list).
const (
	StPending         = "pending"
	StRunning         = "running"
	StVerifying       = "verifying"
	StWaitingHuman    = "waiting_human"
	StWaitingApproval = "waiting_approval"
	StWaitingInput    = "waiting_input"
	StSucceeded       = "succeeded"
	StSkipped         = "skipped"
	StFailed          = "failed"
	StInterrupted     = "interrupted"
	StCancelled       = "cancelled"
	StSuperseded      = "superseded"
)

// Run statuses.
const (
	RunRunning     = "running"
	RunWaiting     = "waiting"
	RunPaused      = "paused"
	RunSucceeded   = "succeeded"
	RunFailed      = "failed"
	RunInterrupted = "interrupted"
	RunCancelled   = "cancelled"
)

func isWaiting(s string) bool {
	return s == StWaitingHuman || s == StWaitingApproval || s == StWaitingInput
}

func isLive(s string) bool { return s == StRunning || s == StVerifying || isWaiting(s) }

func isDone(s string) bool { return s == StSucceeded || s == StSkipped }

func isBlocking(s string) bool { return s == StFailed || s == StInterrupted || s == StCancelled }

// StepGen is a step's current generation G and revision round R.
type StepGen struct {
	G int `json:"g"`
	R int `json:"r"`
}

type RunLimits struct {
	MaxRevisions int `json:"maxRevisions"`
}

type runRow struct {
	ID, ProjectID, VersionID, Status string
	Paused                           bool
	Gens                             map[string]StepGen
	Limits                           RunLimits
}

type attemptRow struct {
	ID, StepID, Status, AssignmentID string
	Generation, Attempt, Round       int
	InputManifest                    json.RawMessage
}

// runState is a consistent snapshot of one run.
type runState struct {
	run      runRow
	version  domain.WorkflowVersion
	graph    *domain.Graph
	attempts map[string]attemptRow // latest attempt of each step at its current generation
	// budgetHold is set (transiently) when ready AI work was held by the
	// project budget during this scheduling pass.
	budgetHold string
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func loadRunRow(ctx context.Context, q querier, runID string) (runRow, error) {
	var r runRow
	var paused int
	var gens, limits string
	err := q.QueryRowContext(ctx, `SELECT id, project_id, workflow_version_id, status, paused, step_generations, limits FROM runs WHERE id = ?`, runID).
		Scan(&r.ID, &r.ProjectID, &r.VersionID, &r.Status, &paused, &gens, &limits)
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("run %q: %w", runID, storage.ErrNotInProject)
	}
	if err != nil {
		return r, err
	}
	r.Paused = paused == 1
	if err := json.Unmarshal([]byte(gens), &r.Gens); err != nil {
		return r, err
	}
	json.Unmarshal([]byte(limits), &r.Limits)
	return r, nil
}

func loadVersion(ctx context.Context, q querier, id string) (domain.WorkflowVersion, error) {
	var v domain.WorkflowVersion
	var spec, snap, pol string
	err := q.QueryRowContext(ctx, `SELECT id, workflow_id, project_id, number, spec_json, assignments_snapshot, policy_snapshot FROM workflow_versions WHERE id = ?`, id).
		Scan(&v.ID, &v.WorkflowID, &v.ProjectID, &v.Number, &spec, &snap, &pol)
	if err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(spec), &v.Spec); err != nil {
		return v, err
	}
	if err := json.Unmarshal([]byte(snap), &v.Assignments); err != nil {
		return v, err
	}
	return v, json.Unmarshal([]byte(pol), &v.Policy)
}

func loadState(ctx context.Context, q querier, runID string) (*runState, error) {
	r, err := loadRunRow(ctx, q, runID)
	if err != nil {
		return nil, err
	}
	v, err := loadVersion(ctx, q, r.VersionID)
	if err != nil {
		return nil, err
	}
	st := &runState{run: r, version: v, graph: domain.NewGraph(&v.Spec), attempts: map[string]attemptRow{}}
	rows, err := q.QueryContext(ctx, `SELECT id, step_id, status, COALESCE(assignment_id, ''), generation, attempt, revision_round, input_manifest
		FROM step_attempts WHERE run_id = ? ORDER BY step_id, generation, attempt`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var a attemptRow
		var manifest string
		if err := rows.Scan(&a.ID, &a.StepID, &a.Status, &a.AssignmentID, &a.Generation, &a.Attempt, &a.Round, &manifest); err != nil {
			return nil, err
		}
		a.InputManifest = json.RawMessage(manifest)
		if a.Generation == r.Gens[a.StepID].G {
			st.attempts[a.StepID] = a // ordered, so the last one wins
		}
	}
	return st, rows.Err()
}

// status is the step's state at its current generation.
func (s *runState) status(step string) string {
	if a, ok := s.attempts[step]; ok {
		return a.Status
	}
	return StPending
}

// depsDone reports whether every dependency succeeded or was skipped,
// and whether any dependency is blocked by a failure.
func (s *runState) depsDone(n *domain.Node) (done, blocked bool) {
	done = true
	for _, d := range n.DependsOn {
		st := s.status(d)
		if isBlocking(st) {
			blocked = true
		}
		if !isDone(st) {
			done = false
		}
	}
	return
}

// computeRunStatus derives the run status from step states (spec §7.1).
func (s *runState) computeRunStatus() string {
	switch s.run.Status {
	case RunCancelled:
		return RunCancelled
	}
	all, running, waiting, ready := true, false, false, false
	for _, id := range s.graph.Order() {
		st := s.status(id)
		if !isDone(st) {
			all = false
		}
		switch {
		case st == StRunning || st == StVerifying:
			running = true
		case isWaiting(st):
			waiting = true
		case st == StPending:
			if done, _ := s.depsDone(s.graph.Node(id)); done {
				ready = true
			}
		}
	}
	switch {
	case all:
		return RunSucceeded
	case s.run.Paused:
		return RunPaused
	case running:
		return RunRunning
	case waiting:
		return RunWaiting
	case ready && s.budgetHold != "":
		return RunWaiting // a person must raise the budget
	case ready:
		return RunRunning
	default:
		return RunFailed
	}
}

// saveRunStatus stores a status change, emitting run.status once.
func saveRunStatus(ctx context.Context, c *storage.Change, s *runState) error {
	next := s.computeRunStatus()
	if next == s.run.Status {
		return nil
	}
	ended := sql.NullString{}
	if next == RunSucceeded || next == RunFailed || next == RunCancelled {
		ended = sql.NullString{String: storage.Now(), Valid: true}
	}
	if _, err := c.Tx.ExecContext(ctx, `UPDATE runs SET status = ?, ended_at = ? WHERE id = ?`, next, ended, s.run.ID); err != nil {
		return err
	}
	prev := s.run.Status
	s.run.Status = next
	return c.Emit(s.run.ProjectID, s.run.ID, "", "run.status", map[string]string{"from": prev, "to": next})
}

func saveGens(ctx context.Context, c *storage.Change, s *runState) error {
	b, _ := json.Marshal(s.run.Gens)
	_, err := c.Tx.ExecContext(ctx, `UPDATE runs SET step_generations = ? WHERE id = ?`, string(b), s.run.ID)
	return err
}

// setAttemptStatus moves an attempt from one of the allowed statuses to
// next. It returns false (and changes nothing) if the attempt was not in
// an allowed status: the guard that makes double clicks and late events
// harmless (T11, T24).
func setAttemptStatus(ctx context.Context, c *storage.Change, s *runState, a attemptRow, next, errMsg string, from ...string) (bool, error) {
	q := `UPDATE step_attempts SET status = ?, error = CASE WHEN ? <> '' THEN ? ELSE error END,
		ended_at = CASE WHEN ? IN ('succeeded','skipped','failed','interrupted','cancelled','superseded') THEN ? ELSE ended_at END
		WHERE id = ? AND status IN (` + placeholders(len(from)) + `)`
	args := []any{next, errMsg, errMsg, next, storage.Now(), a.ID}
	for _, f := range from {
		args = append(args, f)
	}
	res, err := c.Tx.ExecContext(ctx, q, args...)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return false, nil
	}
	if cur, ok := s.attempts[a.StepID]; ok && cur.ID == a.ID {
		cur.Status = next
		s.attempts[a.StepID] = cur
	}
	payload := map[string]any{"stepId": a.StepID, "generation": a.Generation, "attempt": a.Attempt, "status": next}
	if errMsg != "" {
		payload["error"] = errMsg
	}
	return true, c.Emit(s.run.ProjectID, s.run.ID, a.ID, "step."+next, payload)
}

func placeholders(n int) string {
	if n == 0 {
		return "NULL"
	}
	b := make([]byte, 0, 2*n)
	for i := 0; i < n; i++ {
		if i > 0 {
			b = append(b, ',')
		}
		b = append(b, '?')
	}
	return string(b)
}

// isCurrent reports whether attempt a is still the live attempt for its
// step: same generation as the run, not superseded, run not cancelled.
func (s *runState) isCurrent(a attemptRow) bool {
	cur, ok := s.attempts[a.StepID]
	return ok && cur.ID == a.ID && s.run.Status != RunCancelled
}

func loadAttempt(ctx context.Context, q querier, id string) (attemptRow, string, error) {
	var a attemptRow
	var runID, manifest string
	err := q.QueryRowContext(ctx, `SELECT id, run_id, step_id, status, COALESCE(assignment_id, ''), generation, attempt, revision_round, input_manifest FROM step_attempts WHERE id = ?`, id).
		Scan(&a.ID, &runID, &a.StepID, &a.Status, &a.AssignmentID, &a.Generation, &a.Attempt, &a.Round, &manifest)
	a.InputManifest = json.RawMessage(manifest)
	if errors.Is(err, sql.ErrNoRows) {
		return a, "", fmt.Errorf("attempt %q: %w", id, storage.ErrNotInProject)
	}
	return a, runID, err
}
