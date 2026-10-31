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

// Company requests (docs/plan/company-requests/plan.md). A request is its
// own AI job: the intake runner claims it from received to triaging, and
// every provider call is a request_attempts row (plan §4.3, §4.7).

// EventRequestUpdated lets open screens of a service refresh when one of
// its requests changes. It is not a work event: no department gets it.
const EventRequestUpdated = "request.updated"

// maxRequestBody bounds what a person may write in one request.
const maxRequestBody = 4000

// SetRequestHook sets a function called after any request changes, for
// screens that show company requests (which have no service events).
func (db *DB) SetRequestHook(fn func()) { db.requestHook.Store(&fn) }

func (db *DB) requestChanged() {
	if fn := db.requestHook.Load(); fn != nil && *fn != nil {
		(*fn)()
	}
}

// requestChange is Change for request writes: it tells the hook after a
// successful commit.
func (db *DB) requestChange(ctx context.Context, fn func(c *Change) error) error {
	_, err := db.Change(ctx, fn)
	if err == nil {
		db.requestChanged()
	}
	return err
}

// ---- intake desks ----

const deskCols = `id, organization_id, COALESCE(project_id, ''), name, COALESCE(assignment_id, ''), COALESCE(connection_id, ''), model,
	instructions, budget, enabled, created_at, updated_at`

func scanDesk(s interface{ Scan(...any) error }) (domain.IntakeDesk, error) {
	var d domain.IntakeDesk
	var budget string
	var enabled int
	err := s.Scan(&d.ID, &d.OrganizationID, &d.ProjectID, &d.Name, &d.AssignmentID, &d.ConnectionID, &d.Model,
		&d.Instructions, &budget, &enabled, &d.CreatedAt, &d.UpdatedAt)
	d.Enabled = enabled == 1
	json.Unmarshal([]byte(budget), &d.Budget)
	return d, err
}

func deskTx(ctx context.Context, q queryer, projectID string) (domain.IntakeDesk, bool, error) {
	var d domain.IntakeDesk
	var err error
	if projectID == "" {
		d, err = scanDesk(q.QueryRowContext(ctx, `SELECT `+deskCols+` FROM intake_desks WHERE organization_id = ? AND project_id IS NULL`, domain.DefaultOrganizationID))
	} else {
		d, err = scanDesk(q.QueryRowContext(ctx, `SELECT `+deskCols+` FROM intake_desks WHERE project_id = ?`, projectID))
	}
	if errors.Is(err, sql.ErrNoRows) {
		return domain.IntakeDesk{}, false, nil
	}
	return d, err == nil, err
}

// IntakeDesk returns the company desk (projectID "") or a service's desk;
// ok is false when none was set up.
func (db *DB) IntakeDesk(ctx context.Context, projectID string) (domain.IntakeDesk, bool, error) {
	if projectID != "" {
		if err := db.CheckScope(ctx, projectID); err != nil {
			return domain.IntakeDesk{}, false, err
		}
	}
	return deskTx(ctx, db.sql, projectID)
}

// SaveIntakeDesk sets up the company desk (ProjectID "": a connection and
// model of its own) or a service's desk (one of its AI assignments).
// Requests that waited for that desk go back to the queue.
func (db *DB) SaveIntakeDesk(ctx context.Context, d domain.IntakeDesk) (domain.IntakeDesk, error) {
	d.Name, d.Instructions, d.Model = strings.TrimSpace(d.Name), strings.TrimSpace(d.Instructions), strings.TrimSpace(d.Model)
	if b := d.Budget.MonthlyCostUSD; b != nil && *b <= 0 {
		return d, invalid("월 예산은 0보다 커야 합니다 (제한하지 않으려면 비워 두세요)")
	}
	err := db.requestChange(ctx, func(c *Change) error {
		if d.ProjectID == "" {
			if d.Name == "" {
				d.Name = "비서실"
			}
			d.AssignmentID = ""
			if d.ConnectionID != "" {
				if _, err := connectionTx(ctx, c.Tx, d.ConnectionID); err != nil {
					return invalid("AI 연결을 찾을 수 없습니다")
				}
			}
		} else {
			if err := writableProjectTx(ctx, c.Tx, d.ProjectID); err != nil {
				return err
			}
			a, err := scanAssignment(c.Tx.QueryRowContext(ctx, `SELECT `+assignmentCols+` FROM assignments WHERE id = ? AND project_id = ?`, d.AssignmentID, d.ProjectID))
			if err != nil {
				return invalid("접수 담당으로 고른 담당자를 이 서비스에서 찾을 수 없습니다")
			}
			if a.ActorKind != domain.ActorAI {
				return invalid("접수 담당은 AI 담당자여야 합니다")
			}
			if d.Name == "" {
				d.Name = a.DisplayName
			}
			d.ConnectionID, d.Model, d.Budget = "", "", domain.DeskBudget{}
		}
		budget, _ := json.Marshal(d.Budget)
		cur, ok, err := deskTx(ctx, c.Tx, d.ProjectID)
		if err != nil {
			return err
		}
		now := Now()
		if ok {
			d.ID = cur.ID
			if _, err := c.Tx.ExecContext(ctx, `UPDATE intake_desks SET name = ?, assignment_id = NULLIF(?, ''), connection_id = NULLIF(?, ''), model = ?,
				instructions = ?, budget = ?, enabled = ?, updated_at = ? WHERE id = ?`,
				d.Name, d.AssignmentID, d.ConnectionID, d.Model, d.Instructions, string(budget), boolInt(d.Enabled), now, d.ID); err != nil {
				return err
			}
		} else {
			d.ID = NewID("desk")
			if _, err := c.Tx.ExecContext(ctx, `INSERT INTO intake_desks (id, organization_id, project_id, name, assignment_id, connection_id, model,
				instructions, budget, enabled, created_at, updated_at) VALUES (?, ?, NULLIF(?, ''), ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?, ?, ?)`,
				d.ID, domain.DefaultOrganizationID, d.ProjectID, d.Name, d.AssignmentID, d.ConnectionID, d.Model, d.Instructions, string(budget),
				boolInt(d.Enabled), now, now); err != nil {
				return err
			}
		}
		return requeueWaitingTx(ctx, c, d.ProjectID)
	})
	if err != nil {
		return d, err
	}
	out, _, err := deskTx(ctx, db.sql, d.ProjectID)
	return out, err
}

// RemoveIntakeDesk takes a service's desk away; new requests then wait.
func (db *DB) RemoveIntakeDesk(ctx context.Context, projectID string) error {
	if projectID == "" {
		return invalid("회사 접수 담당은 지우지 않고 ‘사용 안 함’으로 둡니다")
	}
	return db.requestChange(ctx, func(c *Change) error {
		if err := CheckScopeTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		_, err := c.Tx.ExecContext(ctx, `DELETE FROM intake_desks WHERE project_id = ?`, projectID)
		return err
	})
}

// requeueWaitingTx gives requests that waited for a desk (or for the
// budget) back to the runner, which checks them again.
func requeueWaitingTx(ctx context.Context, c *Change, projectID string) error {
	var err error
	if projectID == "" {
		_, err = c.Tx.ExecContext(ctx, `UPDATE requests SET status = 'received', hold_reason = '', updated_at = ?
			WHERE project_id IS NULL AND status IN ('waiting_desk', 'held')`, Now())
	} else {
		_, err = c.Tx.ExecContext(ctx, `UPDATE requests SET status = 'received', hold_reason = '', updated_at = ?
			WHERE project_id = ? AND status IN ('waiting_desk', 'held')`, Now(), projectID)
	}
	return err
}

func connectionTx(ctx context.Context, q queryer, id string) (string, error) {
	var name string
	err := q.QueryRowContext(ctx, `SELECT name FROM provider_connections WHERE id = ?`, id).Scan(&name)
	return name, err
}

// ---- requests ----

const requestCols = `r.id, r.organization_id, COALESCE(r.project_id, ''), COALESCE(p.name, ''), COALESCE(r.parent_id, ''), COALESCE(r.previous_id, ''),
	r.body, r.status, r.outcome, r.round, r.answer, r.question, r.reply, r.reason, r.error, r.hold_reason, r.max_attempts,
	(SELECT COUNT(*) FROM request_attempts a WHERE a.request_id = r.id AND a.round = r.round), r.created_at, r.updated_at, COALESCE(r.closed_at, ''),
	COALESCE((SELECT json_extract(s.result, '$.name') FROM desk_staffing s WHERE s.request_id = r.id AND s.kind = 'create' AND s.status = 'succeeded'
		ORDER BY s.started_at DESC LIMIT 1), '')`

const requestFrom = ` FROM requests r LEFT JOIN projects p ON p.id = r.project_id`

func scanRequest(s interface{ Scan(...any) error }) (domain.Request, error) {
	var r domain.Request
	err := s.Scan(&r.ID, &r.OrganizationID, &r.ProjectID, &r.ProjectName, &r.ParentID, &r.PreviousID, &r.Body, &r.Status, &r.Outcome, &r.Round,
		&r.Answer, &r.Question, &r.Reply, &r.Reason, &r.Error, &r.HoldReason, &r.MaxAttempts, &r.Attempts, &r.CreatedAt, &r.UpdatedAt, &r.ClosedAt, &r.StaffedDesk)
	r.Items, r.Children = []domain.RequestItem{}, []domain.Request{}
	return r, err
}

func requestTx(ctx context.Context, q queryer, id string) (domain.Request, error) {
	r, err := scanRequest(q.QueryRowContext(ctx, `SELECT `+requestCols+requestFrom+` WHERE r.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("request %q: %w", id, ErrNotInProject)
	}
	return r, err
}

// scopedRequestTx returns a request of the given service ("" for a
// company request), refusing one of another scope.
func scopedRequestTx(ctx context.Context, q queryer, projectID, id string) (domain.Request, error) {
	r, err := requestTx(ctx, q, id)
	if err == nil && r.ProjectID != projectID {
		return domain.Request{}, fmt.Errorf("request %q: %w", id, ErrNotInProject)
	}
	return r, err
}

const itemCols = `i.id, i.request_id, COALESCE(i.project_id, ''), i.round, i.position, i.kind, i.payload, i.status, COALESCE(i.work_item_id, ''),
	COALESCE(w.title, ''), COALESCE(w.status, ''), COALESCE(i.child_request_id, ''), i.result, i.error, i.decided_by, i.decision_note,
	COALESCE(i.decided_at, ''), i.created_at`

const itemFrom = ` FROM request_items i LEFT JOIN work_items w ON w.id = i.work_item_id`

func scanItem(s interface{ Scan(...any) error }) (domain.RequestItem, error) {
	var it domain.RequestItem
	var payload, result string
	err := s.Scan(&it.ID, &it.RequestID, &it.ProjectID, &it.Round, &it.Position, &it.Kind, &payload, &it.Status, &it.WorkItemID,
		&it.WorkTitle, &it.WorkStatus, &it.ChildRequestID, &result, &it.Error, &it.DecidedBy, &it.DecisionNote, &it.DecidedAt, &it.CreatedAt)
	it.Payload, it.Result = json.RawMessage(payload), json.RawMessage(result)
	return it, err
}

func itemsTx(ctx context.Context, q rowsQueryer, requestID string) ([]domain.RequestItem, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+itemCols+itemFrom+` WHERE i.request_id = ? ORDER BY i.round, i.position`, requestID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.RequestItem{}
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func itemTx(ctx context.Context, q queryer, id string) (domain.RequestItem, error) {
	it, err := scanItem(q.QueryRowContext(ctx, `SELECT `+itemCols+itemFrom+` WHERE i.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return it, fmt.Errorf("request item %q: %w", id, ErrNotInProject)
	}
	return it, err
}

// fill adds a request's items and its children (with theirs).
func (db *DB) fill(ctx context.Context, r *domain.Request) error {
	var err error
	if r.Items, err = itemsTx(ctx, db.sql, r.ID); err != nil {
		return err
	}
	kids, err := db.queryRequests(ctx, `r.parent_id = ? ORDER BY r.created_at, r.rowid`, r.ID)
	if err != nil {
		return err
	}
	for i := range kids {
		if kids[i].Items, err = itemsTx(ctx, db.sql, kids[i].ID); err != nil {
			return err
		}
	}
	r.Children = kids
	return nil
}

func (db *DB) queryRequests(ctx context.Context, where string, args ...any) ([]domain.Request, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+requestCols+requestFrom+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Request{}
	for rows.Next() {
		r, err := scanRequest(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Request returns one request of a service ("" for the company) with its
// items and the requests it was passed on as.
func (db *DB) Request(ctx context.Context, projectID, id string) (domain.Request, error) {
	r, err := scopedRequestTx(ctx, db.sql, projectID, id)
	if err != nil {
		return r, err
	}
	return r, db.fill(ctx, &r)
}

// RequestFilter narrows the request list.
type RequestFilter struct {
	// ProjectID lists one service's requests; "" lists company requests.
	ProjectID string `json:"projectId"`
	// All lists every request of the company and its services.
	All      bool     `json:"all"`
	Statuses []string `json:"statuses"`
	Limit    int      `json:"limit"`
}

// Requests lists requests, newest first, with their items.
func (db *DB) Requests(ctx context.Context, f RequestFilter) ([]domain.Request, error) {
	var where []string
	var args []any
	switch {
	case f.All:
	case f.ProjectID == "":
		where = append(where, `r.project_id IS NULL`)
	default:
		if err := db.CheckScope(ctx, f.ProjectID); err != nil {
			return nil, err
		}
		where = append(where, `r.project_id = ?`)
		args = append(args, f.ProjectID)
	}
	if len(f.Statuses) > 0 {
		where = append(where, `r.status IN (?`+strings.Repeat(`, ?`, len(f.Statuses)-1)+`)`)
		for _, s := range f.Statuses {
			args = append(args, s)
		}
	}
	if len(where) == 0 {
		where = append(where, `1 = 1`)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	list, err := db.queryRequests(ctx, strings.Join(where, ` AND `)+` ORDER BY r.created_at DESC, r.rowid DESC LIMIT ?`, append(args, f.Limit)...)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if err := db.fill(ctx, &list[i]); err != nil {
			return nil, err
		}
	}
	return list, nil
}

// SubmitRequest stores a person's request to the company (ProjectID "")
// or a service. PreviousID continues an earlier request of the same place.
func (db *DB) SubmitRequest(ctx context.Context, r domain.Request) (domain.Request, error) {
	r.Body = strings.TrimSpace(r.Body)
	if r.Body == "" {
		return r, invalid("요청 내용을 적어 주세요")
	}
	if len([]rune(r.Body)) > maxRequestBody {
		return r, invalid("요청은 %d자까지 쓸 수 있습니다", maxRequestBody)
	}
	r.ID = NewID("req")
	err := db.requestChange(ctx, func(c *Change) error {
		if r.ProjectID != "" {
			if err := writableProjectTx(ctx, c.Tx, r.ProjectID); err != nil {
				return err
			}
		}
		if r.PreviousID != "" {
			if _, err := scopedRequestTx(ctx, c.Tx, r.ProjectID, r.PreviousID); err != nil {
				return err
			}
		}
		now := Now()
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO requests (id, organization_id, project_id, previous_id, body, status, created_at, updated_at)
			VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, 'received', ?, ?)`,
			r.ID, domain.DefaultOrganizationID, r.ProjectID, r.PreviousID, r.Body, now, now); err != nil {
			return err
		}
		return emitRequestTx(c, r.ProjectID, r.ID, domain.ReqReceived)
	})
	if err != nil {
		return r, err
	}
	return db.Request(ctx, r.ProjectID, r.ID)
}

func emitRequestTx(c *Change, projectID, id, status string) error {
	if projectID == "" {
		return nil
	}
	return c.Emit(projectID, "", "", EventRequestUpdated, map[string]string{"requestId": id, "status": status})
}

func emitRequestEventTx(c *Change, projectID, id, status string) (int64, error) {
	rec, err := c.EmitEvent(projectID, "", "", EventRequestUpdated, map[string]string{"requestId": id, "status": status})
	return rec.Sequence, err
}

// ---- the intake runner's side ----

// RequestQueue lists requests the runner may take or check again: new,
// waiting for a desk, or held; oldest first.
func (db *DB) RequestQueue(ctx context.Context, limit int) ([]domain.Request, error) {
	return db.queryRequests(ctx, `r.status IN ('received', 'waiting_desk', 'held') ORDER BY r.created_at, r.rowid LIMIT ?`, limit)
}

// WaitRequest parks a queued request as waiting for a desk or held, with
// the reason. Nothing is written when it already says so, or when the
// request was claimed or changed meanwhile.
func (db *DB) WaitRequest(ctx context.Context, id, status, reason string) error {
	if status != domain.ReqWaitingDesk && status != domain.ReqHeld {
		return invalid("요청을 기다리게 할 수 없는 상태입니다: %s", status)
	}
	var changed bool
	_, err := db.Change(ctx, func(c *Change) error {
		r, err := requestTx(ctx, c.Tx, id)
		if err != nil {
			return err
		}
		if r.Status == status && r.HoldReason == reason {
			return nil
		}
		res, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = ?, hold_reason = ?, updated_at = ?
			WHERE id = ? AND status IN ('received', 'waiting_desk', 'held')`, status, reason, Now(), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if changed = n == 1; !changed {
			return nil
		}
		return emitRequestTx(c, r.ProjectID, id, status)
	})
	if err == nil && changed {
		db.requestChanged()
	}
	return err
}

// ParkRequest puts a triaging request back to waiting for a desk or held,
// with the reason (its desk could not be set up, or went away meanwhile).
func (db *DB) ParkRequest(ctx context.Context, id, status, reason string) error {
	if status != domain.ReqWaitingDesk && status != domain.ReqHeld {
		return invalid("요청을 기다리게 할 수 없는 상태입니다: %s", status)
	}
	return db.requestChange(ctx, func(c *Change) error {
		r, err := requestTx(ctx, c.Tx, id)
		if err != nil || r.Status != domain.ReqTriaging {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = ?, hold_reason = ?, updated_at = ? WHERE id = ? AND status = 'triaging'`,
			status, reason, Now(), id); err != nil {
			return err
		}
		return emitRequestTx(c, r.ProjectID, id, status)
	})
}

// ClaimRequest moves a queued request to triaging, reporting false when
// it was taken or changed meanwhile.
func (db *DB) ClaimRequest(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := db.requestChange(ctx, func(c *Change) error {
		r, err := requestTx(ctx, c.Tx, id)
		if err != nil {
			return err
		}
		res, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = 'triaging', hold_reason = '', updated_at = ?
			WHERE id = ? AND status IN ('received', 'waiting_desk', 'held')`, Now(), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if ok = n == 1; !ok {
			return nil
		}
		return emitRequestTx(c, r.ProjectID, id, domain.ReqTriaging)
	})
	return ok, err
}

// StartRequestAttempt records the call about to be made for a triaging
// request, before it is made, so a crash still counts it.
func (db *DB) StartRequestAttempt(ctx context.Context, r domain.Request, connectionID, model string) (string, error) {
	id := NewID("rqa")
	err := db.Write(ctx, func(tx *sql.Tx) error {
		var n int
		tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_attempts WHERE request_id = ? AND round = ?`, r.ID, r.Round).Scan(&n)
		_, err := tx.ExecContext(ctx, `INSERT INTO request_attempts (id, request_id, project_id, organization_id, round, number, connection_id, model, status, started_at)
			VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, NULLIF(?, ''), ?, 'working', ?)`,
			id, r.ID, r.ProjectID, r.OrganizationID, r.Round, n+1, connectionID, model, Now())
		return err
	})
	return id, err
}

// RecordRequestUsage keeps the latest running total an attempt reported.
func (db *DB) RecordRequestUsage(ctx context.Context, attemptID string, usage json.RawMessage) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE request_attempts SET usage = ?, usage_reported = 1 WHERE id = ?`, string(usage), attemptID)
		return err
	})
}

// FinishRequestAttempt ends a working attempt, keeping what came back.
func (db *DB) FinishRequestAttempt(ctx context.Context, attemptID, status string, result []byte, errText string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE request_attempts SET status = ?, result = ?, error = ?, ended_at = ? WHERE id = ? AND status = 'working'`,
			status, string(result), errText, Now(), attemptID)
		return err
	})
}

// ReleaseRequest puts a triaging request back in the queue for an
// automatic retry, or fails it when it used every attempt of the round.
func (db *DB) ReleaseRequest(ctx context.Context, id, errText string) error {
	return db.requestChange(ctx, func(c *Change) error {
		r, err := requestTx(ctx, c.Tx, id)
		if err != nil {
			return err
		}
		if r.Status != domain.ReqTriaging {
			return nil
		}
		status, closed := domain.ReqReceived, any(nil)
		if r.Attempts >= r.MaxAttempts {
			status, closed = domain.ReqFailed, Now()
			if errText == "" {
				errText = "알 수 없는 오류"
			}
		} else {
			errText = ""
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = ?, error = ?, closed_at = ?, updated_at = ? WHERE id = ?`,
			status, errText, closed, Now(), id); err != nil {
			return err
		}
		if status == domain.ReqFailed {
			if err := settleParentTx(ctx, c, r); err != nil {
				return err
			}
		}
		return emitRequestTx(c, r.ProjectID, id, status)
	})
}

// FailRequest ends a triaging request that cannot be decided at all.
func (db *DB) FailRequest(ctx context.Context, id, errText string) error {
	return db.requestChange(ctx, func(c *Change) error {
		r, err := requestTx(ctx, c.Tx, id)
		if err != nil || r.Status != domain.ReqTriaging {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = 'failed', error = ?, closed_at = ?, updated_at = ? WHERE id = ?`,
			errText, Now(), Now(), id); err != nil {
			return err
		}
		if err := settleParentTx(ctx, c, r); err != nil {
			return err
		}
		return emitRequestTx(c, r.ProjectID, id, domain.ReqFailed)
	})
}

// ApplyIntakeDecision stores a desk's checked decision for a triaging
// request in one transaction: the answer or question, the routes as new
// service requests, and the work and actions as items. In auto mode
// (autoWork) proposed work is registered at once; actions always wait
// for the person.
func (db *DB) ApplyIntakeDecision(ctx context.Context, id string, d domain.IntakeDecision, autoWork bool) error {
	return db.requestChange(ctx, func(c *Change) error {
		r, err := requestTx(ctx, c.Tx, id)
		if err != nil {
			return err
		}
		if r.Status != domain.ReqTriaging {
			return invalid("접수 판단 중인 요청이 아닙니다")
		}
		now := Now()
		set := func(status, answer, question, reason string) error {
			var closed any
			if domain.RequestClosed(status) {
				closed = now
			}
			_, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = ?, outcome = ?, answer = ?, question = ?, reason = ?, error = '',
				closed_at = ?, updated_at = ? WHERE id = ?`, status, d.Outcome, answer, question, reason, closed, now, id)
			return err
		}
		insertItem := func(pos int, kind string, payload any, status, workID, childID, decidedBy string) (string, error) {
			raw, _ := json.Marshal(payload)
			itemID := NewID("rqi")
			var decided any
			if decidedBy != "" {
				decided = now
			}
			_, err := c.Tx.ExecContext(ctx, `INSERT INTO request_items (id, request_id, project_id, round, position, kind, payload, status,
				work_item_id, child_request_id, decided_by, decided_at, created_at, updated_at)
				VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?, ?)`,
				itemID, id, r.ProjectID, r.Round, pos, kind, string(raw), status, workID, childID, decidedBy, decided, now, now)
			return itemID, err
		}
		switch d.Outcome {
		case domain.OutcomeAnswer:
			if err := set(domain.ReqDone, d.Answer, "", d.Reason); err != nil {
				return err
			}
			if err := settleParentTx(ctx, c, r); err != nil {
				return err
			}
		case domain.OutcomeQuestion:
			status := domain.ReqQuestion
			if r.Round >= 2 {
				status = domain.ReqManual
			}
			if err := set(status, "", d.Question, d.Reason); err != nil {
				return err
			}
		case domain.OutcomeDecline:
			if strings.TrimSpace(d.Reason) == "" {
				d.Reason = "접수 담당이 처리할 수 없다고 판단했습니다"
			}
			if err := set(domain.ReqDeclined, d.Answer, "", d.Reason); err != nil {
				return err
			}
			if err := settleParentTx(ctx, c, r); err != nil {
				return err
			}
		case domain.OutcomeRoute:
			if r.ProjectID != "" {
				return invalid("서비스 접수는 다른 서비스로 넘길 수 없습니다")
			}
			if err := set(domain.ReqRouted, "", "", d.Reason); err != nil {
				return err
			}
			for i, rt := range d.Routes {
				if err := writableProjectTx(ctx, c.Tx, rt.ProjectID); err != nil {
					return err
				}
				childID, itemID := NewID("req"), ""
				body := strings.TrimSpace(rt.Body)
				if body == "" {
					body = r.Body
				}
				// The item names its child and the child its item: the
				// child goes first with the item id it will get.
				raw, _ := json.Marshal(rt)
				itemID = NewID("rqi")
				if _, err := c.Tx.ExecContext(ctx, `INSERT INTO requests (id, organization_id, project_id, parent_id, parent_item_id, body, status, created_at, updated_at)
					VALUES (?, ?, ?, ?, ?, ?, 'received', ?, ?)`, childID, r.OrganizationID, rt.ProjectID, id, itemID, body, now, now); err != nil {
					return err
				}
				if _, err := c.Tx.ExecContext(ctx, `INSERT INTO request_items (id, request_id, round, position, kind, payload, status,
					child_request_id, decided_by, decided_at, created_at, updated_at) VALUES (?, ?, ?, ?, 'route', ?, 'done', ?, 'auto', ?, ?, ?)`,
					itemID, id, r.Round, i+1, string(raw), childID, now, now, now); err != nil {
					return err
				}
				if err := emitRequestTx(c, rt.ProjectID, childID, domain.ReqReceived); err != nil {
					return err
				}
			}
			return nil
		case domain.OutcomeWork, domain.OutcomeAction:
			if r.ProjectID == "" {
				return invalid("회사 접수는 업무나 조치를 바로 정하지 않고 서비스로 넘깁니다")
			}
			if err := set(domain.ReqProposed, d.Answer, "", d.Reason); err != nil {
				return err
			}
			seq, err := emitRequestEventTx(c, r.ProjectID, id, domain.ReqProposed)
			if err != nil {
				return err
			}
			pos := 0
			for _, a := range d.Actions {
				pos++
				if _, err := insertItem(pos, domain.ItemAction, a, domain.ItemProposed, "", "", ""); err != nil {
					return err
				}
			}
			for _, w := range d.Work {
				pos++
				if !autoWork {
					if _, err := insertItem(pos, domain.ItemWork, w, domain.ItemProposed, "", "", ""); err != nil {
						return err
					}
					continue
				}
				workID, err := insertRequestWorkTx(ctx, c, r, w, workOrigin{Seq: seq, Automated: true, Request: id})
				if err != nil {
					return err
				}
				if _, err := insertItem(pos, domain.ItemWork, w, domain.ItemAccepted, workID, "", "auto"); err != nil {
					return err
				}
			}
			return refreshRequestTx(ctx, c, id)
		default:
			return invalid("알 수 없는 접수 판단입니다: %s", d.Outcome)
		}
		after, err := requestTx(ctx, c.Tx, id)
		if err != nil {
			return err
		}
		return emitRequestTx(c, r.ProjectID, id, after.Status)
	})
}

// insertRequestWorkTx registers proposed work in the request's service.
func insertRequestWorkTx(ctx context.Context, c *Change, r domain.Request, p domain.WorkProposalPayload, origin workOrigin) (string, error) {
	w := domain.WorkItem{ProjectID: r.ProjectID, Kind: domain.WorkTask, ParentID: p.GoalID, DepartmentID: p.DepartmentID,
		AssigneeAssignmentID: p.AssigneeAssignmentID, Title: p.Title, Description: p.Description, DoneCriteria: p.DoneCriteria,
		Categories: p.Categories, Priority: p.Priority, Status: domain.WorkReceived}
	if err := normalizeWork(&w); err != nil {
		return "", err
	}
	if err := checkWorkLinks(ctx, c.Tx, w, true); err != nil {
		return "", err
	}
	return insertWorkTx(ctx, c, w, origin)
}

// refreshRequestTx works out a request's status from its items: proposed
// while the person has something to decide (a proposal, a failed action,
// an action to check), in progress while an action runs, done after.
func refreshRequestTx(ctx context.Context, c *Change, id string) error {
	r, err := requestTx(ctx, c.Tx, id)
	if err != nil {
		return err
	}
	if !slices.Contains([]string{domain.ReqProposed, domain.ReqInProgress, domain.ReqRouted}, r.Status) {
		return nil
	}
	status := domain.ReqDone
	if r.Status == domain.ReqRouted {
		var open int
		if err := c.Tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM requests WHERE parent_id = ? AND status NOT IN ('done', 'declined', 'cancelled', 'failed')`, id).Scan(&open); err != nil {
			return err
		}
		if open > 0 {
			return nil
		}
	} else {
		items, err := itemsTx(ctx, c.Tx, id)
		if err != nil {
			return err
		}
		for _, it := range items {
			switch it.Status {
			case domain.ItemProposed, domain.ItemFailed, domain.ItemCheck:
				status = domain.ReqProposed
			case domain.ItemRunning:
				if status == domain.ReqDone {
					status = domain.ReqInProgress
				}
			}
		}
	}
	if status == r.Status {
		return nil
	}
	var closed any
	if status == domain.ReqDone {
		closed = Now()
	}
	if _, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = ?, closed_at = ?, updated_at = ? WHERE id = ?`, status, closed, Now(), id); err != nil {
		return err
	}
	if err := emitRequestTx(c, r.ProjectID, id, status); err != nil {
		return err
	}
	if status == domain.ReqDone {
		return settleParentTx(ctx, c, r)
	}
	return nil
}

// settleParentTx lets a routed company request finish once every service
// request it was passed on as is over.
func settleParentTx(ctx context.Context, c *Change, r domain.Request) error {
	if r.ParentID == "" {
		return nil
	}
	return refreshRequestTx(ctx, c, r.ParentID)
}

// ---- the person's side ----

// AnswerRequest gives the person's answer to the desk's question; the
// desk decides once more (round 2).
func (db *DB) AnswerRequest(ctx context.Context, projectID, id, reply string) error {
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return invalid("답을 적어 주세요")
	}
	return db.requestChange(ctx, func(c *Change) error {
		r, err := scopedRequestTx(ctx, c.Tx, projectID, id)
		if err != nil {
			return err
		}
		if r.Status != domain.ReqQuestion || r.Round != 1 {
			return invalid("접수 담당이 묻고 있는 요청이 아닙니다")
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = 'received', round = 2, reply = ?, updated_at = ? WHERE id = ?`,
			reply, Now(), id); err != nil {
			return err
		}
		return emitRequestTx(c, r.ProjectID, id, domain.ReqReceived)
	})
}

// CloseRequest ends a request the person handled themselves (the desk
// asked back twice) or no longer needs.
func (db *DB) CloseRequest(ctx context.Context, projectID, id, note string) error {
	return db.requestChange(ctx, func(c *Change) error {
		r, err := scopedRequestTx(ctx, c.Tx, projectID, id)
		if err != nil {
			return err
		}
		if r.Status != domain.ReqManual && r.Status != domain.ReqQuestion {
			return invalid("사람이 직접 처리할 요청만 닫을 수 있습니다")
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = 'done', reason = CASE WHEN ? <> '' THEN ? ELSE reason END,
			closed_at = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(note), strings.TrimSpace(note), Now(), Now(), id); err != nil {
			return err
		}
		if err := settleParentTx(ctx, c, r); err != nil {
			return err
		}
		return emitRequestTx(c, r.ProjectID, id, domain.ReqDone)
	})
}

// CancelRequest stops a request: undecided items are rejected and open
// service requests it was passed on as are cancelled too. Registered work
// and actions already carried out stay as they are.
func (db *DB) CancelRequest(ctx context.Context, projectID, id string) error {
	return db.requestChange(ctx, func(c *Change) error {
		r, err := scopedRequestTx(ctx, c.Tx, projectID, id)
		if err != nil {
			return err
		}
		if domain.RequestClosed(r.Status) {
			return invalid("이미 끝난 요청입니다")
		}
		if r.Status == domain.ReqTriaging {
			return invalid("접수 담당이 판단하는 중입니다. 끝난 뒤에 취소하세요")
		}
		return cancelRequestTx(ctx, c, r)
	})
}

func cancelRequestTx(ctx context.Context, c *Change, r domain.Request) error {
	now := Now()
	if _, err := c.Tx.ExecContext(ctx, `UPDATE request_items SET status = 'rejected', decided_by = 'user', decision_note = '요청 취소',
		decided_at = ?, updated_at = ? WHERE request_id = ? AND status IN ('proposed', 'failed', 'check')`, now, now, r.ID); err != nil {
		return err
	}
	if _, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = 'cancelled', closed_at = ?, updated_at = ? WHERE id = ?`, now, now, r.ID); err != nil {
		return err
	}
	if err := emitRequestTx(c, r.ProjectID, r.ID, domain.ReqCancelled); err != nil {
		return err
	}
	rows, err := c.Tx.QueryContext(ctx, `SELECT id FROM requests WHERE parent_id = ? AND status NOT IN ('done', 'declined', 'cancelled', 'failed', 'triaging')`, r.ID)
	if err != nil {
		return err
	}
	var kids []string
	for rows.Next() {
		var k string
		rows.Scan(&k)
		kids = append(kids, k)
	}
	rows.Close()
	for _, k := range kids {
		kr, err := requestTx(ctx, c.Tx, k)
		if err != nil {
			return err
		}
		if err := cancelRequestTx(ctx, c, kr); err != nil {
			return err
		}
	}
	return settleParentTx(ctx, c, r)
}

// RetryRequest queues a failed request again with one more attempt; the
// earlier attempts and their cost stay recorded.
func (db *DB) RetryRequest(ctx context.Context, projectID, id string) error {
	return db.requestChange(ctx, func(c *Change) error {
		r, err := scopedRequestTx(ctx, c.Tx, projectID, id)
		if err != nil {
			return err
		}
		if r.Status != domain.ReqFailed {
			return invalid("실패한 요청만 다시 시도할 수 있습니다")
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = 'received', error = '', closed_at = NULL,
			max_attempts = MAX(max_attempts, ?) + 1, updated_at = ? WHERE id = ?`, r.Attempts, Now(), id); err != nil {
			return err
		}
		return emitRequestTx(c, r.ProjectID, id, domain.ReqReceived)
	})
}

// scopedItemTx returns an item of a request in the given scope.
func scopedItemTx(ctx context.Context, q queryer, projectID, itemID string) (domain.RequestItem, domain.Request, error) {
	it, err := itemTx(ctx, q, itemID)
	if err != nil {
		return it, domain.Request{}, err
	}
	r, err := scopedRequestTx(ctx, q, projectID, it.RequestID)
	return it, r, err
}

// AcceptRequestWork registers proposed work, with the person's edits.
func (db *DB) AcceptRequestWork(ctx context.Context, projectID, itemID string, edit domain.WorkProposalPayload) (domain.WorkItem, error) {
	var workID string
	err := db.requestChange(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		it, r, err := scopedItemTx(ctx, c.Tx, projectID, itemID)
		if err != nil {
			return err
		}
		if it.Kind != domain.ItemWork || it.Status != domain.ItemProposed {
			return invalid("이미 처리한 업무 제안입니다")
		}
		var p domain.WorkProposalPayload
		json.Unmarshal(it.Payload, &p)
		if t := strings.TrimSpace(edit.Title); t != "" {
			p.Title = t
		}
		if edit.Description != "" {
			p.Description = edit.Description
		}
		if edit.DoneCriteria != "" {
			p.DoneCriteria = edit.DoneCriteria
		}
		if edit.Priority != "" {
			p.Priority = edit.Priority
		}
		if edit.Categories != nil {
			p.Categories = edit.Categories
		}
		if edit.DepartmentID != "" {
			p.DepartmentID = edit.DepartmentID
		}
		if edit.AssigneeAssignmentID != "" {
			p.AssigneeAssignmentID = edit.AssigneeAssignmentID
		}
		seq, err := emitRequestEventTx(c, projectID, r.ID, r.Status)
		if err != nil {
			return err
		}
		if workID, err = insertRequestWorkTx(ctx, c, r, p, workOrigin{Seq: seq, Request: r.ID}); err != nil {
			return err
		}
		raw, _ := json.Marshal(p)
		res, err := c.Tx.ExecContext(ctx, `UPDATE request_items SET status = 'accepted', work_item_id = ?, payload = ?, decided_by = 'user',
			decided_at = ?, updated_at = ? WHERE id = ? AND status = 'proposed'`, workID, string(raw), Now(), Now(), itemID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return invalid("이미 처리한 업무 제안입니다")
		}
		return refreshRequestTx(ctx, c, r.ID)
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return db.WorkItem(ctx, projectID, workID)
}

// RejectRequestItem turns down a proposal, or an action that failed or
// needs checking.
func (db *DB) RejectRequestItem(ctx context.Context, projectID, itemID, note string) error {
	return db.requestChange(ctx, func(c *Change) error {
		it, r, err := scopedItemTx(ctx, c.Tx, projectID, itemID)
		if err != nil {
			return err
		}
		if !slices.Contains([]string{domain.ItemProposed, domain.ItemFailed, domain.ItemCheck}, it.Status) {
			return invalid("이미 처리한 항목입니다")
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE request_items SET status = 'rejected', decided_by = 'user', decision_note = ?, decided_at = ?,
			updated_at = ? WHERE id = ?`, strings.TrimSpace(note), Now(), Now(), itemID); err != nil {
			return err
		}
		return refreshRequestTx(ctx, c, r.ID)
	})
}

// StartRequestAction marks an approved action as running, once: a second
// approval of the same action is refused. A failed action, or one left
// to check after a stop, may run again.
func (db *DB) StartRequestAction(ctx context.Context, projectID, itemID, note string) (domain.RequestItem, error) {
	err := db.requestChange(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		it, r, err := scopedItemTx(ctx, c.Tx, projectID, itemID)
		if err != nil {
			return err
		}
		if it.Kind != domain.ItemAction {
			return invalid("조치 항목이 아닙니다")
		}
		res, err := c.Tx.ExecContext(ctx, `UPDATE request_items SET status = 'running', error = '', decided_by = 'user', decision_note = ?,
			decided_at = ?, updated_at = ? WHERE id = ? AND status IN ('proposed', 'failed', 'check')`, strings.TrimSpace(note), Now(), Now(), itemID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return invalid("이미 실행 중이거나 처리한 조치입니다")
		}
		return refreshRequestTx(ctx, c, r.ID)
	})
	if err != nil {
		return domain.RequestItem{}, err
	}
	return itemTx(ctx, db.sql, itemID)
}

// FinishRequestAction records what a running action did.
func (db *DB) FinishRequestAction(ctx context.Context, itemID string, result any, errText string) error {
	return db.requestChange(ctx, func(c *Change) error {
		it, err := itemTx(ctx, c.Tx, itemID)
		if err != nil {
			return err
		}
		status := domain.ItemDone
		if errText != "" {
			status = domain.ItemFailed
		}
		raw, _ := json.Marshal(result)
		if _, err := c.Tx.ExecContext(ctx, `UPDATE request_items SET status = ?, result = ?, error = ?, updated_at = ? WHERE id = ? AND status = 'running'`,
			status, string(raw), errText, Now(), itemID); err != nil {
			return err
		}
		return refreshRequestTx(ctx, c, it.RequestID)
	})
}

// ConfirmRequestAction closes an action the person checked after the app
// stopped while it ran.
func (db *DB) ConfirmRequestAction(ctx context.Context, projectID, itemID, note string) error {
	return db.requestChange(ctx, func(c *Change) error {
		it, r, err := scopedItemTx(ctx, c.Tx, projectID, itemID)
		if err != nil {
			return err
		}
		if it.Status != domain.ItemCheck {
			return invalid("확인할 조치가 아닙니다")
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE request_items SET status = 'done', decision_note = ?, updated_at = ? WHERE id = ?`,
			strings.TrimSpace(note), Now(), itemID); err != nil {
			return err
		}
		return refreshRequestTx(ctx, c, r.ID)
	})
}

// RecoverRequests runs at start: calls cut off by a stop are interrupted
// attempts and their requests go back to the queue (or fail when out of
// attempts); actions cut off are left for the person to check, never run
// again on their own.
func (db *DB) RecoverRequests(ctx context.Context) error {
	// At start no screen is open yet, so the hook is not told.
	_, err := db.Change(ctx, func(c *Change) error {
		now := Now()
		if _, err := c.Tx.ExecContext(ctx, `UPDATE request_attempts SET status = 'interrupted', error = '앱이 다시 시작되어 중단되었습니다', ended_at = ?
			WHERE status = 'working'`, now); err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE desk_staffing SET status = 'interrupted', error = '앱이 다시 시작되어 중단되었습니다', ended_at = ?
			WHERE status = 'working'`, now); err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE org_proposals SET status = 'interrupted', error = '앱이 다시 시작되어 중단되었습니다', ended_at = ?
			WHERE status = 'working'`, now); err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE requests SET status = CASE
				WHEN (SELECT COUNT(*) FROM request_attempts a WHERE a.request_id = requests.id AND a.round = requests.round) >= max_attempts THEN 'failed'
				ELSE 'received' END,
			error = CASE WHEN (SELECT COUNT(*) FROM request_attempts a WHERE a.request_id = requests.id AND a.round = requests.round) >= max_attempts
				THEN '앱이 다시 시작되어 중단되었고 시도 횟수를 모두 썼습니다' ELSE '' END,
			closed_at = CASE WHEN (SELECT COUNT(*) FROM request_attempts a WHERE a.request_id = requests.id AND a.round = requests.round) >= max_attempts
				THEN ? ELSE NULL END,
			updated_at = ? WHERE status = 'triaging'`, now, now); err != nil {
			return err
		}
		rows, err := c.Tx.QueryContext(ctx, `SELECT DISTINCT request_id FROM request_items WHERE status = 'running'`)
		if err != nil {
			return err
		}
		var ids []string
		for rows.Next() {
			var id string
			rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()
		if _, err := c.Tx.ExecContext(ctx, `UPDATE request_items SET status = 'check', updated_at = ? WHERE status = 'running'`, now); err != nil {
			return err
		}
		for _, id := range ids {
			if err := refreshRequestTx(ctx, c, id); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// RequestTodos lists requests that wait on the person: a desk's question,
// one handed back, proposals and actions to decide, failures, holds and
// missing desks.
func (db *DB) RequestTodos(ctx context.Context) ([]domain.Request, error) {
	return db.Requests(ctx, RequestFilter{All: true, Limit: 100, Statuses: []string{
		domain.ReqQuestion, domain.ReqManual, domain.ReqProposed, domain.ReqFailed, domain.ReqHeld, domain.ReqWaitingDesk}})
}

// CompanyIntakeUsage sums the company desk's calls since a time (the
// start of the month for its budget).
func (db *DB) CompanyIntakeUsage(ctx context.Context, since time.Time) (AIAttemptUsage, error) {
	t := since.UTC().Format(time.RFC3339Nano)
	return sumUsageRows(ctx, db.sql, `SELECT usage, usage_reported FROM request_attempts WHERE project_id IS NULL AND started_at >= ?
		UNION ALL SELECT usage, usage_reported FROM desk_staffing WHERE project_id IS NULL AND started_at >= ?`, t, t)
}
