package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-office/internal/domain"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// Outcome reports what a human action did. Already is true when the item
// had been decided before (a repeated click): nothing changed.
type Outcome struct {
	Status   string `json:"status"`
	Already  bool   `json:"already"`
	Decision string `json:"decision,omitempty"`
}

// VerificationError is returned when a human submission does not meet
// the step's completion criteria; the step stays open for resubmission.
type VerificationError struct{ Problem string }

func (e *VerificationError) Error() string { return "완료 기준 미충족: " + e.Problem }

// humanTarget loads an attempt for a human action and checks it belongs
// to the project, is current and is waiting in the expected status.
func (e *Engine) humanTarget(ctx context.Context, q querier, projectID, attemptID string, generation int, want string) (*runState, attemptRow, *domain.Node, error) {
	a, runID, err := loadAttempt(ctx, q, attemptID)
	if err != nil {
		return nil, a, nil, err
	}
	st, err := loadState(ctx, q, runID)
	if err != nil {
		return nil, a, nil, err
	}
	if st.run.ProjectID != projectID {
		return nil, a, nil, storage.ErrNotInProject
	}
	n := st.graph.Node(a.StepID)
	if a.Generation != generation || !st.isCurrent(a) {
		return st, a, n, ErrStale
	}
	if a.Status != want {
		return st, a, n, nil // caller decides: already done or wrong state
	}
	return st, a, n, nil
}

// stage writes submitted text outputs to a fresh directory so concurrent
// submissions never overwrite each other's files.
func (e *Engine) stage(projectID, runID, attemptID string, n *domain.Node, outputs map[string]string) (string, error) {
	dir := filepath.Join(e.attemptDir(projectID, runID, attemptID), "submissions", storage.NewID("sub"))
	for key, content := range outputs {
		var out *domain.Output
		for i := range n.Outputs {
			if n.Outputs[i].Key == key {
				out = &n.Outputs[i]
			}
		}
		if out == nil {
			return "", fmt.Errorf("%w: 정의되지 않은 결과 %q", ErrInvalid, key)
		}
		path := outputPath(dir, *out)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return "", err
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// SubmitHumanResult completes a human task with the given text outputs.
func (e *Engine) SubmitHumanResult(ctx context.Context, projectID, attemptID string, generation int, outputs map[string]string) (Outcome, error) {
	if err := e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefStepAttempt, ID: attemptID}); err != nil {
		return Outcome{}, err
	}
	st, a, n, err := e.humanTarget(ctx, e.db.Read(), projectID, attemptID, generation, StWaitingHuman)
	if err != nil {
		if errors.Is(err, ErrStale) && a.Status == StSucceeded {
			return Outcome{Status: a.Status, Already: true}, nil
		}
		return Outcome{}, err
	}
	if a.Status != StWaitingHuman {
		return Outcome{Status: a.Status, Already: a.Status == StSucceeded}, nil
	}
	if n.Kind != domain.KindTask {
		return Outcome{}, fmt.Errorf("%w: 리뷰는 SubmitReview로 제출합니다", ErrInvalid)
	}
	ws, err := e.humanWorkspace(ctx, st, n, a.ID)
	if err != nil {
		return Outcome{}, err
	}
	dir, err := e.stage(projectID, st.run.ID, a.ID, n, outputs)
	if err != nil {
		return Outcome{}, err
	}
	// A person's code work is committed from their worktree like an AI's.
	verr := e.captureCode(ctx, st, n, a.ID, dir)
	var results []verifiedOutput
	if verr == nil {
		results, verr = e.verifyOutputs(ctx, n, dir, ws.Dir)
	}
	if verr != nil {
		return Outcome{}, &VerificationError{Problem: verr.Error()}
	}
	if results, err = e.freeze(projectID, st.run.ID, a.ID, results); err != nil {
		return Outcome{}, err
	}
	return e.commitHuman(ctx, projectID, st.run.ID, a, results, "사람(나)이 제출한 결과입니다.", nil)
}

// humanWorkspace returns where a person's step works. In a Git run a
// code step must have its worktree before anything can be submitted.
func (e *Engine) humanWorkspace(ctx context.Context, st *runState, n *domain.Node, attemptID string) (stepWorkspace, error) {
	ws, err := e.attemptWorkspace(ctx, e.db.Read(), st, attemptID)
	if err != nil {
		return ws, err
	}
	if rr, ok := loadRunRepo(ctx, e.db.Read(), st.run.ID); ok && rr.Kind == WsBase && isCodeStep(n) && ws.Kind != WsCode {
		return ws, fmt.Errorf("%w: 작업 공간을 준비하는 중입니다. 잠시 후 다시 제출하세요", ErrInvalid)
	}
	return ws, nil
}

// commitHuman stores a verified human result exactly once. note is
// handed to the steps that use the results.
func (e *Engine) commitHuman(ctx context.Context, projectID, runID string, a attemptRow, results []verifiedOutput, note string, after func(c *storage.Change, st *runState) error) (Outcome, error) {
	out := Outcome{Status: StSucceeded}
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, a.ID)
		if err != nil {
			return err
		}
		if row.Status == StSucceeded || row.Status == StSuperseded {
			out = Outcome{Status: row.Status, Already: true}
			return nil
		}
		if !st.isCurrent(row) || row.Status != StWaitingHuman {
			return ErrStale
		}
		refs, err := storeArtifacts(ctx, c, st, row, results)
		if err != nil {
			return err
		}
		if after != nil {
			return after(c, st)
		}
		if err := postHandoffs(ctx, c, st, row, refs, note); err != nil {
			return err
		}
		ok, err := setAttemptStatus(ctx, c, st, row, StSucceeded, "", StWaitingHuman)
		if err != nil || !ok {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	if err == nil {
		e.Wake()
	}
	return out, err
}

// Review decisions.
const (
	ReviewPass             = "pass"
	ReviewChangesRequested = "changes_requested"
)

// SubmitReview records a human review. The engine writes the review
// report (decision, comment and the reviewed input versions); outputs
// holds any other outputs the review step defines. Passing completes the
// step, so it must meet the step's full completion criteria; requesting
// changes sends the chosen targets back for a new round (T05) and only
// stores the report.
func (e *Engine) SubmitReview(ctx context.Context, projectID, attemptID string, generation int, decision, comment string, targets []string, outputs map[string]string) (Outcome, error) {
	if err := e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefStepAttempt, ID: attemptID}); err != nil {
		return Outcome{}, err
	}
	st, a, n, err := e.humanTarget(ctx, e.db.Read(), projectID, attemptID, generation, StWaitingHuman)
	if err != nil {
		if errors.Is(err, ErrStale) && (a.Status == StSucceeded || a.Status == StSuperseded) {
			return Outcome{Status: a.Status, Already: true}, nil
		}
		return Outcome{}, err
	}
	if a.Status != StWaitingHuman {
		return Outcome{Status: a.Status, Already: true}, nil
	}
	if n.Kind != domain.KindReview {
		return Outcome{}, fmt.Errorf("%w: 리뷰 업무가 아닙니다", ErrInvalid)
	}
	if decision != ReviewPass && decision != ReviewChangesRequested {
		return Outcome{}, fmt.Errorf("%w: decision은 pass 또는 changes_requested입니다", ErrInvalid)
	}
	if decision == ReviewChangesRequested {
		if err := checkReworkRequest(n, targets, comment); err != nil {
			return Outcome{}, err
		}
	}
	var manifest []ManifestEntry
	json.Unmarshal(a.InputManifest, &manifest)
	report, _ := json.MarshalIndent(map[string]any{
		"decision": decision, "comment": comment, "reworkTargets": targets,
		"reviewer": domain.LocalOwner, "generation": a.Generation, "reviewedInputs": manifest, "at": storage.Now(),
	}, "", "  ")
	reportKey := ""
	for _, o := range n.Outputs {
		if o.Type == domain.OutReport {
			reportKey = o.Key
			break
		}
	}
	staged := map[string]string{}
	if reportKey != "" {
		staged[reportKey] = string(report)
	}
	if decision == ReviewPass {
		for k, v := range outputs {
			if k == reportKey {
				return Outcome{}, fmt.Errorf("%w: 리뷰 보고서 %q는 앱이 작성합니다", ErrInvalid, k)
			}
			staged[k] = v
		}
	}
	dir, err := e.stage(projectID, st.run.ID, a.ID, n, staged)
	if err != nil {
		return Outcome{}, err
	}
	if decision == ReviewPass {
		ws, err := e.humanWorkspace(ctx, st, n, a.ID)
		if err != nil {
			return Outcome{}, err
		}
		results, verr := e.verifyOutputs(ctx, n, dir, ws.Dir)
		if verr != nil {
			return Outcome{}, &VerificationError{Problem: verr.Error()}
		}
		if results, err = e.freeze(projectID, st.run.ID, a.ID, results); err != nil {
			return Outcome{}, err
		}
		note := "사람(나)의 리뷰: 통과"
		if strings.TrimSpace(comment) != "" {
			note += "\n" + comment
		}
		out, err := e.commitHuman(ctx, projectID, st.run.ID, a, results, note, nil)
		out.Decision = decision
		return out, err
	}
	var results []verifiedOutput
	for _, o := range n.Outputs {
		if o.Key == reportKey {
			v, err := e.verifyOne(o, outputPath(dir, o))
			if err != nil {
				return Outcome{}, &VerificationError{Problem: err.Error()}
			}
			results = append(results, v)
		}
	}
	if results, err = e.freeze(projectID, st.run.ID, a.ID, results); err != nil {
		return Outcome{}, err
	}
	var live []string
	out, err := e.commitHuman(ctx, projectID, st.run.ID, a, results, "", func(c *storage.Change, st *runState) error {
		if err := c.Emit(projectID, st.run.ID, a.ID, "review.changes_requested", map[string]any{"stepId": n.ID, "targets": targets, "comment": comment}); err != nil {
			return err
		}
		var err error
		live, err = e.reworkTx(ctx, c, st, n, a.ID, targets, comment)
		if err != nil {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	out.Decision = decision
	if out.Status == StSucceeded && !out.Already {
		out.Status = StSuperseded
	}
	e.cancelLive(live)
	return out, err
}

// Approval decisions.
const (
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	ApprovalHeld     = "held"
)

// DecideApproval decides a step approval. Repeating a decision returns
// the recorded one; a decision for an old generation or a cancelled run
// is rejected as stale (spec §7.3). Holding records the reason without
// deciding, and nothing is ever approved by silence.
func (e *Engine) DecideApproval(ctx context.Context, projectID, approvalID string, generation int, decision, reason string, targets []string) (Outcome, error) {
	if err := e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefApproval, ID: approvalID}); err != nil {
		return Outcome{}, err
	}
	if decision != ApprovalApproved && decision != ApprovalRejected && decision != ApprovalHeld {
		return Outcome{}, fmt.Errorf("%w: decision은 approved, rejected, held 중 하나입니다", ErrInvalid)
	}
	out := Outcome{Decision: decision}
	var live []string
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		var runID, attemptID, kind string
		var gen int
		var existing sql.NullString
		if err := c.Tx.QueryRowContext(ctx, `SELECT run_id, step_attempt_id, generation, kind, decision FROM approvals WHERE id = ?`, approvalID).
			Scan(&runID, &attemptID, &gen, &kind, &existing); err != nil {
			return err
		}
		if kind != "step" {
			return fmt.Errorf("%w: 도구 승인은 DecideToolApproval로 결정합니다", ErrInvalid)
		}
		if existing.Valid {
			out = Outcome{Status: existing.String, Already: true, Decision: existing.String}
			if existing.String != decision && decision != ApprovalHeld {
				return fmt.Errorf("%w: 이미 %s로 결정되었습니다", ErrStale, existing.String)
			}
			return nil
		}
		st, err := loadState(ctx, c.Tx, runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, attemptID)
		if err != nil {
			return err
		}
		if gen != generation || !st.isCurrent(row) || row.Status != StWaitingApproval {
			return ErrStale
		}
		n := st.graph.Node(row.StepID)
		switch decision {
		case ApprovalHeld:
			out.Status = StWaitingApproval
			return c.Emit(projectID, runID, attemptID, "approval.held", map[string]string{"approvalId": approvalID, "reason": reason})
		case ApprovalRejected:
			if err := checkReworkRequest(n, targets, reason); err != nil {
				return err
			}
		}
		targetsJSON, _ := json.Marshal(targets)
		res, err := c.Tx.ExecContext(ctx, `UPDATE approvals SET decision = ?, reason = ?, rework_targets = ?, decided_at = ? WHERE id = ? AND decision IS NULL`,
			decision, reason, string(targetsJSON), storage.Now(), approvalID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrStale
		}
		if err := c.Emit(projectID, runID, attemptID, "approval.decided", map[string]any{"approvalId": approvalID, "decision": decision, "reason": reason, "targets": targets}); err != nil {
			return err
		}
		if decision == ApprovalApproved {
			out.Status = StSucceeded
			if _, err := setAttemptStatus(ctx, c, st, row, StSucceeded, "", StWaitingApproval); err != nil {
				return err
			}
		} else {
			out.Status = StSuperseded
			if live, err = e.reworkTx(ctx, c, st, n, attemptID, targets, reason); err != nil {
				return err
			}
		}
		return saveRunStatus(ctx, c, st)
	})
	if err != nil {
		return Outcome{}, err
	}
	e.cancelLive(live)
	e.Wake()
	return out, nil
}

// DecideToolApproval answers a provider's tool permission request. A
// declined action is not executed; the decision is recorded (T12).
func (e *Engine) DecideToolApproval(ctx context.Context, projectID, approvalID string, accept bool) (Outcome, error) {
	if err := e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefApproval, ID: approvalID}); err != nil {
		return Outcome{}, err
	}
	var attemptID, requestKey string
	e.db.Read().QueryRowContext(ctx, `SELECT step_attempt_id, request_key FROM approvals WHERE id = ? AND kind = 'tool'`, approvalID).Scan(&attemptID, &requestKey)
	live := e.lookupActive(attemptID)
	if live == nil {
		return Outcome{}, ErrStale
	}
	decision := ApprovalRejected
	if accept {
		decision = ApprovalApproved
	}
	out := Outcome{Status: StRunning, Decision: decision}
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		var existing sql.NullString
		var runID string
		if err := c.Tx.QueryRowContext(ctx, `SELECT run_id, decision FROM approvals WHERE id = ?`, approvalID).Scan(&runID, &existing); err != nil {
			return err
		}
		if existing.Valid {
			out = Outcome{Status: existing.String, Already: true, Decision: existing.String}
			return nil
		}
		st, err := loadState(ctx, c.Tx, runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, attemptID)
		if err != nil {
			return err
		}
		if !st.isCurrent(row) || row.Status != StWaitingApproval {
			return ErrStale
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE approvals SET decision = ?, decided_at = ? WHERE id = ? AND decision IS NULL`, decision, storage.Now(), approvalID); err != nil {
			return err
		}
		if err := c.Emit(projectID, runID, attemptID, "approval.decided", map[string]any{"approvalId": approvalID, "kind": "tool", "decision": decision}); err != nil {
			return err
		}
		if _, err := setAttemptStatus(ctx, c, st, row, StRunning, "", StWaitingApproval); err != nil {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	if err != nil || out.Already {
		return out, err
	}
	pd := providers.DecisionDecline
	if accept {
		pd = providers.DecisionAccept
	}
	if err := live.respond(ctx, providers.Response{RequestID: requestKey, Decision: pd}); err != nil {
		return out, fmt.Errorf("공급자에 전달 실패: %w", err)
	}
	return out, nil
}

// AnswerQuestion answers an AI's question and resumes the attempt. Only
// questions addressed to the person, or passed on to the person after an
// AI could not answer, are the person's to answer.
func (e *Engine) AnswerQuestion(ctx context.Context, projectID, messageID, answer string) (Outcome, error) {
	if strings.TrimSpace(answer) == "" {
		return Outcome{}, fmt.Errorf("%w: 답변이 비어 있습니다", ErrInvalid)
	}
	if err := e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefMessage, ID: messageID}); err != nil {
		return Outcome{}, err
	}
	var recipient string
	var escalated int
	e.db.Read().QueryRowContext(ctx, `SELECT recipient, (SELECT COUNT(*) FROM messages e WHERE e.reply_to = m.id AND e.kind = 'escalation')
		FROM messages m WHERE id = ? AND kind = 'question'`, messageID).Scan(&recipient, &escalated)
	if recipient != domain.LocalOwner && escalated == 0 {
		return Outcome{}, fmt.Errorf("%w: 다른 AI 담당자에게 보낸 질문입니다", ErrInvalid)
	}
	return e.deliverAnswer(ctx, messageID, domain.LocalOwner, answer)
}

// deliverAnswer records an answer to a pending question exactly once and
// passes it to the asking session, which then continues.
func (e *Engine) deliverAnswer(ctx context.Context, messageID, sender, answer string) (Outcome, error) {
	var attemptID, runID, projectID string
	e.db.Read().QueryRowContext(ctx, `SELECT COALESCE(step_attempt_id, ''), run_id, project_id FROM messages WHERE id = ? AND kind = 'question'`, messageID).
		Scan(&attemptID, &runID, &projectID)
	live := e.lookupActive(attemptID)
	if live == nil {
		return Outcome{}, ErrStale
	}
	live.mu.Lock()
	requestID, ok := live.questions[messageID]
	live.mu.Unlock()
	if !ok {
		return Outcome{Status: StRunning, Already: true}, nil
	}
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		var answered int
		c.Tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE reply_to = ? AND kind = 'answer'`, messageID).Scan(&answered)
		if answered > 0 {
			return ErrStale
		}
		st, err := loadState(ctx, c.Tx, runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, attemptID)
		if err != nil {
			return err
		}
		// An AI-to-AI question keeps the asker running; one passed to the
		// person made it wait for input.
		if !st.isCurrent(row) || (row.Status != StWaitingInput && row.Status != StRunning) {
			return ErrStale
		}
		if _, err := addMessage(ctx, c, projectID, newMessage{
			RunID: runID, AttemptID: attemptID, Sender: sender, Recipient: row.AssignmentID, Kind: MsgAnswer, Body: answer, ReplyTo: messageID,
		}); err != nil {
			return err
		}
		if row.Status == StWaitingInput && !e.waitsForOtherQuestions(ctx, c.Tx, attemptID, messageID) {
			if _, err := setAttemptStatus(ctx, c, st, row, StRunning, "", StWaitingInput); err != nil {
				return err
			}
		}
		return saveRunStatus(ctx, c, st)
	})
	if err != nil {
		return Outcome{}, err
	}
	live.mu.Lock()
	delete(live.questions, messageID)
	live.mu.Unlock()
	return Outcome{Status: StRunning}, live.respond(ctx, providers.Response{RequestID: requestID, Answer: answer})
}

// waitsForOtherQuestions reports whether the attempt still has another
// question passed to the person that is unanswered.
func (e *Engine) waitsForOtherQuestions(ctx context.Context, q querier, attemptID, except string) bool {
	var n int
	q.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages m WHERE m.step_attempt_id = ? AND m.kind = 'question' AND m.id <> ?
		AND NOT EXISTS (SELECT 1 FROM messages r WHERE r.reply_to = m.id AND r.kind = 'answer')
		AND (m.recipient = 'local-owner' OR EXISTS (SELECT 1 FROM messages x WHERE x.reply_to = m.id AND x.kind = 'escalation'))`, attemptID, except).Scan(&n)
	return n > 0
}
