package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"agent-office/internal/domain"
)

// ---- event chains ----

// chainRootExpr is the chain an event belongs to, in SQL (plan §9.2): the
// chain root it names, or its own sequence when it started the chain.
const chainRootExpr = `COALESCE(json_extract(payload, '$.chainRootSequence'), sequence)`

// continueChainTx makes ev an automated event caused by the event at
// causeSeq: it keeps that event's chain and goes one step deeper.
func continueChainTx(ctx context.Context, q queryer, ev *WorkEvent, causeSeq int64) error {
	var payload string
	if err := q.QueryRowContext(ctx, `SELECT payload FROM execution_events WHERE sequence = ?`, causeSeq).Scan(&payload); err != nil {
		return err
	}
	var cause WorkEvent
	json.Unmarshal([]byte(payload), &cause)
	root := cause.ChainRoot(causeSeq)
	ev.CauseSequence, ev.ChainRootSequence, ev.ChainDepth, ev.Automated = &causeSeq, &root, cause.ChainDepth+1, true
	return nil
}

// chainAutoWorkTx counts the work automatic processing registered in a
// chain.
func chainAutoWorkTx(ctx context.Context, q queryer, root int64) (int, error) {
	var n int
	err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM execution_events WHERE kind = ? AND json_extract(payload, '$.automated') = 1
		AND `+chainRootExpr+` = ?`, domain.EventWorkCreated, root).Scan(&n)
	return n, err
}

// ChainHold says why automatic processing may not register more work
// after the event at seq, or "": the chain would go deeper than the
// depth limit, or already registered its share of work (plan §4.7).
func (db *DB) ChainHold(ctx context.Context, seq int64) (string, error) {
	return chainHoldTx(ctx, db.sql, seq)
}

func chainHoldTx(ctx context.Context, q queryer, seq int64) (string, error) {
	var payload string
	if err := q.QueryRowContext(ctx, `SELECT payload FROM execution_events WHERE sequence = ?`, seq).Scan(&payload); err != nil {
		return "", err
	}
	var ev WorkEvent
	json.Unmarshal([]byte(payload), &ev)
	if ev.ChainDepth+1 > domain.DefaultChainDepth {
		return fmt.Sprintf("자동으로 이어진 사건이 %d단계를 넘어 자동 처리를 멈췄습니다. 사람이 결정하세요", domain.DefaultChainDepth), nil
	}
	n, err := chainAutoWorkTx(ctx, q, ev.ChainRoot(seq))
	if err != nil {
		return "", err
	}
	if n >= domain.DefaultChainWork {
		return fmt.Sprintf("이 사건에서 이어진 자동 등록 업무가 한도(%d건)에 닿았습니다. 사람이 결정하세요", domain.DefaultChainWork), nil
	}
	return "", nil
}

// ---- workflows a work item may run ----

// sharedWorkflowVersionTx checks that a version belongs to one of the
// service's ordinary workflows (not a work item's own).
func sharedWorkflowVersionTx(ctx context.Context, q queryer, projectID, versionID string) (WorkflowChoice, error) {
	var c WorkflowChoice
	var own sql.NullString
	err := q.QueryRowContext(ctx, `SELECT v.id, v.workflow_id, w.title, v.number, v.created_at, w.work_item_id FROM workflow_versions v
		JOIN workflows w ON w.id = v.workflow_id WHERE v.id = ? AND v.project_id = ?`, versionID, projectID).
		Scan(&c.VersionID, &c.WorkflowID, &c.Title, &c.Number, &c.CreatedAt, &own)
	if errors.Is(err, sql.ErrNoRows) {
		return c, fmt.Errorf("workflow version %q: %w", versionID, ErrNotInProject)
	}
	if err != nil {
		return c, err
	}
	if own.Valid {
		return c, invalid("업무 전용 흐름은 다른 업무에 연결할 수 없습니다")
	}
	return c, nil
}

// WorkflowChoice is a confirmed version of a service workflow a work item
// can run.
type WorkflowChoice struct {
	VersionID  string `json:"versionId"`
	WorkflowID string `json:"workflowId"`
	Title      string `json:"title"`
	Number     int    `json:"number"`
	CreatedAt  string `json:"createdAt"`
}

// WorkflowChoice returns a confirmed version of one of the service's
// ordinary workflows.
func (db *DB) WorkflowChoice(ctx context.Context, projectID, versionID string) (WorkflowChoice, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return WorkflowChoice{}, err
	}
	return sharedWorkflowVersionTx(ctx, db.sql, projectID, versionID)
}

// WorkflowChoices lists the confirmed versions of the service's ordinary
// workflows, newest version of each workflow first.
func (db *DB) WorkflowChoices(ctx context.Context, projectID string) ([]WorkflowChoice, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT v.id, v.workflow_id, w.title, v.number, v.created_at FROM workflow_versions v
		JOIN workflows w ON w.id = v.workflow_id WHERE v.project_id = ? AND w.work_item_id IS NULL ORDER BY w.title, v.number DESC`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkflowChoice{}
	for rows.Next() {
		var c WorkflowChoice
		if err := rows.Scan(&c.VersionID, &c.WorkflowID, &c.Title, &c.Number, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---- department automation ----

// SetDepartmentAutomation stores what a department lets happen without a
// person. Connections must exist, result types must be known, and
// workflow versions must be the service's own.
func (db *DB) SetDepartmentAutomation(ctx context.Context, projectID, departmentID string, a domain.DepartmentAutomation) (domain.Department, error) {
	a.Categories, a.ConnectionIDs, a.WorkflowVersionIDs = orEmpty(dedupe(a.Categories)), orEmpty(dedupe(a.ConnectionIDs)), orEmpty(dedupe(a.WorkflowVersionIDs))
	var types []domain.OutputType
	for _, t := range a.OutputTypes {
		if !domain.ValidOutputType(t) {
			return domain.Department{}, invalid("알 수 없는 결과 형식입니다: %s", t)
		}
		if !slices.Contains(types, t) {
			types = append(types, t)
		}
	}
	a.OutputTypes = types
	if a.OutputTypes == nil {
		a.OutputTypes = []domain.OutputType{}
	}
	err := db.Write(ctx, func(tx *sql.Tx) error {
		if err := writableProjectTx(ctx, tx, projectID); err != nil {
			return err
		}
		if _, err := departmentTx(ctx, tx, projectID, departmentID); err != nil {
			return err
		}
		for _, id := range a.ConnectionIDs {
			var one int
			if err := tx.QueryRowContext(ctx, `SELECT 1 FROM provider_connections WHERE id = ?`, id).Scan(&one); err != nil {
				return invalid("없는 AI 연결입니다: %s", id)
			}
		}
		for _, v := range a.WorkflowVersionIDs {
			if _, err := sharedWorkflowVersionTx(ctx, tx, projectID, v); err != nil {
				return err
			}
		}
		raw, _ := json.Marshal(a)
		_, err := tx.ExecContext(ctx, `UPDATE departments SET automation = ?, updated_at = ? WHERE id = ? AND project_id = ?`,
			string(raw), Now(), departmentID, projectID)
		return err
	})
	if err != nil {
		return domain.Department{}, err
	}
	return db.Department(ctx, projectID, departmentID)
}

// ---- start approvals ----

// WorkStartHash fingerprints what a start of a work item is approved
// for: the work, its done criteria, its plan (assignee, instructions,
// materials, outputs or workflow version) and its linked material. Any
// change makes an earlier approval void (plan §4.6).
func (db *DB) WorkStartHash(ctx context.Context, projectID, workItemID string) (string, error) {
	w, err := db.WorkItem(ctx, projectID, workItemID)
	if err != nil {
		return "", err
	}
	plan, _, err := db.WorkPlan(ctx, projectID, workItemID)
	if err != nil {
		return "", err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT version_id FROM document_links WHERE work_item_id = ? AND unlinked_at IS NULL ORDER BY version_id`, workItemID)
	if err != nil {
		return "", err
	}
	var links []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			rows.Close()
			return "", err
		}
		links = append(links, v)
	}
	rows.Close()
	b, _ := json.Marshal([]any{w.Title, w.Description, w.DoneCriteria, plan.AssignmentID, plan.Instructions, plan.Materials, plan.Outputs,
		plan.WorkflowVersionID, links})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

// StartApproval is the approval the work's last start was made under.
type StartApproval struct {
	By    string `json:"by"` // user | auto, "" when never approved
	At    string `json:"at"`
	Valid bool   `json:"valid"` // nothing changed since
}

// ApproveWorkStart records that a start of the work was approved for the
// state fingerprinted by hash.
func (db *DB) ApproveWorkStart(ctx context.Context, projectID, workItemID, by, hash string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE work_plans SET approved_hash = ?, approved_by = ?, approved_at = ? WHERE work_item_id = ? AND project_id = ?`,
			hash, by, Now(), workItemID, projectID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return invalid("작업 계획이 없어 시작을 승인할 수 없습니다")
		}
		return nil
	})
}

// WorkStartApproval reports the work's start approval and whether it
// still holds.
func (db *DB) WorkStartApproval(ctx context.Context, projectID, workItemID string) (StartApproval, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefWorkItem, workItemID}); err != nil {
		return StartApproval{}, err
	}
	var hash, by, at sql.NullString
	err := db.sql.QueryRowContext(ctx, `SELECT approved_hash, approved_by, approved_at FROM work_plans WHERE work_item_id = ?`, workItemID).Scan(&hash, &by, &at)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !hash.Valid) {
		return StartApproval{}, nil
	}
	if err != nil {
		return StartApproval{}, err
	}
	cur, err := db.WorkStartHash(ctx, projectID, workItemID)
	return StartApproval{By: by.String, At: at.String, Valid: cur == hash.String}, err
}

// ---- auto starts ----

// Auto-start statuses.
const (
	AutoPending   = "pending"
	AutoPlanning  = "planning"
	AutoApproved  = "approved"
	AutoStarted   = "started"
	AutoFinished  = "finished"
	AutoReview    = "review"
	AutoCancelled = "cancelled"
)

// AutoStart is the auto starter's record of one work item made from an
// event in an auto-mode service.
type AutoStart struct {
	ID            string `json:"id"`
	ProjectID     string `json:"projectId"`
	ProjectName   string `json:"projectName"`
	WorkItemID    string `json:"workItemId"`
	WorkTitle     string `json:"workTitle"`
	DepartmentID  string `json:"departmentId"`
	CauseSequence int64  `json:"causeSequence"`
	Status        string `json:"status"`
	PlanJobID     string `json:"planJobId"`
	RunID         string `json:"runId"`
	Reason        string `json:"reason"` // why it needs a person
	Note          string `json:"note"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

const autoStartSelect = `SELECT s.id, s.project_id, p.name, s.work_item_id, w.title, COALESCE(w.department_id, ''), COALESCE(s.cause_sequence, 0),
	s.status, COALESCE(s.plan_job_id, ''), COALESCE(s.run_id, ''), s.reason, s.note, s.created_at, s.updated_at
	FROM auto_starts s JOIN projects p ON p.id = s.project_id JOIN work_items w ON w.id = s.work_item_id`

func queryAutoStarts(ctx context.Context, q rowsQueryer, where string, args ...any) ([]AutoStart, error) {
	rows, err := q.QueryContext(ctx, autoStartSelect+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AutoStart{}
	for rows.Next() {
		var a AutoStart
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.ProjectName, &a.WorkItemID, &a.WorkTitle, &a.DepartmentID, &a.CauseSequence,
			&a.Status, &a.PlanJobID, &a.RunID, &a.Reason, &a.Note, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// queueAutoStartTx records new work for the auto starter when its service
// is in auto mode and its department starts work on its own. Whether it
// may really start is checked later, when the starter takes it.
func queueAutoStartTx(ctx context.Context, c *Change, w domain.WorkItem, causeSeq int64) error {
	if w.DepartmentID == "" {
		return nil
	}
	p, err := projectTx(ctx, c.Tx, w.ProjectID)
	if err != nil || p.Mode != domain.ModeAuto {
		return err
	}
	d, err := departmentTx(ctx, c.Tx, w.ProjectID, w.DepartmentID)
	if err != nil || !domain.ParseAutomation(d.Automation).AutoStart {
		return err
	}
	now := Now()
	id := NewID("auto")
	if _, err := c.Tx.ExecContext(ctx, `INSERT INTO auto_starts (id, project_id, work_item_id, cause_sequence, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT(work_item_id) DO NOTHING`, id, w.ProjectID, w.ID, causeSeq, now, now); err != nil {
		return err
	}
	return c.Emit(w.ProjectID, "", "", "autostart.updated", map[string]string{"autoStartId": id, "status": AutoPending})
}

// OpenAutoStarts lists what the auto starter still has to move on, in
// active services, oldest first.
func (db *DB) OpenAutoStarts(ctx context.Context, limit int) ([]AutoStart, error) {
	return queryAutoStarts(ctx, db.sql, `p.status = 'active' AND s.status IN ('pending', 'planning', 'approved', 'started')
		ORDER BY s.created_at, s.rowid LIMIT ?`, limit)
}

// AutoStartFilter narrows the auto-start list.
type AutoStartFilter struct {
	WorkItemID string `json:"workItemId"`
	Status     string `json:"status"`
}

// AutoStarts lists a service's auto starts ("" for every active service),
// newest first.
func (db *DB) AutoStarts(ctx context.Context, projectID string, f AutoStartFilter) ([]AutoStart, error) {
	if projectID != "" {
		if err := db.CheckScope(ctx, projectID); err != nil {
			return nil, err
		}
	}
	return queryAutoStarts(ctx, db.sql, `p.status = 'active' AND (? = '' OR s.project_id = ?) AND (? = '' OR s.work_item_id = ?) AND (? = '' OR s.status = ?)
		ORDER BY s.updated_at DESC, s.rowid DESC LIMIT 200`, projectID, projectID, f.WorkItemID, f.WorkItemID, f.Status, f.Status)
}

// AutoStartUpdate moves an auto start on. Empty fields keep their value,
// except Reason, which only a review keeps.
type AutoStartUpdate struct {
	Status, PlanJobID, RunID, Reason, Note string
}

// MoveAutoStart changes an auto start that is still in one of from. It
// reports false when it was not (someone else moved it first).
func (db *DB) MoveAutoStart(ctx context.Context, id string, from []string, u AutoStartUpdate) (bool, error) {
	if len(from) == 0 {
		return false, invalid("이전 상태가 필요합니다")
	}
	moved := false
	_, err := db.Change(ctx, func(c *Change) error {
		var project, status string
		if err := c.Tx.QueryRowContext(ctx, `SELECT project_id, status FROM auto_starts WHERE id = ?`, id).Scan(&project, &status); err != nil {
			return err
		}
		if !slices.Contains(from, status) {
			return nil
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE auto_starts SET status = ?, plan_job_id = COALESCE(NULLIF(?, ''), plan_job_id),
			run_id = COALESCE(NULLIF(?, ''), run_id), reason = CASE WHEN ? = 'review' THEN ? ELSE '' END, note = CASE WHEN ? = '' THEN note ELSE ? END,
			updated_at = ? WHERE id = ?`, u.Status, u.PlanJobID, u.RunID, u.Status, u.Reason, u.Note, u.Note, Now(), id); err != nil {
			return err
		}
		moved = true
		return c.Emit(project, "", "", "autostart.updated", map[string]string{"autoStartId": id, "status": u.Status})
	})
	return moved, err
}

// SettleAutoStart closes what is left of a work item's auto start when a
// person took over (started it, or decided it does not start on its own).
func (db *DB) SettleAutoStart(ctx context.Context, projectID, workItemID, note string) error {
	list, err := db.AutoStarts(ctx, projectID, AutoStartFilter{WorkItemID: workItemID})
	if err != nil {
		return err
	}
	for _, a := range list {
		if _, err := db.MoveAutoStart(ctx, a.ID, []string{AutoPending, AutoPlanning, AutoApproved, AutoReview},
			AutoStartUpdate{Status: AutoCancelled, Note: strings.TrimSpace(note)}); err != nil {
			return err
		}
	}
	return nil
}

// DismissAutoStart closes an auto start a person looked at.
func (db *DB) DismissAutoStart(ctx context.Context, projectID, id, note string) error {
	list, err := queryAutoStarts(ctx, db.sql, `s.id = ? AND s.project_id = ?`, id, projectID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return fmt.Errorf("auto start %q: %w", id, ErrNotInProject)
	}
	moved, err := db.MoveAutoStart(ctx, id, []string{AutoReview}, AutoStartUpdate{Status: AutoCancelled, Note: strings.TrimSpace(note)})
	if err == nil && !moved {
		return invalid("검토할 자동 시작이 아닙니다")
	}
	return err
}

// RetryAutoStart sends an auto start under review back to the starter,
// for example after the department's rules were changed.
func (db *DB) RetryAutoStart(ctx context.Context, projectID, id string) error {
	list, err := queryAutoStarts(ctx, db.sql, `s.id = ? AND s.project_id = ?`, id, projectID)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		return fmt.Errorf("auto start %q: %w", id, ErrNotInProject)
	}
	moved, err := db.MoveAutoStart(ctx, id, []string{AutoReview}, AutoStartUpdate{Status: AutoPending})
	if err == nil && !moved {
		return invalid("검토할 자동 시작이 아닙니다")
	}
	return err
}

// RunStatus is a run's status, for the auto starter.
func (db *DB) RunStatus(ctx context.Context, runID string) (string, error) {
	var s string
	err := db.sql.QueryRowContext(ctx, `SELECT status FROM runs WHERE id = ?`, runID).Scan(&s)
	return s, err
}
