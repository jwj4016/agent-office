package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// outputPath is where a step's output file lives inside the attempt dir.
func outputPath(dir string, o domain.Output) string {
	ext := ".md"
	switch o.Type {
	case domain.OutJSON, domain.OutReport, domain.OutCodeChange:
		ext = ".json"
	case domain.OutFile:
		ext = ""
	}
	return filepath.Join(dir, "out", o.Key+ext)
}

// workspaceDir is where the provider works: the project's configured
// workspace, or a per-run scratch directory.
func (e *Engine) workspaceDir(st *runState) (string, error) {
	if p := st.version.Policy.WorkspacePath; p != "" {
		return p, nil
	}
	dir := filepath.Join(e.projectDir(st.run.ProjectID), "runs", st.run.ID, "work")
	return dir, os.MkdirAll(dir, 0o700)
}

// startAI creates a running attempt and starts its provider session.
// Caller holds e.mu.
func (e *Engine) startAI(ctx context.Context, seen *runState, n *domain.Node, snap domain.AssignmentSnapshot) error {
	conn, ok := e.connectionReady(ctx, snap.ConnectionID)
	var prov providers.Provider
	var perr error
	if ok {
		prov, perr = e.cfg.Providers(conn)
	} else {
		perr = fmt.Errorf("AI 연결이 없거나 확인되지 않았습니다")
	}
	if perr != nil {
		_, err := e.createAttempt(ctx, seen, n, StFailed, func(c *storage.Change, st *runState, a attemptRow) error {
			_, err := c.Tx.ExecContext(ctx, `UPDATE step_attempts SET error = ? WHERE id = ?`, perr.Error(), a.ID)
			return err
		})
		return err
	}
	work, err := e.workspaceDir(seen)
	if err != nil {
		return err
	}

	var req providers.StartRequest
	id, err := e.createAttempt(ctx, seen, n, StRunning, func(c *storage.Change, st *runState, a attemptRow) error {
		dir := e.attemptDir(st.run.ProjectID, st.run.ID, a.ID)
		if err := os.MkdirAll(filepath.Join(dir, "out"), 0o700); err != nil {
			return err
		}
		var manifest []ManifestEntry
		json.Unmarshal(a.InputManifest, &manifest)
		feedback, err := feedbackFor(ctx, c.Tx, st, n.ID)
		if err != nil {
			return err
		}
		notes, err := stepContext(ctx, c.Tx, st, n, manifest)
		if err != nil {
			return err
		}
		req = providers.StartRequest{
			ProjectID: st.run.ProjectID, RunID: st.run.ID, StepAttemptID: a.ID, StepID: n.ID, Generation: a.Generation,
			Instructions: domain.ComposeInstructions(instructionLayers(snap, n, feedback)),
			Prompt:       buildPrompt(e.cfg.DataDir, st, n, manifest, notes, dir),
			Workspace:    work,
			WritableDirs: []string{filepath.Join(dir, "out")},
			Model:        snap.Model,
			Policy:       providers.Policy{Sandbox: "workspace-write", AskApproval: true},
		}
		for _, m := range manifest {
			if m.ArtifactID != "" {
				req.InputManifest = append(req.InputManifest, providers.InputRef{Name: m.Name, ArtifactID: m.ArtifactID, Path: filepath.Join(e.cfg.DataDir, m.Path), Hash: m.Hash})
			}
		}
		for _, o := range n.Outputs {
			req.OutputSpec = append(req.OutputSpec, providers.OutputSpec{Key: o.Key, Type: string(o.Type), Required: o.IsRequired(), Schema: o.Schema, Path: outputPath(dir, o)})
		}
		if n.Limits != nil && n.Limits.Timeout != "" {
			req.Limits.Timeout, _ = time.ParseDuration(n.Limits.Timeout)
		}
		return nil
	})
	if err != nil || id == "" {
		return err
	}
	session, startErr := prov.Start(e.ctx, req)
	if startErr != nil {
		// A start failure (bad executable path, missing login…) is an
		// ordinary step failure, never an engine crash.
		reason := "공급자 시작 실패: " + startErr.Error()
		_, err := e.db.Change(ctx, func(c *storage.Change) error {
			st, err := loadState(ctx, c.Tx, seen.run.ID)
			if err != nil {
				return err
			}
			if _, err := setAttemptStatus(ctx, c, st, st.attempts[n.ID], StFailed, reason, StRunning); err != nil {
				return err
			}
			return saveRunStatus(ctx, c, st)
		})
		return err
	}
	a := &activeAttempt{id: id, projectID: seen.run.ProjectID, runID: seen.run.ID, stepID: n.ID, generation: req.Generation,
		session: session, questions: map[string]string{}, toolReqs: map[string]string{}}
	e.active[id] = a
	e.wg.Add(1)
	go e.pump(a, n)
	return nil
}

// pump relays one session's events until it completes.
func (e *Engine) pump(a *activeAttempt, n *domain.Node) {
	defer e.wg.Done()
	for ev := range a.session.Events() {
		if err := e.safeEvent(a, n, ev); err != nil {
			e.cfg.Logf("engine: attempt %s event %s: %v", a.id, ev.Kind, err)
		}
	}
	e.mu.Lock()
	delete(e.active, a.id)
	e.mu.Unlock()
	e.Wake()
}

// safeEvent handles one provider event, turning a panic into an error so
// a malformed event cannot crash the app.
func (e *Engine) safeEvent(a *activeAttempt, n *domain.Node, ev providers.Event) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return e.onEvent(a, n, ev)
}

func (e *Engine) onEvent(a *activeAttempt, n *domain.Node, ev providers.Event) error {
	ctx := context.Background()
	if ev.Kind == providers.KindMessageDelta {
		if e.cfg.Ephemeral != nil {
			e.cfg.Ephemeral(a.projectID, ev)
		}
		return nil
	}
	if ev.Kind == providers.KindCompleted {
		var p providers.CompletedPayload
		json.Unmarshal(ev.Payload, &p)
		return e.complete(ctx, a, n, p)
	}
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, a.runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, a.id)
		if err != nil {
			return err
		}
		if !st.isCurrent(row) || !isLive(row.Status) {
			// Late event from a superseded or cancelled attempt: keep it as
			// history only (T11).
			return c.Emit(a.projectID, a.runID, a.id, "provider.late", map[string]any{"kind": ev.Kind, "payload": ev.Payload})
		}
		switch ev.Kind {
		case providers.KindStarted:
			var p providers.StartedPayload
			json.Unmarshal(ev.Payload, &p)
			if _, err := c.Tx.ExecContext(ctx, `UPDATE step_attempts SET provider_session = ? WHERE id = ?`, p.SessionID, a.id); err != nil {
				return err
			}
		case providers.KindApprovalRequest:
			var p providers.RequestPayload
			json.Unmarshal(ev.Payload, &p)
			apr := storage.NewID("apr")
			target, _ := json.Marshal(map[string]string{"action": p.Action, "detail": p.Detail})
			if _, err := c.Tx.ExecContext(ctx, `INSERT INTO approvals (id, project_id, run_id, step_attempt_id, generation, kind, request_key, target_manifest, created_at)
				VALUES (?, ?, ?, ?, ?, 'tool', ?, ?, ?)`, apr, a.projectID, a.runID, a.id, a.generation, p.RequestID, string(target), storage.Now()); err != nil {
				return err
			}
			a.mu.Lock()
			a.toolReqs[apr] = p.RequestID
			a.mu.Unlock()
			if _, err := setAttemptStatus(ctx, c, st, row, StWaitingApproval, "", StRunning, StWaitingInput); err != nil {
				return err
			}
			if err := c.Emit(a.projectID, a.runID, a.id, "approval.requested", map[string]any{"approvalId": apr, "kind": "tool", "action": p.Action, "detail": p.Detail}); err != nil {
				return err
			}
			return saveRunStatus(ctx, c, st)
		case providers.KindQuestion:
			var p providers.RequestPayload
			json.Unmarshal(ev.Payload, &p)
			msg := storage.NewID("msg")
			if _, err := c.Tx.ExecContext(ctx, `INSERT INTO messages (id, project_id, run_id, step_attempt_id, sender, recipient, kind, body, created_at)
				VALUES (?, ?, ?, ?, ?, ?, 'question', ?, ?)`, msg, a.projectID, a.runID, a.id, n.AssignmentID, domain.LocalOwner, p.Detail, storage.Now()); err != nil {
				return err
			}
			a.mu.Lock()
			a.questions[msg] = p.RequestID
			a.mu.Unlock()
			if _, err := setAttemptStatus(ctx, c, st, row, StWaitingInput, "", StRunning, StWaitingApproval); err != nil {
				return err
			}
			if err := c.Emit(a.projectID, a.runID, a.id, "message.question", map[string]any{"messageId": msg, "body": p.Detail}); err != nil {
				return err
			}
			return saveRunStatus(ctx, c, st)
		}
		return c.Emit(a.projectID, a.runID, a.id, "provider."+ev.Kind, json.RawMessage(ev.Payload))
	})
	return err
}

// complete records a provider's end. A provider "success" only starts
// verification; the step succeeds only if its outputs check out (T15).
func (e *Engine) complete(ctx context.Context, a *activeAttempt, n *domain.Node, p providers.CompletedPayload) error {
	dir := e.attemptDir(a.projectID, a.runID, a.id)
	current := false
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, a.runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, a.id)
		if err != nil {
			return err
		}
		if !st.isCurrent(row) {
			// Superseded by rework or run cancelled while running.
			next := StSuperseded
			if st.run.Status == RunCancelled {
				next = StCancelled
			}
			if _, err := setAttemptStatus(ctx, c, st, row, next, "", StRunning, StVerifying, StWaitingApproval, StWaitingInput); err != nil {
				return err
			}
			return c.Emit(a.projectID, a.runID, a.id, "provider.late", map[string]any{"kind": "completed", "status": p.Status})
		}
		switch p.Status {
		case providers.StatusSucceeded:
			current = true
			_, err = setAttemptStatus(ctx, c, st, row, StVerifying, "", StRunning, StWaitingApproval, StWaitingInput)
		case providers.StatusCancelled:
			_, err = setAttemptStatus(ctx, c, st, row, StCancelled, "", StRunning, StWaitingApproval, StWaitingInput)
		case providers.StatusInterrupted:
			_, err = setAttemptStatus(ctx, c, st, row, StInterrupted, p.Error, StRunning, StWaitingApproval, StWaitingInput)
		default:
			msg := p.Error
			if msg == "" {
				msg = "공급자가 실패를 보고했습니다"
			}
			_, err = setAttemptStatus(ctx, c, st, row, StFailed, msg, StRunning, StWaitingApproval, StWaitingInput)
		}
		if err != nil {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	if err != nil || !current {
		return err
	}
	fallbackText(dir, n, p.Text)
	work, _ := e.workspaceDir(&runState{run: runRow{ID: a.runID, ProjectID: a.projectID}, version: e.versionFor(ctx, a.runID)})
	results, verr := e.verifyOutputs(ctx, n, dir, work)
	if verr == nil {
		results, verr = e.freeze(a.projectID, a.runID, a.id, results)
	}
	return e.finishVerification(ctx, a.runID, a.id, n, results, p.Text, verr)
}

func (e *Engine) versionFor(ctx context.Context, runID string) domain.WorkflowVersion {
	r, err := loadRunRow(ctx, e.db.Read(), runID)
	if err != nil {
		return domain.WorkflowVersion{}
	}
	v, _ := loadVersion(ctx, e.db.Read(), r.VersionID)
	return v
}

// fallbackText lets a text-only provider satisfy a single required text
// output: its final message becomes the file if the file is missing.
func fallbackText(dir string, n *domain.Node, text string) {
	if text == "" {
		return
	}
	var required []domain.Output
	for _, o := range n.Outputs {
		if o.IsRequired() {
			required = append(required, o)
		}
	}
	if len(required) != 1 {
		return
	}
	o := required[0]
	path := outputPath(dir, o)
	if _, err := os.Lstat(path); err == nil {
		return
	}
	switch o.Type {
	case domain.OutMarkdown:
	case domain.OutJSON, domain.OutReport:
		if !json.Valid([]byte(text)) {
			return
		}
	default:
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o700)
	os.WriteFile(path, []byte(text), 0o600)
}

// finishVerification stores artifacts and marks the attempt succeeded,
// or fails it with the verification problems. Nothing is stored if the
// attempt stopped being current while verifying. note is the provider's
// final message, handed to the steps that use these results.
func (e *Engine) finishVerification(ctx context.Context, runID, attemptID string, n *domain.Node, results []verifiedOutput, note string, verr error) error {
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, attemptID)
		if err != nil {
			return err
		}
		if !st.isCurrent(row) {
			next := StSuperseded
			if st.run.Status == RunCancelled {
				next = StCancelled
			}
			_, err := setAttemptStatus(ctx, c, st, row, next, "", StVerifying, StWaitingHuman)
			return err
		}
		if verr != nil {
			if _, err := setAttemptStatus(ctx, c, st, row, StFailed, "완료 기준 미충족: "+verr.Error(), StVerifying, StWaitingHuman); err != nil {
				return err
			}
			return saveRunStatus(ctx, c, st)
		}
		refs, err := storeArtifacts(ctx, c, st, row, results)
		if err != nil {
			return err
		}
		if err := postHandoffs(ctx, c, st, row, refs, note); err != nil {
			return err
		}
		if _, err := setAttemptStatus(ctx, c, st, row, StSucceeded, "", StVerifying, StWaitingHuman); err != nil {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	e.Wake()
	return err
}

func storeArtifacts(ctx context.Context, c *storage.Change, st *runState, a attemptRow, results []verifiedOutput) ([]ArtifactRef, error) {
	var refs []ArtifactRef
	for _, r := range results {
		var version int
		c.Tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(a.version), 0) + 1 FROM artifacts a JOIN step_attempts s ON s.id = a.step_attempt_id
			WHERE a.run_id = ? AND s.step_id = ? AND a.output_key = ?`, st.run.ID, a.StepID, r.Key).Scan(&version)
		id := storage.NewID("art")
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO artifacts (id, project_id, run_id, step_attempt_id, output_key, version, type, path, hash, size, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, st.run.ProjectID, st.run.ID, a.ID, r.Key, version, r.Type, r.RelPath, r.Hash, r.Size, storage.Now()); err != nil {
			return nil, err
		}
		if err := c.Emit(st.run.ProjectID, st.run.ID, a.ID, "artifact.created", map[string]any{
			"artifactId": id, "stepId": a.StepID, "outputKey": r.Key, "version": version, "type": r.Type, "hash": r.Hash,
		}); err != nil {
			return nil, err
		}
		refs = append(refs, ArtifactRef{ID: id, StepID: a.StepID, OutputKey: r.Key, Version: version, Hash: r.Hash})
	}
	return refs, nil
}
