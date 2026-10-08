package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"agent-office/internal/domain"
)

const workflowCols = `id, project_id, title, draft_json, revision, updated_at`

func scanWorkflow(s interface{ Scan(...any) error }) (domain.Workflow, error) {
	var w domain.Workflow
	var draft string
	err := s.Scan(&w.ID, &w.ProjectID, &w.Title, &draft, &w.Revision, &w.UpdatedAt)
	w.Draft = json.RawMessage(draft)
	return w, err
}

func (db *DB) Workflows(ctx context.Context, projectID string) ([]domain.Workflow, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+workflowCols+` FROM workflows WHERE project_id = ? ORDER BY title`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Workflow
	for rows.Next() {
		w, err := scanWorkflow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (db *DB) Workflow(ctx context.Context, projectID, id string) (domain.Workflow, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefWorkflow, id}); err != nil {
		return domain.Workflow{}, err
	}
	return scanWorkflow(db.sql.QueryRowContext(ctx, `SELECT `+workflowCols+` FROM workflows WHERE id = ?`, id))
}

// draftTitle reads the title from a draft without requiring it to be a
// valid workflow: drafts may be saved half-finished.
func draftTitle(draft json.RawMessage) (string, error) {
	var head struct {
		Title string `json:"title"`
	}
	if err := json.Unmarshal(draft, &head); err != nil {
		return "", invalid("흐름 초안이 JSON 객체가 아닙니다")
	}
	if strings.TrimSpace(head.Title) == "" {
		return "", invalid("흐름 이름이 필요합니다")
	}
	return head.Title, nil
}

func (db *DB) CreateWorkflow(ctx context.Context, projectID string, draft json.RawMessage) (domain.Workflow, error) {
	title, err := draftTitle(draft)
	if err != nil {
		return domain.Workflow{}, err
	}
	id := NewID("wf")
	_, err = db.Change(ctx, func(c *Change) error {
		p, err := projectTx(ctx, c.Tx, projectID)
		if err != nil {
			return err
		}
		if p.Status == "archived" {
			return ErrArchived
		}
		now := Now()
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO workflows (id, project_id, title, draft_json, revision, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`,
			id, projectID, title, string(draft), now, now); err != nil {
			return err
		}
		return c.Emit(projectID, "", "", "workflow.created", map[string]string{"workflowId": id})
	})
	if err != nil {
		return domain.Workflow{}, err
	}
	return db.Workflow(ctx, projectID, id)
}

// SaveWorkflowDraft stores a new draft if baseRevision is still current,
// so two editors cannot silently overwrite each other. Saving a draft
// never affects runs: they use confirmed versions (T13).
func (db *DB) SaveWorkflowDraft(ctx context.Context, projectID, id string, baseRevision int, draft json.RawMessage) (domain.Workflow, error) {
	title, err := draftTitle(draft)
	if err != nil {
		return domain.Workflow{}, err
	}
	_, err = db.Change(ctx, func(c *Change) error {
		if err := CheckScopeTx(ctx, c.Tx, projectID, Ref{RefWorkflow, id}); err != nil {
			return err
		}
		res, err := c.Tx.ExecContext(ctx, `UPDATE workflows SET title = ?, draft_json = ?, revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ?`,
			title, string(draft), Now(), id, baseRevision)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrConflict
		}
		return c.Emit(projectID, "", "", "workflow.draft_saved", map[string]any{"workflowId": id, "revision": baseRevision + 1})
	})
	if err != nil {
		return domain.Workflow{}, err
	}
	return db.Workflow(ctx, projectID, id)
}

// assignmentInfos maps a project's assignments for validation.
func assignmentInfos(ctx context.Context, tx *sql.Tx, projectID string) (map[string]domain.AssignmentInfo, map[string]domain.Assignment, error) {
	list, err := assignmentsTx(ctx, tx, projectID)
	if err != nil {
		return nil, nil, err
	}
	infos := map[string]domain.AssignmentInfo{}
	byID := map[string]domain.Assignment{}
	for _, a := range list {
		byID[a.ID] = a
		info := domain.AssignmentInfo{ActorKind: string(a.ActorKind), Connected: a.ActorKind == domain.ActorHuman}
		if a.ActorKind == domain.ActorAI && a.ConnectionID != "" {
			c, err := scanConn(tx.QueryRowContext(ctx, `SELECT `+connCols+` FROM provider_connections WHERE id = ?`, a.ConnectionID))
			if err == nil {
				// The test provider is an explicit, always-available connection;
				// real providers count only after a successful check.
				info.Connected = c.Provider == "test" || c.Verified()
			}
		}
		infos[a.ID] = info
	}
	return infos, byID, nil
}

// ValidationResult combines structural and assignment issues.
type ValidationResult struct {
	Issues     []domain.Issue `json:"issues"`
	CanVersion bool           `json:"canVersion"`
	CanRun     bool           `json:"canRun"`
}

func validateDraft(ctx context.Context, tx *sql.Tx, projectID string, draft json.RawMessage) (*domain.WorkflowSpec, ValidationResult, map[string]domain.Assignment, error) {
	spec, err := domain.ParseWorkflow(draft)
	if err != nil {
		res := ValidationResult{Issues: []domain.Issue{{Code: "bad_json", Message: err.Error(), Severity: domain.SevError}}}
		return nil, res, nil, nil
	}
	infos, byID, err := assignmentInfos(ctx, tx, projectID)
	if err != nil {
		return nil, ValidationResult{}, nil, err
	}
	issues := append(domain.ValidateStructure(spec), domain.ValidateAssignments(spec, infos)...)
	res := ValidationResult{
		Issues:     issues,
		CanVersion: !domain.HasSeverity(issues, domain.SevError),
	}
	res.CanRun = res.CanVersion && !domain.HasSeverity(issues, domain.SevRun)
	return spec, res, byID, nil
}

// ValidateWorkflow checks the current draft without changing anything.
func (db *DB) ValidateWorkflow(ctx context.Context, projectID, id string) (ValidationResult, error) {
	var res ValidationResult
	err := db.Write(ctx, func(tx *sql.Tx) error {
		if err := CheckScopeTx(ctx, tx, projectID, Ref{RefWorkflow, id}); err != nil {
			return err
		}
		w, err := scanWorkflow(tx.QueryRowContext(ctx, `SELECT `+workflowCols+` FROM workflows WHERE id = ?`, id))
		if err != nil {
			return err
		}
		_, res, _, err = validateDraft(ctx, tx, projectID, w.Draft)
		return err
	})
	return res, err
}

// ErrNotVersionable carries the blocking issues of a rejected draft.
type ErrNotVersionable struct{ Issues []domain.Issue }

func (e *ErrNotVersionable) Error() string {
	return fmt.Sprintf("workflow has %d blocking issue(s)", len(e.Issues))
}

// ConfirmVersion freezes the draft, the assignments it uses (with their
// merged instructions) and the project policy into a new immutable
// version. Later edits to the draft, roles or assignments never change
// an existing version (T13).
func (db *DB) ConfirmVersion(ctx context.Context, projectID, workflowID string, expectedRevision int) (domain.WorkflowVersion, ValidationResult, error) {
	var v domain.WorkflowVersion
	var res ValidationResult
	_, err := db.Change(ctx, func(c *Change) error {
		tx := c.Tx
		if err := CheckScopeTx(ctx, tx, projectID, Ref{RefWorkflow, workflowID}); err != nil {
			return err
		}
		p, err := projectTx(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if p.Status == "archived" {
			return ErrArchived
		}
		w, err := scanWorkflow(tx.QueryRowContext(ctx, `SELECT `+workflowCols+` FROM workflows WHERE id = ?`, workflowID))
		if err != nil {
			return err
		}
		if w.Revision != expectedRevision {
			return ErrConflict
		}
		spec, r, byID, err := validateDraft(ctx, tx, projectID, w.Draft)
		res = r
		if err != nil {
			return err
		}
		if !r.CanVersion {
			return &ErrNotVersionable{Issues: r.Issues}
		}
		infos, _, _ := assignmentInfos(ctx, tx, projectID)
		snap := map[string]domain.AssignmentSnapshot{}
		var ids []string
		for _, n := range spec.Nodes {
			ids = append(ids, n.AssignmentID)
			if n.Meeting != nil {
				ids = append(ids, n.Meeting.Participants...)
			}
		}
		for _, id := range ids {
			if id == "" || snap[id].ID != "" {
				continue
			}
			a := byID[id]
			layers, err := instructionLayers(ctx, tx, a)
			if err != nil {
				return err
			}
			role, err := roleTx(ctx, tx, a.RoleID)
			if err != nil {
				return err
			}
			snap[a.ID] = domain.AssignmentSnapshot{Assignment: a, RoleName: role.Name, Instructions: layers, Connected: infos[a.ID].Connected}
		}
		var num int
		tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(number), 0) + 1 FROM workflow_versions WHERE workflow_id = ?`, workflowID).Scan(&num)
		v = domain.WorkflowVersion{
			ID: NewID("wv"), WorkflowID: workflowID, ProjectID: projectID, Number: num, Spec: *spec, Assignments: snap,
			Policy:    domain.PolicySnapshot{ProjectMode: p.Mode, Budget: p.Budget, WorkspacePath: p.WorkspacePath, Goal: p.Goal},
			CreatedAt: Now(),
		}
		specJSON, _ := json.Marshal(v.Spec)
		snapJSON, _ := json.Marshal(v.Assignments)
		polJSON, _ := json.Marshal(v.Policy)
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflow_versions (id, workflow_id, project_id, number, spec_json, assignments_snapshot, policy_snapshot, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, v.ID, workflowID, projectID, num, string(specJSON), string(snapJSON), string(polJSON), v.CreatedAt); err != nil {
			return err
		}
		return c.Emit(projectID, "", "", "workflow.version_created", map[string]any{"workflowId": workflowID, "versionId": v.ID, "number": num})
	})
	return v, res, err
}

// WorkflowVersion loads a frozen version.
func (db *DB) WorkflowVersion(ctx context.Context, projectID, id string) (domain.WorkflowVersion, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefVersion, id}); err != nil {
		return domain.WorkflowVersion{}, err
	}
	return workflowVersionTx(ctx, db.sql, id)
}

func workflowVersionTx(ctx context.Context, q queryer, id string) (domain.WorkflowVersion, error) {
	var v domain.WorkflowVersion
	var spec, snap, pol string
	err := q.QueryRowContext(ctx, `SELECT id, workflow_id, project_id, number, spec_json, assignments_snapshot, policy_snapshot, created_at FROM workflow_versions WHERE id = ?`, id).
		Scan(&v.ID, &v.WorkflowID, &v.ProjectID, &v.Number, &spec, &snap, &pol, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return v, fmt.Errorf("version %q: %w", id, ErrNotFound)
	}
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
