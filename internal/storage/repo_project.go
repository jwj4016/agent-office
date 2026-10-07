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

const projectCols = `id, organization_id, name, goal, instructions, workspace_path, budget, mode, status, auto_policy, created_at, updated_at`

func scanProject(s interface{ Scan(...any) error }) (domain.Project, error) {
	var p domain.Project
	var budget, auto string
	err := s.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Goal, &p.Instructions, &p.WorkspacePath, &budget, &p.Mode, &p.Status, &auto, &p.CreatedAt, &p.UpdatedAt)
	p.Budget, p.AutoPolicy = json.RawMessage(budget), json.RawMessage(auto)
	return p, err
}

// Projects lists projects; archived ones only when includeArchived.
func (db *DB) Projects(ctx context.Context, includeArchived bool) ([]domain.Project, error) {
	q := `SELECT ` + projectCols + ` FROM projects`
	if !includeArchived {
		q += ` WHERE status = 'active'`
	}
	rows, err := db.sql.QueryContext(ctx, q+` ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Project
	for rows.Next() {
		p, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func projectTx(ctx context.Context, q queryer, id string) (domain.Project, error) {
	p, err := scanProject(q.QueryRowContext(ctx, `SELECT `+projectCols+` FROM projects WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("project %q: %w", id, ErrNotInProject)
	}
	return p, err
}

func (db *DB) Project(ctx context.Context, id string) (domain.Project, error) {
	return projectTx(ctx, db.sql, id)
}

func validateProject(p domain.Project) error {
	if strings.TrimSpace(p.Name) == "" {
		return invalid("서비스 이름이 필요합니다")
	}
	if p.Mode != domain.ModeReview && p.Mode != domain.ModeAuto {
		return invalid("실행 모드는 review 또는 auto입니다")
	}
	return nil
}

func (db *DB) CreateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	if p.Mode == "" {
		p.Mode = domain.ModeReview
	}
	if err := validateProject(p); err != nil {
		return p, err
	}
	p.ID = NewID("prj")
	_, err := db.Change(ctx, func(c *Change) error {
		now := Now()
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO projects (id, organization_id, name, goal, instructions, workspace_path, budget, mode, status, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'active', ?, ?)`,
			p.ID, domain.DefaultOrganizationID, p.Name, p.Goal, p.Instructions, p.WorkspacePath, rawOr(p.Budget, "{}"), p.Mode, now, now); err != nil {
			return err
		}
		return c.Emit(p.ID, "", "", "project.created", map[string]string{"name": p.Name})
	})
	if err != nil {
		return p, err
	}
	return db.Project(ctx, p.ID)
}

func (db *DB) UpdateProject(ctx context.Context, p domain.Project) (domain.Project, error) {
	if err := validateProject(p); err != nil {
		return p, err
	}
	_, err := db.Change(ctx, func(c *Change) error {
		cur, err := projectTx(ctx, c.Tx, p.ID)
		if err != nil {
			return err
		}
		if cur.Status == "archived" {
			return ErrArchived
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE projects SET name = ?, goal = ?, instructions = ?, workspace_path = ?, budget = ?, mode = ?, updated_at = ? WHERE id = ?`,
			p.Name, p.Goal, p.Instructions, p.WorkspacePath, rawOr(p.Budget, "{}"), p.Mode, Now(), p.ID); err != nil {
			return err
		}
		return c.Emit(p.ID, "", "", "project.updated", nil)
	})
	if err != nil {
		return p, err
	}
	return db.Project(ctx, p.ID)
}

// SetProjectArchived archives or restores a project. Archiving keeps all
// data; it only hides the project from the default list.
func (db *DB) SetProjectArchived(ctx context.Context, id string, archived bool) error {
	status := "active"
	if archived {
		status = "archived"
	}
	_, err := db.Change(ctx, func(c *Change) error {
		if _, err := projectTx(ctx, c.Tx, id); err != nil {
			return err
		}
		if archived {
			var active int
			c.Tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE project_id = ? AND status IN ('running', 'waiting', 'paused')`, id).Scan(&active)
			if active > 0 {
				return invalid("진행 중인 실행이 있어 보관할 수 없습니다")
			}
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE projects SET status = ?, updated_at = ? WHERE id = ?`, status, Now(), id); err != nil {
			return err
		}
		return c.Emit(id, "", "", "project."+status, nil)
	})
	return err
}

const assignmentCols = `id, project_id, role_id, actor_kind, actor_id, display_name, COALESCE(connection_id, ''), model, overrides`

func scanAssignment(s interface{ Scan(...any) error }) (domain.Assignment, error) {
	var a domain.Assignment
	var overrides string
	err := s.Scan(&a.ID, &a.ProjectID, &a.RoleID, &a.ActorKind, &a.ActorID, &a.DisplayName, &a.ConnectionID, &a.Model, &overrides)
	if err == nil {
		json.Unmarshal([]byte(overrides), &a.Overrides)
	}
	return a, err
}

func (db *DB) Assignments(ctx context.Context, projectID string) ([]domain.Assignment, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	return assignmentsTx(ctx, db.sql, projectID)
}

func assignmentsTx(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}, projectID string) ([]domain.Assignment, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+assignmentCols+` FROM assignments WHERE project_id = ? ORDER BY display_name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Assignment
	for rows.Next() {
		a, err := scanAssignment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// SaveAssignment creates or updates a project assignment. Creating one
// never starts a model process (spec §2.3).
func (db *DB) SaveAssignment(ctx context.Context, a domain.Assignment) (domain.Assignment, error) {
	if strings.TrimSpace(a.DisplayName) == "" {
		return a, invalid("담당자 표시 이름이 필요합니다")
	}
	switch a.ActorKind {
	case domain.ActorHuman:
		a.ActorID, a.ConnectionID, a.Model = domain.LocalOwner, "", ""
	case domain.ActorAI:
		a.ActorID = ""
	default:
		return a, invalid("담당자 종류는 ai 또는 human입니다")
	}
	overrides, _ := json.Marshal(a.Overrides)
	_, err := db.Change(ctx, func(c *Change) error {
		p, err := projectTx(ctx, c.Tx, a.ProjectID)
		if err != nil {
			return err
		}
		if p.Status == "archived" {
			return ErrArchived
		}
		if _, err := roleTx(ctx, c.Tx, a.RoleID); err != nil {
			return err
		}
		if a.ID == "" {
			a.ID = NewID("asg")
		} else if err := CheckScopeTx(ctx, c.Tx, a.ProjectID, Ref{RefAssignment, a.ID}); err != nil {
			return err
		}
		now := Now()
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO assignments (id, project_id, role_id, actor_kind, actor_id, display_name, connection_id, model, overrides, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET role_id = excluded.role_id, actor_kind = excluded.actor_kind, actor_id = excluded.actor_id,
				display_name = excluded.display_name, connection_id = excluded.connection_id, model = excluded.model,
				overrides = excluded.overrides, updated_at = excluded.updated_at`,
			a.ID, a.ProjectID, a.RoleID, a.ActorKind, a.ActorID, a.DisplayName, a.ConnectionID, a.Model, string(overrides), now, now); err != nil {
			return err
		}
		return c.Emit(a.ProjectID, "", "", "assignment.saved", map[string]string{"assignmentId": a.ID})
	})
	if err != nil {
		return a, err
	}
	return scanAssignment(db.sql.QueryRowContext(ctx, `SELECT `+assignmentCols+` FROM assignments WHERE id = ?`, a.ID))
}

// InstructionLayers returns the merged instruction preview for an
// assignment: company → parent roles → role → project → assignment.
func (db *DB) InstructionLayers(ctx context.Context, projectID, assignmentID string) ([]domain.InstructionLayer, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefAssignment, assignmentID}); err != nil {
		return nil, err
	}
	a, err := scanAssignment(db.sql.QueryRowContext(ctx, `SELECT `+assignmentCols+` FROM assignments WHERE id = ?`, assignmentID))
	if err != nil {
		return nil, err
	}
	return instructionLayers(ctx, db.sql, a)
}

func instructionLayers(ctx context.Context, q queryer, a domain.Assignment) ([]domain.InstructionLayer, error) {
	var orgName, orgText, projName, projText string
	if err := q.QueryRowContext(ctx, `SELECT o.name, o.instructions, p.name, p.instructions FROM projects p JOIN organizations o ON o.id = p.organization_id WHERE p.id = ?`,
		a.ProjectID).Scan(&orgName, &orgText, &projName, &projText); err != nil {
		return nil, err
	}
	layers := []domain.InstructionLayer{{Source: "organization", Name: "회사: " + orgName, Text: orgText}}
	chain, err := roleChain(ctx, q, a.RoleID)
	if err != nil {
		return nil, err
	}
	for _, r := range chain {
		text := r.Instructions
		if r.Mission != "" {
			text = "책임: " + r.Mission + "\n" + text
		}
		layers = append(layers, domain.InstructionLayer{Source: "role", Name: "역할: " + r.Name, Text: text})
	}
	layers = append(layers, domain.InstructionLayer{Source: "project", Name: "서비스: " + projName, Text: projText})
	if a.Overrides.Instructions != "" {
		layers = append(layers, domain.InstructionLayer{Source: "assignment", Name: "담당자: " + a.DisplayName, Text: a.Overrides.Instructions})
	}
	return layers, nil
}
