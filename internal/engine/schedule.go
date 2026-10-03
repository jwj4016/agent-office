package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// ManifestEntry pins one input artifact version for an attempt.
type ManifestEntry struct {
	Name       string `json:"name"`
	FromStep   string `json:"fromStep"`
	OutputKey  string `json:"outputKey"`
	Required   bool   `json:"required"`
	ArtifactID string `json:"artifactId,omitempty"`
	Path       string `json:"path,omitempty"`
	Hash       string `json:"hash,omitempty"`
	Type       string `json:"type,omitempty"`
}

// connectionReady reports whether an AI assignment's connection exists
// now and is usable (the version snapshot may be older than the
// connection settings).
func (e *Engine) connectionReady(ctx context.Context, connID string) (domain.ProviderConnection, bool) {
	if connID == "" {
		return domain.ProviderConnection{}, false
	}
	c, err := e.db.Connection(ctx, connID)
	if err != nil {
		return c, false
	}
	return c, c.Provider == "test" || c.Verified()
}

// StartRun starts a confirmed workflow version. AI work with a missing or
// unverified connection blocks the start with an explanation (T04).
func (e *Engine) StartRun(ctx context.Context, projectID, versionID string) (string, error) {
	if err := e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefVersion, ID: versionID}); err != nil {
		return "", err
	}
	v, err := loadVersion(ctx, e.db.Read(), versionID)
	if err != nil {
		return "", err
	}
	p, err := e.db.Project(ctx, projectID)
	if err != nil {
		return "", err
	}
	if p.Status != "active" {
		return "", storage.ErrArchived
	}
	infos := map[string]domain.AssignmentInfo{}
	for id, a := range v.Assignments {
		_, ok := e.connectionReady(ctx, a.ConnectionID)
		infos[id] = domain.AssignmentInfo{ActorKind: string(a.ActorKind), Connected: a.ActorKind == domain.ActorHuman || ok}
	}
	issues := append(domain.ValidateStructure(&v.Spec), domain.ValidateAssignments(&v.Spec, infos)...)
	if domain.HasSeverity(issues, domain.SevError) || domain.HasSeverity(issues, domain.SevRun) {
		return "", &NotRunnableError{Issues: issues}
	}
	gens := map[string]StepGen{}
	for _, n := range v.Spec.Nodes {
		gens[n.ID] = StepGen{G: 1}
	}
	gensJSON, _ := json.Marshal(gens)
	limits, _ := json.Marshal(RunLimits{MaxRevisions: e.cfg.DefaultMaxRevisions})
	runID := storage.NewID("run")
	_, err = e.db.Change(ctx, func(c *storage.Change) error {
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO runs (id, project_id, workflow_version_id, status, limits, step_generations, started_at) VALUES (?, ?, ?, 'running', ?, ?, ?)`,
			runID, projectID, versionID, string(limits), string(gensJSON), storage.Now()); err != nil {
			return err
		}
		return c.Emit(projectID, runID, "", "run.started", map[string]any{"versionId": versionID, "versionNumber": v.Number})
	})
	if err != nil {
		return "", err
	}
	e.Wake()
	return runID, nil
}

func (e *Engine) activeRunIDs(ctx context.Context) ([]string, error) {
	rows, err := e.db.Read().QueryContext(ctx, `SELECT id FROM runs WHERE status IN ('running', 'waiting', 'paused', 'failed') ORDER BY started_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		rows.Scan(&id)
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// schedule runs one pass over every unfinished run.
func (e *Engine) schedule(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()
	ids, err := e.activeRunIDs(ctx)
	if err != nil {
		e.cfg.Logf("engine: list runs: %v", err)
		return
	}
	for _, id := range ids {
		if err := e.safeAdvance(ctx, id); err != nil {
			e.cfg.Logf("engine: run %s: %v", id, err)
		}
	}
}

// safeAdvance keeps one run's unexpected panic from taking down the app;
// the run stays as stored and is retried on the next pass.
func (e *Engine) safeAdvance(ctx context.Context, runID string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return e.advance(ctx, runID)
}

// advance starts everything that can start in one run. Engine nodes
// (condition, join) complete immediately, so it loops until stable.
// Caller holds e.mu.
func (e *Engine) advance(ctx context.Context, runID string) error {
	for i := 0; i < 1000; i++ {
		st, err := loadState(ctx, e.db.Read(), runID)
		if err != nil {
			return err
		}
		if st.run.Status == RunCancelled || st.run.Status == RunSucceeded {
			return nil
		}
		progressed := false
		if !st.run.Paused {
			for _, id := range st.graph.Order() {
				n := st.graph.Node(id)
				if st.status(id) != StPending || e.hasLiveAttempt(runID, id) {
					continue
				}
				if done, _ := st.depsDone(n); !done {
					continue
				}
				ok, err := e.startStep(ctx, st, n)
				if err != nil {
					return fmt.Errorf("step %s: %w", id, err)
				}
				if ok {
					progressed = true
					break // state changed: reload before deciding more
				}
			}
		}
		if !progressed {
			_, err := e.db.Change(ctx, func(c *storage.Change) error {
				fresh, err := loadState(ctx, c.Tx, runID)
				if err != nil {
					return err
				}
				return saveRunStatus(ctx, c, fresh)
			})
			return err
		}
	}
	return errors.New("advance did not settle")
}

// startStep begins one ready step. It returns false when the step cannot
// start yet (e.g. no free AI slot).
func (e *Engine) startStep(ctx context.Context, st *runState, n *domain.Node) (bool, error) {
	switch n.Kind {
	case domain.KindCondition:
		return true, e.runCondition(ctx, st, n)
	case domain.KindJoin:
		_, err := e.createAttempt(ctx, st, n, StSucceeded, nil)
		return true, err
	case domain.KindApproval:
		_, err := e.createAttempt(ctx, st, n, StWaitingApproval, nil)
		return true, err
	}
	a := st.version.Assignments[n.AssignmentID]
	if a.ActorKind == domain.ActorHuman {
		_, err := e.createAttempt(ctx, st, n, StWaitingHuman, nil)
		return true, err
	}
	app, project := e.activeCounts(st.run.ProjectID)
	if app >= e.cfg.MaxActive || project >= e.cfg.ProjectMaxActive {
		return false, nil
	}
	return true, e.startAI(ctx, st, n, a)
}

// pinInputs resolves each input to the current valid artifact of its
// source step. Pinned versions never change for this attempt.
func pinInputs(ctx context.Context, q querier, st *runState, n *domain.Node) ([]ManifestEntry, error) {
	var out []ManifestEntry
	for _, in := range n.Inputs {
		m := ManifestEntry{Name: in.Name, FromStep: in.FromStep, OutputKey: in.OutputKey, Required: in.IsRequired()}
		src, ok := st.attempts[in.FromStep]
		if ok && src.Status == StSucceeded {
			err := q.QueryRowContext(ctx, `SELECT id, path, hash, type FROM artifacts WHERE step_attempt_id = ? AND output_key = ? AND validity = 'valid' ORDER BY version DESC LIMIT 1`,
				src.ID, in.OutputKey).Scan(&m.ArtifactID, &m.Path, &m.Hash, &m.Type)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
		}
		if m.ArtifactID == "" && m.Required {
			return nil, fmt.Errorf("required input %q (%s.%s) has no current result", in.Name, in.FromStep, in.OutputKey)
		}
		out = append(out, m)
	}
	return out, nil
}

// createAttempt inserts the step's next attempt at its current
// generation, re-checking inside the transaction that the run state the
// scheduler saw is still true. It returns "" if the state moved on.
func (e *Engine) createAttempt(ctx context.Context, seen *runState, n *domain.Node, status string, extra func(c *storage.Change, st *runState, a attemptRow) error) (string, error) {
	var id string
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, seen.run.ID)
		if err != nil {
			return err
		}
		if st.run.Paused || st.run.Status == RunCancelled || st.status(n.ID) != StPending ||
			st.run.Gens[n.ID].G != seen.run.Gens[n.ID].G {
			return nil
		}
		if done, _ := st.depsDone(n); !done {
			return nil
		}
		manifest, err := pinInputs(ctx, c.Tx, st, n)
		if err != nil {
			return err
		}
		manifestJSON, _ := json.Marshal(manifest)
		gen := st.run.Gens[n.ID]
		now := storage.Now()
		var started, ended any
		if status == StRunning || isWaiting(status) || status == StFailed {
			started = now
		}
		if isDone(status) || status == StFailed {
			started, ended = now, now
		}
		a := attemptRow{StepID: n.ID, Status: status, AssignmentID: n.AssignmentID, Generation: gen.G, Round: gen.R, InputManifest: manifestJSON}
		if queued, ok := st.attempts[n.ID]; ok {
			// A retry queued this attempt as 'pending'; start that row.
			a.ID, a.Attempt = queued.ID, queued.Attempt
			if _, err := c.Tx.ExecContext(ctx, `UPDATE step_attempts SET status = ?, assignment_id = NULLIF(?, ''), input_manifest = ?, started_at = ?, ended_at = ? WHERE id = ? AND status = 'pending'`,
				status, a.AssignmentID, string(manifestJSON), started, ended, a.ID); err != nil {
				return err
			}
		} else {
			a.ID = storage.NewID("att")
			c.Tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(attempt), 0) + 1 FROM step_attempts WHERE run_id = ? AND step_id = ? AND generation = ?`,
				st.run.ID, n.ID, gen.G).Scan(&a.Attempt)
			if _, err := c.Tx.ExecContext(ctx, `INSERT INTO step_attempts (id, project_id, run_id, step_id, revision_round, attempt, generation, status, assignment_id, input_manifest, created_at, started_at, ended_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)`,
				a.ID, st.run.ProjectID, st.run.ID, n.ID, a.Round, a.Attempt, a.Generation, status, a.AssignmentID, string(manifestJSON), now, started, ended); err != nil {
				return err
			}
		}
		st.attempts[n.ID] = a
		if status == StWaitingApproval {
			if _, err := c.Tx.ExecContext(ctx, `INSERT INTO approvals (id, project_id, run_id, step_attempt_id, generation, kind, target_manifest, created_at) VALUES (?, ?, ?, ?, ?, 'step', ?, ?)`,
				storage.NewID("apr"), st.run.ProjectID, st.run.ID, a.ID, a.Generation, string(manifestJSON), now); err != nil {
				return err
			}
		}
		if err := c.Emit(st.run.ProjectID, st.run.ID, a.ID, "step."+status, map[string]any{
			"stepId": n.ID, "title": n.Title, "generation": a.Generation, "attempt": a.Attempt, "round": a.Round, "status": status,
		}); err != nil {
			return err
		}
		if extra != nil {
			if err := extra(c, st, a); err != nil {
				return err
			}
		}
		id = a.ID
		return saveRunStatus(ctx, c, st)
	})
	return id, err
}

// runCondition evaluates a condition node, then marks every node of the
// branches not taken as skipped so joins do not wait for them (T09).
func (e *Engine) runCondition(ctx context.Context, seen *runState, n *domain.Node) error {
	r := n.Routing
	src := seen.attempts[r.Source.FromStep]
	var path string
	err := e.db.Read().QueryRowContext(ctx, `SELECT path FROM artifacts WHERE step_attempt_id = ? AND output_key = ? AND validity = 'valid' ORDER BY version DESC LIMIT 1`,
		src.ID, r.Source.OutputKey).Scan(&path)
	var target string
	if err == nil {
		var doc []byte
		doc, err = os.ReadFile(filepath.Join(e.cfg.DataDir, path))
		if err == nil {
			target, err = domain.EvaluateRouting(r, doc)
		}
	}
	if err != nil {
		reason := "분기 판단 실패: " + err.Error()
		_, cerr := e.createAttempt(ctx, seen, n, StFailed, func(c *storage.Change, st *runState, a attemptRow) error {
			_, err := c.Tx.ExecContext(ctx, `UPDATE step_attempts SET error = ? WHERE id = ?`, reason, a.ID)
			return err
		})
		return cerr
	}
	_, err = e.createAttempt(ctx, seen, n, StSucceeded, func(c *storage.Change, st *runState, a attemptRow) error {
		if err := c.Emit(st.run.ProjectID, st.run.ID, a.ID, "condition.routed", map[string]string{"stepId": n.ID, "target": target}); err != nil {
			return err
		}
		for _, t := range st.graph.BranchTargets(n.ID) {
			if t == target {
				continue
			}
			for member := range st.graph.BranchMembers(n.ID, t) {
				if st.status(member) != StPending {
					continue
				}
				gen := st.run.Gens[member]
				sid := storage.NewID("att")
				now := storage.Now()
				if _, err := c.Tx.ExecContext(ctx, `INSERT INTO step_attempts (id, project_id, run_id, step_id, revision_round, attempt, generation, status, created_at, started_at, ended_at)
					VALUES (?, ?, ?, ?, ?, 1, ?, 'skipped', ?, ?, ?)`, sid, st.run.ProjectID, st.run.ID, member, gen.R, gen.G, now, now, now); err != nil {
					return err
				}
				st.attempts[member] = attemptRow{ID: sid, StepID: member, Status: StSkipped, Generation: gen.G}
				if err := c.Emit(st.run.ProjectID, st.run.ID, sid, "step.skipped", map[string]any{"stepId": member, "generation": gen.G, "by": n.ID}); err != nil {
					return err
				}
			}
		}
		return nil
	})
	return err
}
