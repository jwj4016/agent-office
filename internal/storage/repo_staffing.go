package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"agent-office/internal/domain"
)

// Desk staffing (company-requests plan, 3rd revision): an AI writes a new
// desk's name and instructions, or rewrites a desk's instructions. Every
// call is kept in desk_staffing with the usage it reported.

// DeskRoleName is the one company role every service desk assignee
// shares; what differs per service is in the assignee's instructions.
const DeskRoleName = "접수 담당"

const deskRoleMission = "대표와 회사 접수 담당이 서비스에 보낸 요청을 받아 처리 방법을 정한다."

const deskRoleInstructions = `요청을 받으면 직접 코드를 쓰거나 파일을 만들지 않고 처리 방법을 정한다.
- 주어진 정보만으로 답할 수 있으면 짧게 답하고, 없는 내용은 지어내지 않는다.
- 담당자가 해야 할 일은 완료 조건이 분명한 업무로 나눠 제안한다. 이미 진행 중인 업무와 겹치게 만들지 않는다.
- 앱이 하는 조치는 이유와 바뀌는 것을 적어 제안하고, 실행은 대표의 승인에 맡긴다.
- 꼭 필요한 정보가 없을 때만 한 가지를 묻는다.
- 법률·재무 판단과 비용이 큰 결정은 직접 정하지 않고 대표에게 올린다.
- 대표에게는 결론을 먼저, 근거는 짧게 쓴다.`

// EnsureDeskRole returns the shared desk role, creating it the first time.
func (db *DB) EnsureDeskRole(ctx context.Context) (domain.Role, error) {
	roles, err := db.Roles(ctx)
	if err != nil {
		return domain.Role{}, err
	}
	for _, r := range roles {
		if r.Name == DeskRoleName {
			return r, nil
		}
	}
	return db.SaveRole(ctx, domain.Role{Name: DeskRoleName, Mission: deskRoleMission, Instructions: deskRoleInstructions})
}

// Staffing is one desk-writing call.
type Staffing struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"projectId"`
	RequestID    string          `json:"requestId"`
	Kind         string          `json:"kind"` // create | rewrite
	ConnectionID string          `json:"connectionId"`
	Model        string          `json:"model"`
	Status       string          `json:"status"`
	Result       json.RawMessage `json:"result"`
	Error        string          `json:"error"`
	StartedAt    string          `json:"startedAt"`
	EndedAt      string          `json:"endedAt"`
}

// StaffingResult is what a desk-writing call wrote.
type StaffingResult struct {
	Name         string `json:"name"`
	Instructions string `json:"instructions"`
	AssignmentID string `json:"assignmentId,omitempty"`
}

// StartStaffing records a desk-writing call before it is made.
func (db *DB) StartStaffing(ctx context.Context, s Staffing) (string, error) {
	id := NewID("stf")
	err := db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO desk_staffing (id, organization_id, project_id, request_id, kind, connection_id, model, status, started_at)
			VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''), ?, 'working', ?)`,
			id, domain.DefaultOrganizationID, s.ProjectID, s.RequestID, s.Kind, s.ConnectionID, s.Model, Now())
		return err
	})
	return id, err
}

// RecordStaffingUsage keeps the latest running total a call reported.
func (db *DB) RecordStaffingUsage(ctx context.Context, id string, usage json.RawMessage) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE desk_staffing SET usage = ?, usage_reported = 1 WHERE id = ?`, string(usage), id)
		return err
	})
}

// FinishStaffing ends a working call with what it wrote or why it failed.
func (db *DB) FinishStaffing(ctx context.Context, id string, result *StaffingResult, errText string) error {
	status, raw := "succeeded", []byte("{}")
	if errText != "" {
		status = "failed"
	}
	if result != nil {
		raw, _ = json.Marshal(result)
	}
	err := db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE desk_staffing SET status = ?, result = ?, error = ?, ended_at = ? WHERE id = ? AND status = 'working'`,
			status, string(raw), errText, Now(), id)
		return err
	})
	if err == nil {
		db.requestChanged()
	}
	return err
}

// StaffingTries counts the calls made to staff a desk for a request.
func (db *DB) StaffingTries(ctx context.Context, requestID string) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM desk_staffing WHERE request_id = ? AND kind = 'create'`, requestID).Scan(&n)
	return n, err
}

// LatestStaffing returns the last desk-writing call of a service ("" for
// the company).
func (db *DB) LatestStaffing(ctx context.Context, projectID string) (Staffing, bool, error) {
	var s Staffing
	var result string
	q := `SELECT id, COALESCE(project_id, ''), COALESCE(request_id, ''), kind, COALESCE(connection_id, ''), model, status, result, error,
		started_at, COALESCE(ended_at, '') FROM desk_staffing WHERE `
	var row *sql.Row
	if projectID == "" {
		row = db.sql.QueryRowContext(ctx, q+`project_id IS NULL ORDER BY started_at DESC, rowid DESC LIMIT 1`)
	} else {
		row = db.sql.QueryRowContext(ctx, q+`project_id = ? ORDER BY started_at DESC, rowid DESC LIMIT 1`, projectID)
	}
	err := row.Scan(&s.ID, &s.ProjectID, &s.RequestID, &s.Kind, &s.ConnectionID, &s.Model, &s.Status, &result, &s.Error, &s.StartedAt, &s.EndedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return s, false, nil
	}
	s.Result = json.RawMessage(result)
	return s, err == nil, err
}

// PreferredConnection is the verified connection a service's AI assignees
// use most (projectID "" counts the whole company), with the model they
// use most with it. ok is false when there is none.
func (db *DB) PreferredConnection(ctx context.Context, projectID string) (domain.ProviderConnection, string, bool, error) {
	q := `SELECT a.connection_id, a.model FROM assignments a JOIN projects p ON p.id = a.project_id
		WHERE a.actor_kind = 'ai' AND a.connection_id IS NOT NULL AND p.status = 'active'`
	args := []any{}
	if projectID != "" {
		q += ` AND a.project_id = ?`
		args = append(args, projectID)
	}
	rows, err := db.sql.QueryContext(ctx, q, args...)
	if err != nil {
		return domain.ProviderConnection{}, "", false, err
	}
	conns, models := map[string]int{}, map[string]map[string]int{}
	var order []string
	for rows.Next() {
		var c, m string
		rows.Scan(&c, &m)
		if conns[c] == 0 {
			order = append(order, c)
			models[c] = map[string]int{}
		}
		conns[c]++
		models[c][m]++
	}
	rows.Close()
	best, bestN := "", 0
	for _, c := range order {
		conn, err := db.Connection(ctx, c)
		if err != nil || !(conn.Provider == "test" || conn.Verified()) {
			continue
		}
		if conns[c] > bestN {
			best, bestN = c, conns[c]
		}
	}
	if best == "" {
		if projectID != "" {
			return db.PreferredConnection(ctx, "")
		}
		return domain.ProviderConnection{}, "", false, nil
	}
	model, n := "", 0
	for m, k := range models[best] {
		if k > n || (k == n && strings.Compare(m, model) < 0) {
			model, n = m, k
		}
	}
	conn, err := db.Connection(ctx, best)
	return conn, model, err == nil, err
}

// CreateDeskAssignee makes a new AI assignee in the shared desk role and
// sets it as the service's desk, in that order.
func (db *DB) CreateDeskAssignee(ctx context.Context, projectID, name, connectionID, model, instructions string) (domain.IntakeDesk, domain.Assignment, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.IntakeDesk{}, domain.Assignment{}, invalid("접수 담당 이름이 필요합니다")
	}
	if connectionID == "" {
		return domain.IntakeDesk{}, domain.Assignment{}, invalid("접수 담당의 AI 연결을 골라 주세요")
	}
	if err := db.CheckScope(ctx, projectID); err != nil {
		return domain.IntakeDesk{}, domain.Assignment{}, err
	}
	role, err := db.EnsureDeskRole(ctx)
	if err != nil {
		return domain.IntakeDesk{}, domain.Assignment{}, err
	}
	a, err := db.SaveAssignment(ctx, domain.Assignment{ProjectID: projectID, RoleID: role.ID, ActorKind: domain.ActorAI, DisplayName: name,
		ConnectionID: connectionID, Model: strings.TrimSpace(model), Overrides: domain.AssignmentOverrides{Instructions: strings.TrimSpace(instructions)}})
	if err != nil {
		return domain.IntakeDesk{}, a, err
	}
	d, err := db.SaveIntakeDesk(ctx, domain.IntakeDesk{ProjectID: projectID, AssignmentID: a.ID, Enabled: true})
	return d, a, err
}
