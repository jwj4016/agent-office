package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-office/internal/domain"
)

// ScheduleView is a department's schedule with when it last ran and runs
// next (empty when it is off or cannot run).
type ScheduleView struct {
	domain.DepartmentSchedule
	DepartmentName string `json:"departmentName"`
	LastSlotAt     string `json:"lastSlotAt"`
	LastSequence   int64  `json:"lastSequence"`
	NextDueAt      string `json:"nextDueAt"`
}

const scheduleCols = `s.id, s.project_id, s.department_id, s.frequency, s.time_of_day, s.weekday, s.timezone, s.note, s.enabled,
	s.active_from, s.created_at, s.updated_at`

func scanSchedule(row interface{ Scan(...any) error }, extra ...any) (domain.DepartmentSchedule, error) {
	var s domain.DepartmentSchedule
	var weekday sql.NullInt64
	var enabled int
	err := row.Scan(append([]any{&s.ID, &s.ProjectID, &s.DepartmentID, &s.Frequency, &s.TimeOfDay, &weekday, &s.Timezone, &s.Note,
		&enabled, &s.ActiveFrom, &s.CreatedAt, &s.UpdatedAt}, extra...)...)
	if weekday.Valid {
		w := int(weekday.Int64)
		s.Weekday = &w
	}
	s.Enabled = enabled == 1
	return s, err
}

// DepartmentSchedules lists a service's schedules with their last and
// next slot.
func (db *DB) DepartmentSchedules(ctx context.Context, projectID string, now time.Time) ([]ScheduleView, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+scheduleCols+`, d.name, d.status, p.status,
		COALESCE((SELECT MAX(due_at) FROM schedule_slots l WHERE l.schedule_id = s.id), ''),
		COALESCE((SELECT event_sequence FROM schedule_slots l WHERE l.schedule_id = s.id ORDER BY due_at DESC LIMIT 1), 0)
		FROM department_schedules s JOIN departments d ON d.id = s.department_id JOIN projects p ON p.id = s.project_id
		WHERE s.project_id = ? ORDER BY d.name`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ScheduleView{}
	for rows.Next() {
		var v ScheduleView
		var deptStatus, projectStatus string
		v.DepartmentSchedule, err = scanSchedule(rows, &v.DepartmentName, &deptStatus, &projectStatus, &v.LastSlotAt, &v.LastSequence)
		if err != nil {
			return nil, err
		}
		if v.Enabled && deptStatus == domain.StatusActive && projectStatus != "archived" {
			if next, err := v.NextSlot(now); err == nil {
				v.NextDueAt = next.UTC().Format(time.RFC3339)
			}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// SaveDepartmentSchedule creates or changes a department's one schedule.
// Every save moves active_from to now, so a slot that was already due is
// not run because of the save; a slot already run is never run again.
func (db *DB) SaveDepartmentSchedule(ctx context.Context, s domain.DepartmentSchedule) (domain.DepartmentSchedule, error) {
	s.Note = strings.TrimSpace(s.Note)
	if s.Frequency == domain.ScheduleDaily {
		s.Weekday = nil
	}
	if err := s.Check(); err != nil {
		return s, invalid("%s", err.Error())
	}
	var weekday any
	if s.Weekday != nil {
		weekday = *s.Weekday
	}
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, s.ProjectID); err != nil {
			return err
		}
		if _, err := departmentTx(ctx, c.Tx, s.ProjectID, s.DepartmentID); err != nil {
			return err
		}
		now := Now()
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO department_schedules (id, project_id, department_id, frequency, time_of_day, weekday,
			timezone, note, enabled, active_from, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(department_id) DO UPDATE SET frequency = excluded.frequency, time_of_day = excluded.time_of_day,
				weekday = excluded.weekday, timezone = excluded.timezone, note = excluded.note, enabled = excluded.enabled,
				active_from = excluded.active_from, updated_at = excluded.updated_at`,
			NewID("sch"), s.ProjectID, s.DepartmentID, s.Frequency, s.TimeOfDay, weekday, s.Timezone, s.Note, enabled, now, now, now); err != nil {
			return err
		}
		return c.Emit(s.ProjectID, "", "", "schedule.updated", map[string]string{"departmentId": s.DepartmentID})
	})
	if err != nil {
		return s, err
	}
	return scanSchedule(db.sql.QueryRowContext(ctx, `SELECT `+scheduleCols+` FROM department_schedules s WHERE s.department_id = ?`, s.DepartmentID))
}

// DueSchedules lists the schedules that may run: turned on, of an active
// department in a service that is not archived.
func (db *DB) DueSchedules(ctx context.Context) ([]domain.DepartmentSchedule, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+scheduleCols+` FROM department_schedules s
		JOIN departments d ON d.id = s.department_id JOIN projects p ON p.id = s.project_id
		WHERE s.enabled = 1 AND d.status = 'active' AND p.status <> 'archived' ORDER BY s.created_at, s.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.DepartmentSchedule
	for rows.Next() {
		s, err := scanSchedule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// RunScheduleSlot asks the department to check, once per slot: in one
// transaction it records the slot and stores the department check event
// (whose delivery goes to the same judgment as a person's request). It
// reports false without an event when the slot already ran, or when the
// schedule was turned off, changed after the slot was due, or its
// department or service stopped meanwhile.
func (db *DB) RunScheduleSlot(ctx context.Context, scheduleID, slotKey string, due time.Time) (bool, error) {
	ran := false
	_, err := db.Change(ctx, func(c *Change) error {
		var d domain.Department
		s, err := scanSchedule(c.Tx.QueryRowContext(ctx, `SELECT `+scheduleCols+` FROM department_schedules s WHERE s.id = ?`, scheduleID))
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if from, err := time.Parse(time.RFC3339Nano, s.ActiveFrom); !s.Enabled || err != nil || due.Before(from) {
			return err
		}
		if err := writableProjectTx(ctx, c.Tx, s.ProjectID); errors.Is(err, ErrArchived) {
			return nil
		} else if err != nil {
			return err
		}
		if d, err = departmentTx(ctx, c.Tx, s.ProjectID, s.DepartmentID); err != nil {
			return err
		}
		if d.Status != domain.StatusActive {
			return nil
		}
		var exists int
		if err := c.Tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM schedule_slots WHERE schedule_id = ? AND slot_key = ?`, s.ID, slotKey).Scan(&exists); err != nil {
			return err
		}
		if exists > 0 {
			return nil
		}
		rec, err := emitWorkEventTx(ctx, c, s.ProjectID, domain.EventDepartmentCheck, WorkEvent{TargetDepartmentID: d.ID,
			Title: d.Name + " 정기 점검", Note: s.Note, ScheduleSlot: slotKey})
		if err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO schedule_slots (schedule_id, project_id, slot_key, due_at, event_sequence, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`, s.ID, s.ProjectID, slotKey, due.UTC().Format(time.RFC3339), rec.Sequence, Now()); err != nil {
			return fmt.Errorf("schedule slot %s: %w", slotKey, err)
		}
		ran = true
		return nil
	})
	return ran, err
}
