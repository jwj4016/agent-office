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

// DesignRole is a new company role from a design; ParentRef names
// another DesignRole's Ref or an existing role id.
type DesignRole struct {
	Ref, Name, Mission, Instructions, ParentRef string
}

// DesignAssignment refers to its role by design ref.
type DesignAssignment struct {
	Ref, RoleRef, ActorKind, DisplayName, ConnectionID, Model, Instructions string
}

// DesignApply is a checked design ready to write.
type DesignApply struct {
	// Exactly one of ProjectID (existing service) or NewProject.
	ProjectID  string
	NewProject *domain.Project
	// ExistingRoles maps design role refs to reused company role ids.
	ExistingRoles map[string]string
	NewRoles      []DesignRole
	Assignments   []DesignAssignment
	// Workflow uses assignment refs; they are replaced with real ids.
	Workflow   domain.WorkflowSpec
	AutoPolicy json.RawMessage
}

// DesignApplied lists what was created.
type DesignApplied struct {
	ProjectID     string            `json:"projectId"`
	RoleIDs       map[string]string `json:"roleIds"`
	AssignmentIDs map[string]string `json:"assignmentIds"`
	WorkflowID    string            `json:"workflowId"`
	NewProject    bool              `json:"newProject"`
}

// ApplyDesign creates the project (if new), roles, assignments and the
// workflow draft in one transaction: either all of it exists afterwards
// or none of it does. Existing company roles are never modified here.
func (db *DB) ApplyDesign(ctx context.Context, in DesignApply) (DesignApplied, error) {
	out := DesignApplied{RoleIDs: map[string]string{}, AssignmentIDs: map[string]string{}}
	if (in.ProjectID == "") == (in.NewProject == nil) {
		return out, invalid("대상 서비스가 올바르지 않습니다")
	}
	_, err := db.Change(ctx, func(c *Change) error {
		tx, now := c.Tx, Now()
		projectID := in.ProjectID
		if in.NewProject != nil {
			p := *in.NewProject
			if strings.TrimSpace(p.Name) == "" {
				return invalid("서비스 이름이 필요합니다")
			}
			if p.Mode == "" {
				p.Mode = domain.ModeReview
			}
			projectID = NewID("prj")
			if _, err := tx.ExecContext(ctx, `INSERT INTO projects (id, organization_id, name, goal, instructions, workspace_path, budget, mode, status, auto_policy, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, '', ?, ?, 'active', ?, ?, ?)`,
				projectID, domain.DefaultOrganizationID, p.Name, p.Goal, p.Instructions, rawOr(p.Budget, "{}"), p.Mode, rawOr(in.AutoPolicy, "{}"), now, now); err != nil {
				return err
			}
			out.NewProject = true
			if err := c.Emit(projectID, "", "", "project.created", map[string]string{"name": p.Name, "by": "design"}); err != nil {
				return err
			}
		} else {
			p, err := projectTx(ctx, tx, projectID)
			if err != nil {
				return err
			}
			if p.Status == "archived" {
				return ErrArchived
			}
		}
		out.ProjectID = projectID

		for ref, id := range in.ExistingRoles {
			if _, err := roleTx(ctx, tx, id); err != nil {
				return err
			}
			out.RoleIDs[ref] = id
		}
		for _, r := range in.NewRoles {
			out.RoleIDs[r.Ref] = NewID("role")
		}
		for _, r := range in.NewRoles {
			parent := r.ParentRef
			if id, ok := out.RoleIDs[parent]; ok {
				parent = id
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO roles (id, organization_id, parent_role_id, name, mission, instructions, created_at, updated_at)
				VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)`, out.RoleIDs[r.Ref], domain.DefaultOrganizationID, "", r.Name, r.Mission, r.Instructions, now, now); err != nil {
				return err
			}
			if parent != "" {
				if err := checkRoleParent(ctx, tx, out.RoleIDs[r.Ref], parent); err != nil {
					return fmt.Errorf("role %q: %w", r.Name, err)
				}
				if _, err := tx.ExecContext(ctx, `UPDATE roles SET parent_role_id = ? WHERE id = ?`, parent, out.RoleIDs[r.Ref]); err != nil {
					return err
				}
			}
		}

		for _, a := range in.Assignments {
			roleID, ok := out.RoleIDs[a.RoleRef]
			if !ok {
				return invalid("담당자 %q의 역할을 찾을 수 없습니다", a.DisplayName)
			}
			id := NewID("asg")
			actorID, conn, model := "", a.ConnectionID, a.Model
			if a.ActorKind == string(domain.ActorHuman) {
				actorID, conn, model = domain.LocalOwner, "", ""
			} else if a.ActorKind != string(domain.ActorAI) {
				return invalid("담당자 종류 %q", a.ActorKind)
			}
			overrides, _ := json.Marshal(domain.AssignmentOverrides{Instructions: a.Instructions})
			if _, err := tx.ExecContext(ctx, `INSERT INTO assignments (id, project_id, role_id, actor_kind, actor_id, display_name, connection_id, model, overrides, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)`, id, projectID, roleID, a.ActorKind, actorID, a.DisplayName, conn, model, string(overrides), now, now); err != nil {
				return err
			}
			out.AssignmentIDs[a.Ref] = id
		}

		w := in.Workflow
		w.Nodes = append([]domain.Node(nil), in.Workflow.Nodes...)
		for i := range w.Nodes {
			if id, ok := out.AssignmentIDs[w.Nodes[i].AssignmentID]; ok {
				w.Nodes[i].AssignmentID = id
			}
		}
		draft, _ := json.Marshal(w)
		title := strings.TrimSpace(w.Title)
		if title == "" {
			title = "AI 설계 흐름"
		}
		out.WorkflowID = NewID("wf")
		if _, err := tx.ExecContext(ctx, `INSERT INTO workflows (id, project_id, title, draft_json, revision, created_at, updated_at) VALUES (?, ?, ?, ?, 1, ?, ?)`,
			out.WorkflowID, projectID, title, string(draft), now, now); err != nil {
			return err
		}
		return c.Emit(projectID, "", "", "design.applied", map[string]any{"workflowId": out.WorkflowID, "roles": len(in.NewRoles), "assignments": len(in.Assignments)})
	})
	return out, err
}

// AutoPolicy returns a project's stored auto-mode scope.
func (db *DB) AutoPolicy(ctx context.Context, projectID string) (json.RawMessage, error) {
	var raw string
	err := db.sql.QueryRowContext(ctx, `SELECT auto_policy FROM projects WHERE id = ?`, projectID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotInProject
	}
	return json.RawMessage(raw), err
}

func (db *DB) SetAutoPolicy(ctx context.Context, projectID string, policy json.RawMessage) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE projects SET auto_policy = ?, updated_at = ? WHERE id = ?`, rawOr(policy, "{}"), Now(), projectID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotInProject
		}
		return nil
	})
}

// DesignRecord is a stored design request.
type DesignRecord struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"projectId"`
	Goal         string          `json:"goal"`
	Input        json.RawMessage `json:"input"`
	Mode         string          `json:"mode"`
	ConnectionID string          `json:"connectionId"`
	Status       string          `json:"status"`
	Proposal     json.RawMessage `json:"proposal"`
	CheckResult  json.RawMessage `json:"checkResult"`
	Error        string          `json:"error"`
	Applied      json.RawMessage `json:"applied"`
	CreatedAt    string          `json:"createdAt"`
	UpdatedAt    string          `json:"updatedAt"`
}

const designCols = `id, COALESCE(project_id, ''), goal, input, mode, COALESCE(connection_id, ''), status, proposal, check_result, error, applied, created_at, updated_at`

func scanDesign(s interface{ Scan(...any) error }) (DesignRecord, error) {
	var d DesignRecord
	var input, proposal, check, applied string
	err := s.Scan(&d.ID, &d.ProjectID, &d.Goal, &input, &d.Mode, &d.ConnectionID, &d.Status, &proposal, &check, &d.Error, &applied, &d.CreatedAt, &d.UpdatedAt)
	d.Input, d.Proposal, d.CheckResult, d.Applied = json.RawMessage(input), json.RawMessage(proposal), json.RawMessage(check), json.RawMessage(applied)
	return d, err
}

func (db *DB) CreateDesign(ctx context.Context, d DesignRecord) (DesignRecord, error) {
	d.ID, d.Status = NewID("dsn"), "drafting"
	now := Now()
	err := db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO design_requests (id, project_id, goal, input, mode, connection_id, status, created_at, updated_at)
			VALUES (?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), 'drafting', ?, ?)`, d.ID, d.ProjectID, d.Goal, rawOr(d.Input, "{}"), d.Mode, d.ConnectionID, now, now)
		return err
	})
	if err != nil {
		return d, err
	}
	return db.Design(ctx, d.ID)
}

func (db *DB) Design(ctx context.Context, id string) (DesignRecord, error) {
	d, err := scanDesign(db.sql.QueryRowContext(ctx, `SELECT `+designCols+` FROM design_requests WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return d, fmt.Errorf("design %q: %w", id, ErrNotFound)
	}
	return d, err
}

func (db *DB) Designs(ctx context.Context, limit int) ([]DesignRecord, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+designCols+` FROM design_requests ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DesignRecord{}
	for rows.Next() {
		d, err := scanDesign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// UpdateDesign moves a design to a new status only from one of the
// allowed ones, so a late designer result cannot overwrite a discard.
// Empty JSON arguments leave the stored value unchanged.
func (db *DB) UpdateDesign(ctx context.Context, id, status string, proposal, check, applied json.RawMessage, errMsg, projectID string, from ...string) (bool, error) {
	var ok bool
	err := db.Write(ctx, func(tx *sql.Tx) error {
		p, c, a := string(proposal), string(check), string(applied)
		args := []any{status, p, p, c, c, a, a, errMsg, projectID, Now(), id}
		for _, f := range from {
			args = append(args, f)
		}
		res, err := tx.ExecContext(ctx, `UPDATE design_requests SET status = ?,
			proposal = CASE WHEN ? <> '' THEN ? ELSE proposal END,
			check_result = CASE WHEN ? <> '' THEN ? ELSE check_result END,
			applied = CASE WHEN ? <> '' THEN ? ELSE applied END,
			error = ?, project_id = COALESCE(NULLIF(?, ''), project_id), updated_at = ?
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

func placeholders(n int) string {
	if n == 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
