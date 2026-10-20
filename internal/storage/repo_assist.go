package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// AssistRecord is one AI job on a workflow: a proposed edit of its draft
// ("edit") or a pipeline diagram ("diagram").
type AssistRecord struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"projectId"`
	WorkflowID   string          `json:"workflowId"`
	Kind         string          `json:"kind"`
	Request      string          `json:"request"`
	BaseRevision int             `json:"baseRevision"`
	ConnectionID string          `json:"connectionId"`
	Model        string          `json:"model"`
	Status       string          `json:"status"`
	Result       json.RawMessage `json:"result"`
	Error        string          `json:"error"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
	BaseWorkflow json.RawMessage `json:"baseWorkflow"`
	ParentID     string          `json:"parentId,omitempty"`
	ScopeNodeID  string          `json:"scopeNodeId,omitempty"`
}

const assistCols = `id, project_id, workflow_id, kind, request, base_revision, COALESCE(connection_id, ''), model, status, result, error, created_at, updated_at, base_workflow, COALESCE(parent_id, ''), scope_node_id`

func scanAssist(r interface{ Scan(...any) error }) (AssistRecord, error) {
	var a AssistRecord
	var result, base string
	err := r.Scan(&a.ID, &a.ProjectID, &a.WorkflowID, &a.Kind, &a.Request, &a.BaseRevision, &a.ConnectionID, &a.Model, &a.Status, &result, &a.Error, &a.CreatedAt, &a.UpdatedAt, &base, &a.ParentID, &a.ScopeNodeID)
	a.Result = json.RawMessage(result)
	a.BaseWorkflow = json.RawMessage(base)
	return a, err
}

// CreateAssist records a new job in the "working" state.
func (db *DB) CreateAssist(ctx context.Context, a AssistRecord) (AssistRecord, error) {
	a.ID, a.Status = NewID("ast"), "working"
	now := Now()
	err := db.Write(ctx, func(tx *sql.Tx) error {
		if err := CheckScopeTx(ctx, tx, a.ProjectID, Ref{RefWorkflow, a.WorkflowID}); err != nil {
			return err
		}
		if len(a.BaseWorkflow) == 0 {
			a.BaseWorkflow = json.RawMessage(`{}`)
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO workflow_assists (id, project_id, workflow_id, kind, request, base_revision, connection_id, model, status, created_at, updated_at, base_workflow, parent_id, scope_node_id)
			VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, 'working', ?, ?, ?, NULLIF(?, ''), ?)`,
			a.ID, a.ProjectID, a.WorkflowID, a.Kind, a.Request, a.BaseRevision, a.ConnectionID, a.Model, now, now, string(a.BaseWorkflow), a.ParentID, a.ScopeNodeID)
		return err
	})
	if err != nil {
		return a, err
	}
	return db.Assist(ctx, a.ID)
}

func (db *DB) Assist(ctx context.Context, id string) (AssistRecord, error) {
	a, err := scanAssist(db.sql.QueryRowContext(ctx, `SELECT `+assistCols+` FROM workflow_assists WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return a, fmt.Errorf("assist %q: %w", id, ErrNotFound)
	}
	return a, err
}

// Assists lists a workflow's jobs, newest first. An empty workflowID
// lists every job (start-up recovery).
func (db *DB) Assists(ctx context.Context, workflowID string, limit int) ([]AssistRecord, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+assistCols+` FROM workflow_assists WHERE ? = '' OR workflow_id = ? ORDER BY created_at DESC LIMIT ?`, workflowID, workflowID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AssistRecord{}
	for rows.Next() {
		a, err := scanAssist(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateAssist moves a job to status only from one of the allowed ones,
// so a late AI result cannot overwrite a discard. A nil result leaves the
// stored one unchanged.
func (db *DB) UpdateAssist(ctx context.Context, id, status string, result json.RawMessage, errMsg string, from ...string) (bool, error) {
	var ok bool
	err := db.Write(ctx, func(tx *sql.Tx) error {
		args := []any{status, string(result), string(result), errMsg, Now(), id}
		for _, f := range from {
			args = append(args, f)
		}
		res, err := tx.ExecContext(ctx, `UPDATE workflow_assists SET status = ?,
			result = CASE WHEN ? <> '' THEN ? ELSE result END, error = ?, updated_at = ?
			WHERE id = ? AND status IN (`+placeholders(len(from))+`)`, args...)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		ok = n == 1
		return nil
	})
	return ok, err
}

// ValidateDraft checks a workflow draft for a project without saving it.
func (db *DB) ValidateDraft(ctx context.Context, projectID string, draft json.RawMessage) (ValidationResult, error) {
	var res ValidationResult
	err := db.Write(ctx, func(tx *sql.Tx) error {
		var err error
		_, res, _, err = validateDraft(ctx, tx, projectID, draft)
		return err
	})
	return res, err
}
