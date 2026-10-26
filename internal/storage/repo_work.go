package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"agent-office/internal/domain"
)

func dedupe(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" && !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	return out
}

type rowsQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

// pairs reads (key, value) rows into a map of value lists.
func pairs(ctx context.Context, q rowsQueryer, query string, args ...any) (map[string][]string, error) {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = append(out[k], v)
	}
	return out, rows.Err()
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// ---- department templates (company) ----

const templateCols = `id, organization_id, name, responsibility, updated_at`

func scanTemplate(s interface{ Scan(...any) error }) (domain.DepartmentTemplate, error) {
	var d domain.DepartmentTemplate
	err := s.Scan(&d.ID, &d.OrganizationID, &d.Name, &d.Responsibility, &d.UpdatedAt)
	return d, err
}

// DepartmentTemplates lists the company's department templates.
func (db *DB) DepartmentTemplates(ctx context.Context) ([]domain.DepartmentTemplate, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+templateCols+` FROM department_templates WHERE organization_id = ? ORDER BY name`, domain.DefaultOrganizationID)
	if err != nil {
		return nil, err
	}
	out := []domain.DepartmentTemplate{}
	for rows.Next() {
		d, err := scanTemplate(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	roles, err := pairs(ctx, db.sql, `SELECT tr.template_id, tr.role_id FROM department_template_roles tr JOIN roles r ON r.id = tr.role_id ORDER BY r.name`)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].RoleIDs = orEmpty(roles[out[i].ID])
	}
	return out, nil
}

// DepartmentTemplate returns one template with its roles.
func (db *DB) DepartmentTemplate(ctx context.Context, id string) (domain.DepartmentTemplate, error) {
	all, err := db.DepartmentTemplates(ctx)
	if err != nil {
		return domain.DepartmentTemplate{}, err
	}
	for _, d := range all {
		if d.ID == id {
			return d, nil
		}
	}
	return domain.DepartmentTemplate{}, fmt.Errorf("department template %q: %w", id, ErrNotFound)
}

// SaveDepartmentTemplate creates (empty ID) or updates a template and
// replaces its role list. Departments already made from it keep their
// own copy.
func (db *DB) SaveDepartmentTemplate(ctx context.Context, d domain.DepartmentTemplate) (domain.DepartmentTemplate, error) {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return d, invalid("부서 템플릿 이름이 필요합니다")
	}
	err := db.Write(ctx, func(tx *sql.Tx) error {
		if d.ID == "" {
			d.ID = NewID("dtpl")
		} else if _, err := scanTemplate(tx.QueryRowContext(ctx, `SELECT `+templateCols+` FROM department_templates WHERE id = ?`, d.ID)); errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("department template %q: %w", d.ID, ErrNotFound)
		} else if err != nil {
			return err
		}
		var dup int
		tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM department_templates WHERE organization_id = ? AND name = ? AND id <> ?`,
			domain.DefaultOrganizationID, d.Name, d.ID).Scan(&dup)
		if dup > 0 {
			return invalid("같은 이름의 부서 템플릿이 이미 있습니다")
		}
		now := Now()
		if _, err := tx.ExecContext(ctx, `INSERT INTO department_templates (id, organization_id, name, responsibility, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET name = excluded.name, responsibility = excluded.responsibility, updated_at = excluded.updated_at`,
			d.ID, domain.DefaultOrganizationID, d.Name, d.Responsibility, now, now); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM department_template_roles WHERE template_id = ?`, d.ID); err != nil {
			return err
		}
		for _, r := range dedupe(d.RoleIDs) {
			if _, err := roleTx(ctx, tx, r); err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO department_template_roles (template_id, role_id) VALUES (?, ?)`, d.ID, r); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return d, err
	}
	return db.DepartmentTemplate(ctx, d.ID)
}

// DeleteDepartmentTemplate removes a template. Departments copied from it
// stay and only lose the link to it.
func (db *DB) DeleteDepartmentTemplate(ctx context.Context, id string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM department_templates WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return fmt.Errorf("department template %q: %w", id, ErrNotFound)
		}
		return nil
	})
}

// ---- departments (inside a service) ----

const departmentCols = `id, project_id, name, responsibility, COALESCE(lead_assignment_id, ''), COALESCE(template_id, ''),
	automation, limits, status, created_at, updated_at`

func scanDepartment(s interface{ Scan(...any) error }) (domain.Department, error) {
	var d domain.Department
	var auto, limits string
	err := s.Scan(&d.ID, &d.ProjectID, &d.Name, &d.Responsibility, &d.LeadAssignmentID, &d.TemplateID,
		&auto, &limits, &d.Status, &d.CreatedAt, &d.UpdatedAt)
	d.Automation, d.Limits = json.RawMessage(auto), json.RawMessage(limits)
	return d, err
}

// Departments lists a service's departments with their members.
func (db *DB) Departments(ctx context.Context, projectID string) ([]domain.Department, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	return departmentsTx(ctx, db.sql, projectID)
}

func departmentsTx(ctx context.Context, q rowsQueryer, projectID string) ([]domain.Department, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+departmentCols+` FROM departments WHERE project_id = ? ORDER BY name`, projectID)
	if err != nil {
		return nil, err
	}
	out := []domain.Department{}
	for rows.Next() {
		d, err := scanDepartment(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	members, err := pairs(ctx, q, `SELECT m.department_id, m.assignment_id FROM department_members m JOIN assignments a ON a.id = m.assignment_id
		WHERE m.project_id = ? ORDER BY a.display_name`, projectID)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].MemberIDs = orEmpty(members[out[i].ID])
	}
	return out, nil
}

func departmentTx(ctx context.Context, q queryer, projectID, id string) (domain.Department, error) {
	d, err := scanDepartment(q.QueryRowContext(ctx, `SELECT `+departmentCols+` FROM departments WHERE id = ? AND project_id = ?`, id, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return d, fmt.Errorf("department %q: %w", id, ErrNotInProject)
	}
	return d, err
}

// Department returns one department of the service with its members.
func (db *DB) Department(ctx context.Context, projectID, id string) (domain.Department, error) {
	all, err := db.Departments(ctx, projectID)
	if err != nil {
		return domain.Department{}, err
	}
	for _, d := range all {
		if d.ID == id {
			return d, nil
		}
	}
	return domain.Department{}, fmt.Errorf("department %q: %w", id, ErrNotInProject)
}

// SaveDepartment creates (empty ID) or updates a department of a service.
// MemberIDs (when not nil) replaces its members: they must be the
// service's assignments, and one listed here leaves the department it
// was in. The lead must be a member.
func (db *DB) SaveDepartment(ctx context.Context, d domain.Department) (domain.Department, error) {
	err := db.Write(ctx, func(tx *sql.Tx) error {
		return saveDepartmentTx(ctx, tx, &d)
	})
	if err != nil {
		return d, err
	}
	return db.Department(ctx, d.ProjectID, d.ID)
}

func saveDepartmentTx(ctx context.Context, tx *sql.Tx, d *domain.Department) error {
	d.Name = strings.TrimSpace(d.Name)
	if d.Name == "" {
		return invalid("부서 이름이 필요합니다")
	}
	if d.Status == "" {
		d.Status = domain.StatusActive
	}
	if d.Status != domain.StatusActive && d.Status != domain.StatusInactive {
		return invalid("부서 상태는 active 또는 inactive입니다")
	}
	if err := writableProjectTx(ctx, tx, d.ProjectID); err != nil {
		return err
	}
	if d.ID == "" {
		d.ID = NewID("dept")
	} else {
		cur, err := departmentTx(ctx, tx, d.ProjectID, d.ID)
		if err != nil {
			return err
		}
		d.TemplateID = cur.TemplateID // the origin never changes
	}
	var dup int
	tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM departments WHERE project_id = ? AND name = ? AND id <> ?`, d.ProjectID, d.Name, d.ID).Scan(&dup)
	if dup > 0 {
		return invalid("이 서비스에 같은 이름의 부서가 이미 있습니다")
	}
	members := d.MemberIDs
	if members == nil {
		cur, err := pairs(ctx, tx, `SELECT department_id, assignment_id FROM department_members WHERE department_id = ?`, d.ID)
		if err != nil {
			return err
		}
		members = cur[d.ID]
	}
	members = dedupe(members)
	for _, m := range members {
		if err := CheckScopeTx(ctx, tx, d.ProjectID, Ref{RefAssignment, m}); err != nil {
			return err
		}
	}
	if d.LeadAssignmentID != "" && !slices.Contains(members, d.LeadAssignmentID) {
		return invalid("대표 담당자는 이 부서의 구성원이어야 합니다")
	}
	now := Now()
	if _, err := tx.ExecContext(ctx, `INSERT INTO departments (id, project_id, name, responsibility, lead_assignment_id, template_id, automation, limits, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name, responsibility = excluded.responsibility,
			lead_assignment_id = excluded.lead_assignment_id, status = excluded.status, updated_at = excluded.updated_at`,
		d.ID, d.ProjectID, d.Name, d.Responsibility, d.LeadAssignmentID, d.TemplateID,
		rawOr(d.Automation, "{}"), rawOr(d.Limits, "{}"), d.Status, now, now); err != nil {
		return err
	}
	if d.MemberIDs == nil {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM department_members WHERE department_id = ?`, d.ID); err != nil {
		return err
	}
	// A member moving here stops leading the department it leaves.
	for _, m := range members {
		if _, err := tx.ExecContext(ctx, `UPDATE departments SET lead_assignment_id = NULL, updated_at = ?
			WHERE lead_assignment_id = ? AND id <> ?`, now, m, d.ID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO department_members (assignment_id, project_id, department_id, created_at) VALUES (?, ?, ?, ?)
			ON CONFLICT(assignment_id) DO UPDATE SET department_id = excluded.department_id, created_at = excluded.created_at`,
			m, d.ProjectID, d.ID, now); err != nil {
			return err
		}
	}
	return nil
}

// ImportDepartmentTemplate copies a template into a service as a new
// department. The service's assignments of the template's roles that are
// not yet in a department become its members, and the first AI member
// (or the first member) leads it. A name already used in the service gets
// a number.
func (db *DB) ImportDepartmentTemplate(ctx context.Context, projectID, templateID string) (domain.Department, error) {
	tpl, err := db.DepartmentTemplate(ctx, templateID)
	if err != nil {
		return domain.Department{}, err
	}
	var d domain.Department
	err = db.Write(ctx, func(tx *sql.Tx) error {
		if err := writableProjectTx(ctx, tx, projectID); err != nil {
			return err
		}
		name := tpl.Name
		for n := 2; ; n++ {
			var dup int
			tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM departments WHERE project_id = ? AND name = ?`, projectID, name).Scan(&dup)
			if dup == 0 {
				break
			}
			name = fmt.Sprintf("%s %d", tpl.Name, n)
		}
		members := []string{}
		lead := ""
		if len(tpl.RoleIDs) > 0 {
			rows, err := tx.QueryContext(ctx, `SELECT a.id, a.actor_kind, a.role_id FROM assignments a
				WHERE a.project_id = ? AND NOT EXISTS (SELECT 1 FROM department_members m WHERE m.assignment_id = a.id)
				ORDER BY a.display_name`, projectID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var id, kind, role string
				if err := rows.Scan(&id, &kind, &role); err != nil {
					rows.Close()
					return err
				}
				if slices.Contains(tpl.RoleIDs, role) {
					members = append(members, id)
					if lead == "" && kind == string(domain.ActorAI) {
						lead = id
					}
				}
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return err
			}
		}
		if lead == "" && len(members) > 0 {
			lead = members[0]
		}
		d = domain.Department{ProjectID: projectID, Name: name, Responsibility: tpl.Responsibility, LeadAssignmentID: lead,
			MemberIDs: members, TemplateID: tpl.ID}
		return saveDepartmentTx(ctx, tx, &d)
	})
	if err != nil {
		return d, err
	}
	return db.Department(ctx, projectID, d.ID)
}

// SaveDepartmentAsTemplate copies a service department into a new company
// template, with the roles its members hold.
func (db *DB) SaveDepartmentAsTemplate(ctx context.Context, projectID, departmentID, name string) (domain.DepartmentTemplate, error) {
	d, err := db.Department(ctx, projectID, departmentID)
	if err != nil {
		return domain.DepartmentTemplate{}, err
	}
	if strings.TrimSpace(name) == "" {
		name = d.Name
	}
	var roles []string
	for _, m := range d.MemberIDs {
		var role string
		if err := db.sql.QueryRowContext(ctx, `SELECT role_id FROM assignments WHERE id = ?`, m).Scan(&role); err != nil {
			return domain.DepartmentTemplate{}, err
		}
		roles = append(roles, role)
	}
	return db.SaveDepartmentTemplate(ctx, domain.DepartmentTemplate{Name: name, Responsibility: d.Responsibility, RoleIDs: roles})
}

// departmentLayerTx is the instruction layer for an assignment that is a
// member of an active department of its service, or false.
func departmentLayerTx(ctx context.Context, q queryer, a domain.Assignment) (domain.InstructionLayer, bool, error) {
	var name, resp string
	err := q.QueryRowContext(ctx, `SELECT d.name, d.responsibility FROM department_members m
		JOIN departments d ON d.id = m.department_id
		WHERE m.assignment_id = ? AND d.status = 'active'`, a.ID).Scan(&name, &resp)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.InstructionLayer{}, false, nil
	}
	if err != nil {
		return domain.InstructionLayer{}, false, err
	}
	return domain.InstructionLayer{Source: "department", Name: "부서: " + name, Text: strings.TrimSpace(resp)}, true, nil
}

// ---- categories ----

func seedCategoriesTx(ctx context.Context, tx *sql.Tx, projectID string) error {
	for i, name := range domain.DefaultCategories {
		if _, err := tx.ExecContext(ctx, `INSERT INTO project_categories (project_id, name, position) VALUES (?, ?, ?)`, projectID, name, i+1); err != nil {
			return err
		}
	}
	return nil
}

func categoriesTx(ctx context.Context, q rowsQueryer, projectID string) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT name FROM project_categories WHERE project_id = ? ORDER BY position, name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// ProjectCategories is the service's fixed category list, in order.
func (db *DB) ProjectCategories(ctx context.Context, projectID string) ([]string, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	return categoriesTx(ctx, db.sql, projectID)
}

// SetProjectCategories replaces the service's category list. Work that
// still uses a removed category keeps it.
func (db *DB) SetProjectCategories(ctx context.Context, projectID string, names []string) ([]string, error) {
	names = dedupe(names)
	for _, n := range names {
		if len([]rune(n)) > 30 {
			return nil, invalid("분류 이름은 30자 이하로 적어 주세요")
		}
	}
	err := db.Write(ctx, func(tx *sql.Tx) error {
		p, err := projectTx(ctx, tx, projectID)
		if err != nil {
			return err
		}
		if p.Status == "archived" {
			return ErrArchived
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM project_categories WHERE project_id = ?`, projectID); err != nil {
			return err
		}
		for i, n := range names {
			if _, err := tx.ExecContext(ctx, `INSERT INTO project_categories (project_id, name, position) VALUES (?, ?, ?)`, projectID, n, i+1); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return db.ProjectCategories(ctx, projectID)
}

// ---- work items ----

const workCols = `id, project_id, kind, COALESCE(parent_id, ''), COALESCE(department_id, ''), COALESCE(assignee_assignment_id, ''),
	title, description, done_criteria, priority, status, status_reason, categories, COALESCE(due_at, ''), created_at, updated_at, COALESCE(closed_at, '')`

func scanWork(s interface{ Scan(...any) error }) (domain.WorkItem, error) {
	var w domain.WorkItem
	var cats string
	err := s.Scan(&w.ID, &w.ProjectID, &w.Kind, &w.ParentID, &w.DepartmentID, &w.AssigneeAssignmentID,
		&w.Title, &w.Description, &w.DoneCriteria, &w.Priority, &w.Status, &w.StatusReason, &cats, &w.DueAt, &w.CreatedAt, &w.UpdatedAt, &w.ClosedAt)
	if err == nil {
		json.Unmarshal([]byte(cats), &w.Categories)
		if w.Categories == nil {
			w.Categories = []string{}
		}
	}
	return w, err
}

func workTx(ctx context.Context, q queryer, projectID, id string) (domain.WorkItem, error) {
	w, err := scanWork(q.QueryRowContext(ctx, `SELECT `+workCols+` FROM work_items WHERE id = ? AND project_id = ?`, id, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return w, fmt.Errorf("work item %q: %w", id, ErrNotInProject)
	}
	return w, err
}

// WorkFilter narrows a service's work list. Empty fields match all.
type WorkFilter struct {
	Kind         string `json:"kind"`
	Status       string `json:"status"` // a status, or "open" / "closed"
	Priority     string `json:"priority"`
	Category     string `json:"category"`
	DepartmentID string `json:"departmentId"`
	ParentID     string `json:"parentId"`
	DueBefore    string `json:"dueBefore"` // YYYY-MM-DD, inclusive
}

// WorkItems lists a service's work, most urgent and soonest due first.
func (db *DB) WorkItems(ctx context.Context, projectID string, f WorkFilter) ([]domain.WorkItem, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	q := `SELECT ` + workCols + ` FROM work_items WHERE project_id = ?`
	args := []any{projectID}
	add := func(cond string, v any) {
		q += ` AND ` + cond
		args = append(args, v)
	}
	if f.Kind != "" {
		add(`kind = ?`, f.Kind)
	}
	switch f.Status {
	case "":
	case "open":
		q += ` AND status NOT IN ('done', 'cancelled')`
	case "closed":
		q += ` AND status IN ('done', 'cancelled')`
	default:
		add(`status = ?`, f.Status)
	}
	if f.Priority != "" {
		add(`priority = ?`, f.Priority)
	}
	if f.Category != "" {
		add(`EXISTS (SELECT 1 FROM json_each(work_items.categories) WHERE value = ?)`, f.Category)
	}
	if f.DepartmentID != "" {
		add(`department_id = ?`, f.DepartmentID)
	}
	if f.ParentID != "" {
		add(`parent_id = ?`, f.ParentID)
	}
	if f.DueBefore != "" {
		add(`due_at IS NOT NULL AND due_at <= ?`, f.DueBefore)
	}
	q += ` ORDER BY CASE WHEN status IN ('done', 'cancelled') THEN 1 ELSE 0 END,
		CASE priority WHEN 'urgent' THEN 0 WHEN 'high' THEN 1 WHEN 'normal' THEN 2 ELSE 3 END,
		CASE WHEN due_at IS NULL THEN 1 ELSE 0 END, due_at, created_at`
	rows, err := db.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WorkItem{}
	for rows.Next() {
		w, err := scanWork(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// WorkItem returns one work item of the service.
func (db *DB) WorkItem(ctx context.Context, projectID, id string) (domain.WorkItem, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return domain.WorkItem{}, err
	}
	return workTx(ctx, db.sql, projectID, id)
}

// WorkNotes returns a work item's history, oldest first.
func (db *DB) WorkNotes(ctx context.Context, projectID, workItemID string) ([]domain.WorkNote, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefWorkItem, workItemID}); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT id, work_item_id, kind, body, from_status, to_status, author, created_at
		FROM work_item_notes WHERE work_item_id = ? ORDER BY created_at, rowid`, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WorkNote{}
	for rows.Next() {
		var n domain.WorkNote
		if err := rows.Scan(&n.ID, &n.WorkItemID, &n.Kind, &n.Body, &n.FromStatus, &n.ToStatus, &n.Author, &n.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func writableProjectTx(ctx context.Context, tx *sql.Tx, projectID string) error {
	p, err := projectTx(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if p.Status == "archived" {
		return ErrArchived
	}
	return nil
}

// checkWorkLinks validates the department, assignee, parent and fields
// of a work item against the service before it is stored.
func checkWorkLinks(ctx context.Context, tx *sql.Tx, w domain.WorkItem, isNew bool) error {
	if w.DepartmentID != "" {
		d, err := departmentTx(ctx, tx, w.ProjectID, w.DepartmentID)
		if err != nil {
			return err
		}
		if d.Status != domain.StatusActive && isNew {
			return invalid("사용하지 않는 부서에는 새 업무를 맡길 수 없습니다")
		}
	}
	if w.Kind == domain.WorkGoal && w.DepartmentID == "" {
		return invalid("공동 목표에는 결과를 책임질 부서가 필요합니다")
	}
	if w.AssigneeAssignmentID != "" {
		if err := CheckScopeTx(ctx, tx, w.ProjectID, Ref{RefAssignment, w.AssigneeAssignmentID}); err != nil {
			return err
		}
	}
	if w.ParentID != "" {
		if w.Kind == domain.WorkGoal {
			return invalid("공동 목표는 다른 목표 아래에 둘 수 없습니다")
		}
		parent, err := workTx(ctx, tx, w.ProjectID, w.ParentID)
		if err != nil {
			return err
		}
		if parent.Kind != domain.WorkGoal {
			return invalid("상위 업무는 공동 목표여야 합니다")
		}
	}
	return nil
}

func normalizeWork(w *domain.WorkItem) error {
	w.Title = strings.TrimSpace(w.Title)
	if w.Title == "" {
		return invalid("업무 제목이 필요합니다")
	}
	if w.Kind == "" {
		w.Kind = domain.WorkTask
	}
	if w.Kind != domain.WorkGoal && w.Kind != domain.WorkTask {
		return invalid("업무 종류는 goal 또는 work입니다")
	}
	if w.Priority == "" {
		w.Priority = "normal"
	}
	if !slices.Contains(domain.WorkPriorities, w.Priority) {
		return invalid("우선순위는 urgent, high, normal, low 중 하나입니다")
	}
	if w.DueAt != "" {
		if _, err := time.Parse("2006-01-02", w.DueAt); err != nil {
			return invalid("기한은 YYYY-MM-DD 형식이어야 합니다")
		}
	}
	w.Categories = dedupe(w.Categories)
	return nil
}

// CreateWorkItem stores a new work item or goal. It starts as received
// unless a status is given; a waiting or blocked start needs a reason.
func (db *DB) CreateWorkItem(ctx context.Context, w domain.WorkItem) (domain.WorkItem, error) {
	if err := normalizeWork(&w); err != nil {
		return w, err
	}
	if w.Status == "" {
		w.Status = domain.WorkReceived
	}
	if domain.WorkClosed(w.Status) || !slices.Contains(domain.WorkStatuses, w.Status) {
		return w, invalid("새 업무는 진행 중인 상태로만 만들 수 있습니다")
	}
	if domain.WorkNeedsReason(w.Status) && strings.TrimSpace(w.StatusReason) == "" {
		return w, invalid("대기나 막힘 상태에는 이유가 필요합니다")
	}
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, w.ProjectID); err != nil {
			return err
		}
		if err := checkWorkLinks(ctx, c.Tx, w, true); err != nil {
			return err
		}
		id, err := insertWorkTx(ctx, c, w, 0, "")
		w.ID = id
		return err
	})
	if err != nil {
		return w, err
	}
	return db.WorkItem(ctx, w.ProjectID, w.ID)
}

// insertWorkTx stores a checked new work item with its first history
// entry and emits work.created. originSeq and originProposal record the
// event and proposal it came from, if any; the event is then its cause.
func insertWorkTx(ctx context.Context, c *Change, w domain.WorkItem, originSeq int64, originProposal string) (string, error) {
	if w.Status == "" {
		w.Status = domain.WorkReceived
	}
	cats, _ := json.Marshal(w.Categories)
	w.ID = NewID("work")
	now := Now()
	var seq any
	if originSeq > 0 {
		seq = originSeq
	}
	if _, err := c.Tx.ExecContext(ctx, `INSERT INTO work_items (id, project_id, kind, parent_id, department_id, assignee_assignment_id,
		title, description, done_criteria, priority, status, status_reason, categories, due_at, origin_event_sequence, origin_proposal_id, created_at, updated_at)
		VALUES (?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), ?, ?)`,
		w.ID, w.ProjectID, w.Kind, w.ParentID, w.DepartmentID, w.AssigneeAssignmentID,
		w.Title, w.Description, w.DoneCriteria, w.Priority, w.Status, strings.TrimSpace(w.StatusReason), string(cats), w.DueAt, seq, originProposal, now, now); err != nil {
		return "", err
	}
	if err := addNoteTx(ctx, c.Tx, w.ProjectID, w.ID, domain.NoteStatus, strings.TrimSpace(w.StatusReason), "", w.Status, domain.LocalOwner, now); err != nil {
		return "", err
	}
	ev := workEventFor(w)
	if originSeq > 0 {
		ev.CauseSequence = &originSeq
	}
	_, err := emitWorkEventTx(ctx, c, w.ProjectID, domain.EventWorkCreated, ev)
	return w.ID, err
}

// UpdateWorkItem changes a work item's content. Status changes go
// through SetWorkStatus so every change is kept in the history; the
// kind of an item never changes.
func (db *DB) UpdateWorkItem(ctx context.Context, w domain.WorkItem) (domain.WorkItem, error) {
	_, err := db.Change(ctx, func(c *Change) error {
		tx := c.Tx
		if err := writableProjectTx(ctx, tx, w.ProjectID); err != nil {
			return err
		}
		cur, err := workTx(ctx, tx, w.ProjectID, w.ID)
		if err != nil {
			return err
		}
		w.Kind = cur.Kind
		if err := normalizeWork(&w); err != nil {
			return err
		}
		if cur.Kind == domain.WorkGoal && w.ParentID != "" {
			return invalid("공동 목표는 다른 목표 아래에 둘 수 없습니다")
		}
		// An inactive department may keep the work it already has.
		if err := checkWorkLinks(ctx, tx, w, w.DepartmentID != cur.DepartmentID); err != nil {
			return err
		}
		cats, _ := json.Marshal(w.Categories)
		_, err = tx.ExecContext(ctx, `UPDATE work_items SET parent_id = NULLIF(?, ''), department_id = NULLIF(?, ''),
			assignee_assignment_id = NULLIF(?, ''), title = ?, description = ?, done_criteria = ?, priority = ?, categories = ?,
			due_at = NULLIF(?, ''), updated_at = ? WHERE id = ? AND project_id = ?`,
			w.ParentID, w.DepartmentID, w.AssigneeAssignmentID, w.Title, w.Description, w.DoneCriteria, w.Priority,
			string(cats), w.DueAt, Now(), w.ID, w.ProjectID)
		if err != nil || w.ParentID == cur.ParentID || w.ParentID == "" {
			return err
		}
		w.Status = cur.Status
		return flagMovedUnderDoneGoalTx(ctx, c, w)
	})
	if err != nil {
		return w, err
	}
	return db.WorkItem(ctx, w.ProjectID, w.ID)
}

// SetWorkStatus moves a work item to a new status and records the change
// in its history. Closing sets closed_at; reopening clears it but keeps
// every earlier note. Waiting and blocked need a reason.
func (db *DB) SetWorkStatus(ctx context.Context, projectID, id, status, reason string) (domain.WorkItem, error) {
	reason = strings.TrimSpace(reason)
	if !slices.Contains(domain.WorkStatuses, status) {
		return domain.WorkItem{}, invalid("알 수 없는 업무 상태입니다")
	}
	if domain.WorkNeedsReason(status) && reason == "" {
		return domain.WorkItem{}, invalid("대기나 막힘 상태에는 이유가 필요합니다")
	}
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		cur, err := workTx(ctx, c.Tx, projectID, id)
		if err != nil {
			return err
		}
		return setWorkStatusTx(ctx, c, cur, status, reason, domain.LocalOwner)
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return db.WorkItem(ctx, projectID, id)
}

// AddWorkNote appends a person's note to a work item.
func (db *DB) AddWorkNote(ctx context.Context, projectID, id, body string) (domain.WorkNote, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return domain.WorkNote{}, invalid("메모 내용이 필요합니다")
	}
	var noteID string
	err := db.Write(ctx, func(tx *sql.Tx) error {
		if err := writableProjectTx(ctx, tx, projectID); err != nil {
			return err
		}
		if _, err := workTx(ctx, tx, projectID, id); err != nil {
			return err
		}
		noteID = NewID("wnote")
		_, err := tx.ExecContext(ctx, `INSERT INTO work_item_notes (id, project_id, work_item_id, kind, body, author, created_at)
			VALUES (?, ?, ?, 'note', ?, ?, ?)`, noteID, projectID, id, body, domain.LocalOwner, Now())
		return err
	})
	if err != nil {
		return domain.WorkNote{}, err
	}
	notes, err := db.WorkNotes(ctx, projectID, id)
	for _, n := range notes {
		if n.ID == noteID {
			return n, err
		}
	}
	return domain.WorkNote{}, err
}

// setWorkStatusTx moves cur to status, keeps the change in the history
// with its author, and emits the completed or reopened event. It is a
// no-op when nothing changes.
func setWorkStatusTx(ctx context.Context, c *Change, cur domain.WorkItem, status, reason, author string) error {
	if cur.Status == status && cur.StatusReason == reason {
		return nil
	}
	// Completing checks the required collaboration in the transaction
	// that stores it.
	if status == domain.WorkDone && cur.Status != domain.WorkDone {
		if err := checkRequiredMetTx(ctx, c.Tx, cur); err != nil {
			return err
		}
	}
	now := Now()
	closed := sql.NullString{String: cur.ClosedAt, Valid: cur.ClosedAt != ""}
	switch {
	case domain.WorkClosed(status) && !domain.WorkClosed(cur.Status):
		closed = sql.NullString{String: now, Valid: true}
	case !domain.WorkClosed(status):
		closed = sql.NullString{}
	}
	// Only waiting and blocked keep a reason on the item itself; the
	// history keeps every reason.
	stored := ""
	if domain.WorkNeedsReason(status) {
		stored = reason
	}
	if _, err := c.Tx.ExecContext(ctx, `UPDATE work_items SET status = ?, status_reason = ?, closed_at = ?, updated_at = ? WHERE id = ? AND project_id = ?`,
		status, stored, closed, now, cur.ID, cur.ProjectID); err != nil {
		return err
	}
	if err := addNoteTx(ctx, c.Tx, cur.ProjectID, cur.ID, domain.NoteStatus, reason, cur.Status, status, author, now); err != nil {
		return err
	}
	// Completing and reopening are events other departments may react
	// to; other status changes stay in the item's history only.
	ev := workEventFor(cur)
	ev.Note = reason
	var err error
	switch {
	case status == domain.WorkDone && cur.Status != domain.WorkDone:
		_, err = emitWorkEventTx(ctx, c, cur.ProjectID, domain.EventWorkCompleted, ev)
	case domain.WorkClosed(cur.Status) && !domain.WorkClosed(status):
		_, err = emitWorkEventTx(ctx, c, cur.ProjectID, domain.EventWorkReopened, ev)
	}
	if err != nil || cur.Status != domain.WorkDone || status == domain.WorkDone {
		return err
	}
	// Work another department required is no longer done: the asking
	// work and its goal are told, never reopened behind the person's back.
	return requiredWorkLeftDoneTx(ctx, c, cur, status)
}

func addNoteTx(ctx context.Context, tx *sql.Tx, projectID, workID, kind, body, from, to, author, now string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO work_item_notes (id, project_id, work_item_id, kind, body, from_status, to_status, author, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, NewID("wnote"), projectID, workID, kind, body, from, to, author, now)
	return err
}
