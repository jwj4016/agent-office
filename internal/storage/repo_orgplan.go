package storage

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"strings"

	"agent-office/internal/domain"
)

// OrgApplied says what applying an organization plan did.
type OrgApplied struct {
	Departments []string `json:"departments"` // created
	Skipped     []string `json:"skipped"`     // already there, by name
	Desk        string   `json:"desk"`        // the desk created, if any
	AutoStart   bool     `json:"autoStart"`
	// AdoptedWork counts open work that joined its assignee's new department.
	AdoptedWork int      `json:"adoptedWork"`
	Problems    []string `json:"problems"` // parts that could not be created
}

// ApplyOrgPlan creates a plan's departments (members, lead, watches) and
// desk in a service. ids maps the plan's member refs to assignment ids
// (nil when they already are ids). In an auto-mode service the new
// departments start work, share drafts and decide reference requests on
// their own, within the service's auto policy and budget. A department
// whose name exists and a desk that exists are left as they are; a part
// that cannot be created is reported, not fatal.
func (db *DB) ApplyOrgPlan(ctx context.Context, projectID string, plan domain.OrgPlan, ids map[string]string) (OrgApplied, error) {
	out := OrgApplied{Departments: []string{}, Skipped: []string{}, Problems: []string{}}
	p, err := db.Project(ctx, projectID)
	if err != nil {
		return out, err
	}
	existing, err := db.Departments(ctx, projectID)
	if err != nil {
		return out, err
	}
	id := func(ref string) string {
		if ids == nil {
			return ref
		}
		return ids[ref]
	}
	out.AutoStart = p.Mode == domain.ModeAuto
	for _, d := range plan.Departments {
		name := strings.TrimSpace(d.Name)
		if slices.ContainsFunc(existing, func(x domain.Department) bool { return x.Name == name }) {
			out.Skipped = append(out.Skipped, name)
			continue
		}
		var members []string
		for _, m := range d.Members {
			if v := id(m); v != "" {
				members = append(members, v)
			}
		}
		dep, err := db.SaveDepartment(ctx, domain.Department{ProjectID: projectID, Name: name, Responsibility: strings.TrimSpace(d.Responsibility),
			MemberIDs: members, LeadAssignmentID: id(d.Lead), Status: domain.StatusActive})
		if err != nil {
			out.Problems = append(out.Problems, fmt.Sprintf("부서 ‘%s’: %v", name, err))
			continue
		}
		out.Departments = append(out.Departments, name)
		for _, w := range d.Watches {
			if _, err := db.SaveSubscription(ctx, domain.Subscription{ProjectID: projectID, DepartmentID: dep.ID, EventKinds: w.EventKinds,
				Guidance: strings.TrimSpace(w.Guidance), Enabled: true}); err != nil {
				out.Problems = append(out.Problems, fmt.Sprintf("부서 ‘%s’의 받아 볼 사건: %v", name, err))
			}
		}
		if out.AutoStart {
			if _, err := db.SetDepartmentAutomation(ctx, projectID, dep.ID, domain.DepartmentAutomation{AutoStart: true, AutoShare: true, AutoCollab: true}); err != nil {
				out.Problems = append(out.Problems, fmt.Sprintf("부서 ‘%s’의 자동 시작: %v", name, err))
			}
		}
	}
	if pd := plan.IntakeDesk; pd != nil {
		if _, ok, err := db.IntakeDesk(ctx, projectID); err != nil {
			return out, err
		} else if !ok {
			conn, model, found, err := db.PreferredConnection(ctx, projectID)
			switch {
			case err != nil:
				return out, err
			case !found:
				out.Problems = append(out.Problems, "검증된 AI 연결이 없어 접수 담당을 만들지 못했습니다")
			default:
				if _, a, err := db.CreateDeskAssignee(ctx, projectID, pd.Name, conn.ID, model, pd.Instructions); err != nil {
					out.Problems = append(out.Problems, "접수 담당: "+err.Error())
				} else {
					out.Desk = a.DisplayName
				}
			}
		}
	}
	return out, nil
}

// AdoptWorkIntoDepartments gives open work without a department its
// assignee's department, and queues the work that came from an event or a
// request for the auto starter (it decides later whether it may start).
// It returns how many work items it moved.
func (db *DB) AdoptWorkIntoDepartments(ctx context.Context, projectID string) (int, error) {
	n := 0
	_, err := db.Change(ctx, func(c *Change) error {
		rows, err := c.Tx.QueryContext(ctx, `SELECT w.id, m.department_id FROM work_items w
			JOIN department_members m ON m.assignment_id = w.assignee_assignment_id AND m.project_id = w.project_id
			WHERE w.project_id = ? AND w.kind = 'work' AND w.department_id IS NULL AND w.status IN ('received', 'ready')`, projectID)
		if err != nil {
			return err
		}
		type move struct{ work, dept string }
		var moves []move
		for rows.Next() {
			var m move
			rows.Scan(&m.work, &m.dept)
			moves = append(moves, m)
		}
		rows.Close()
		for _, m := range moves {
			if _, err := c.Tx.ExecContext(ctx, `UPDATE work_items SET department_id = ?, updated_at = ? WHERE id = ? AND project_id = ?`,
				m.dept, Now(), m.work, projectID); err != nil {
				return err
			}
			w, err := workTx(ctx, c.Tx, projectID, m.work)
			if err != nil {
				return err
			}
			if err := addNoteTx(ctx, c.Tx, projectID, w.ID, domain.NoteMemo, "조직 정비로 담당자의 부서에 들어갔습니다", "", "", domain.LocalOwner, Now()); err != nil {
				return err
			}
			var seq sql.NullInt64
			c.Tx.QueryRowContext(ctx, `SELECT origin_event_sequence FROM work_items WHERE id = ?`, w.ID).Scan(&seq)
			if seq.Valid {
				if err := queueAutoStartTx(ctx, c, w, seq.Int64); err != nil {
					return err
				}
			}
			n++
		}
		return nil
	})
	return n, err
}
