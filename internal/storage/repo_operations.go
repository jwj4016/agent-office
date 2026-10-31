package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"agent-office/internal/domain"
)

// Reading how a small company runs (plan stage 9): why a work item
// started, what came of it, how loaded each department is and what waits
// for the person. Everything here is counted from stored records; nothing
// is estimated.

// WorkCause is why a work item exists: a department's proposal from an
// event, or a collaboration request it was taken on for.
type WorkCause struct {
	Via                string `json:"via"` // proposal | collab | request
	ProposalID         string `json:"proposalId,omitempty"`
	CollabRequestID    string `json:"collabRequestId,omitempty"`
	EventSequence      int64  `json:"eventSequence"`
	EventKind          string `json:"eventKind"`
	EventTitle         string `json:"eventTitle"`
	EventNote          string `json:"eventNote"`
	ScheduleSlot       string `json:"scheduleSlot,omitempty"`
	FromWorkItemID     string `json:"fromWorkItemId,omitempty"`
	FromWorkTitle      string `json:"fromWorkTitle,omitempty"`
	FromDepartmentName string `json:"fromDepartmentName,omitempty"`
	DocumentID         string `json:"documentId,omitempty"`
	DocumentTitle      string `json:"documentTitle,omitempty"`
	DepartmentName     string `json:"departmentName"` // the department that judged, or was asked
	Reason             string `json:"reason"`         // the judgment's reason, or the request
	DecidedBy          string `json:"decidedBy"`      // user | ai
	// RequestID and RequestBody name the person's request (via request).
	RequestID   string `json:"requestId,omitempty"`
	RequestBody string `json:"requestBody,omitempty"`
}

// LinkedWork is a work item that came from another.
type LinkedWork struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	Status         string `json:"status"`
	DepartmentName string `json:"departmentName"`
	Via            string `json:"via"` // proposal | collab
}

// WorkLineage is where a work item came from and what it led to.
type WorkLineage struct {
	Causes []WorkCause  `json:"causes"`
	Next   []LinkedWork `json:"next"`
}

// WorkLineage reads a work item's causes (none for work a person made
// directly) and the work that came from its events, its shared documents
// and its collaboration requests.
func (db *DB) WorkLineage(ctx context.Context, projectID, workID string) (WorkLineage, error) {
	out := WorkLineage{Causes: []WorkCause{}, Next: []LinkedWork{}}
	w, err := workTx(ctx, db.sql, projectID, workID)
	if err != nil {
		return out, err
	}
	if w.OriginRequestID != "" {
		if r, err := requestTx(ctx, db.sql, w.OriginRequestID); err == nil {
			var by string
			db.sql.QueryRowContext(ctx, `SELECT decided_by FROM request_items WHERE work_item_id = ?`, workID).Scan(&by)
			if by == "auto" {
				by = "ai"
			}
			out.Causes = append(out.Causes, WorkCause{Via: "request", RequestID: r.ID, RequestBody: r.Body, Reason: r.Reason, DecidedBy: by})
		}
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT w.id, j.reason, e.sequence, e.kind, e.payload, d.name
		FROM work_proposals w JOIN judgments j ON j.id = w.judgment_id JOIN event_deliveries dl ON dl.id = j.delivery_id
		JOIN execution_events e ON e.sequence = dl.event_sequence JOIN departments d ON d.id = w.department_id
		WHERE w.work_item_id = ? AND w.project_id = ?`, workID, projectID)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var c WorkCause
		var payload string
		if err := rows.Scan(&c.ProposalID, &c.Reason, &c.EventSequence, &c.EventKind, &payload, &c.DepartmentName); err != nil {
			rows.Close()
			return out, err
		}
		var ev WorkEvent
		json.Unmarshal([]byte(payload), &ev)
		c.Via, c.DecidedBy = "proposal", "user"
		c.EventTitle, c.EventNote, c.ScheduleSlot, c.FromWorkItemID, c.DocumentID = ev.Title, ev.Note, ev.ScheduleSlot, ev.WorkItemID, ev.DocumentID
		out.Causes = append(out.Causes, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return out, err
	}
	for i := range out.Causes {
		c := &out.Causes[i]
		if c.FromWorkItemID != "" {
			db.sql.QueryRowContext(ctx, `SELECT title FROM work_items WHERE id = ?`, c.FromWorkItemID).Scan(&c.FromWorkTitle)
		}
		if c.DocumentID != "" {
			db.sql.QueryRowContext(ctx, `SELECT title FROM shared_documents WHERE id = ?`, c.DocumentID).Scan(&c.DocumentTitle)
		}
	}
	asked, err := queryCollabs(ctx, db.sql, `r.project_id = ? AND r.result_work_item_id = ?`, projectID, workID)
	if err != nil {
		return out, err
	}
	for _, r := range asked {
		var kind string
		db.sql.QueryRowContext(ctx, `SELECT kind FROM execution_events WHERE sequence = ?`, r.EventSequence).Scan(&kind)
		out.Causes = append(out.Causes, WorkCause{Via: "collab", CollabRequestID: r.ID, EventSequence: r.EventSequence, EventKind: kind,
			EventTitle: r.FromWorkTitle, EventNote: r.Question, FromWorkItemID: r.FromWorkItemID, FromWorkTitle: r.FromWorkTitle,
			FromDepartmentName: r.FromDepartmentName, DepartmentName: r.ToDepartmentName, Reason: r.Question, DecidedBy: r.DecidedBy})
	}

	sent, err := queryCollabs(ctx, db.sql, `r.project_id = ? AND r.from_work_item_id = ? AND r.result_work_item_id IS NOT NULL`, projectID, workID)
	if err != nil {
		return out, err
	}
	for _, r := range sent {
		out.Next = append(out.Next, LinkedWork{ID: r.ResultWorkItemID, Title: r.ResultWorkTitle, Status: r.ResultWorkStatus,
			DepartmentName: r.ToDepartmentName, Via: "collab"})
	}
	rows, err = db.sql.QueryContext(ctx, `SELECT DISTINCT t.id, t.title, t.status, COALESCE(d.name, '')
		FROM work_proposals w JOIN judgments j ON j.id = w.judgment_id JOIN event_deliveries dl ON dl.id = j.delivery_id
		JOIN execution_events e ON e.sequence = dl.event_sequence JOIN work_items t ON t.id = w.work_item_id
		LEFT JOIN departments d ON d.id = t.department_id
		WHERE w.project_id = ? AND (json_extract(e.payload, '$.workItemId') = ?
			OR json_extract(e.payload, '$.documentId') IN (SELECT id FROM shared_documents WHERE work_item_id = ?))
		ORDER BY t.created_at`, projectID, workID, workID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		l := LinkedWork{Via: "proposal"}
		if err := rows.Scan(&l.ID, &l.Title, &l.Status, &l.DepartmentName); err != nil {
			return out, err
		}
		if l.ID != workID {
			out.Next = append(out.Next, l)
		}
	}
	return out, rows.Err()
}

// WorkRef is a work item named on an overview.
type WorkRef struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Status string `json:"status"`
	Reason string `json:"reason"`
}

// DepartmentLoad is one department's open work and what waits on it.
// DepartmentID "" holds work no department has.
type DepartmentLoad struct {
	DepartmentID     string    `json:"departmentId"`
	Name             string    `json:"name"`
	Status           string    `json:"status"`
	Open             int       `json:"open"` // not done or cancelled, including the two below
	Waiting          int       `json:"waiting"`
	Blocked          int       `json:"blocked"`
	ResultsToCheck   int       `json:"resultsToCheck"`
	Proposals        int       `json:"proposals"`
	Questions        int       `json:"questions"`
	CollabIncoming   int       `json:"collabIncoming"`
	AutoReviews      int       `json:"autoReviews"`
	FailedDeliveries int       `json:"failedDeliveries"`
	HeldDeliveries   int       `json:"heldDeliveries"`
	Attention        []WorkRef `json:"attention"` // blocked, then waiting, at most five
}

// resultReady is true for a waiting work item whose latest run succeeded:
// a person has its result to check.
const resultReady = `w.status = 'waiting' AND (SELECT r.status FROM runs r WHERE r.work_item_id = w.id
	ORDER BY r.started_at DESC, r.rowid DESC LIMIT 1) = 'succeeded'`

// DepartmentLoads counts each department's open work, waiting decisions
// and deliveries in trouble.
func (db *DB) DepartmentLoads(ctx context.Context, projectID string) ([]DepartmentLoad, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT d.id, d.name, d.status,
		(SELECT COUNT(*) FROM work_items w WHERE w.department_id = d.id AND w.kind = 'work' AND w.status NOT IN ('done', 'cancelled')),
		(SELECT COUNT(*) FROM work_items w WHERE w.department_id = d.id AND w.kind = 'work' AND w.status = 'waiting'),
		(SELECT COUNT(*) FROM work_items w WHERE w.department_id = d.id AND w.kind = 'work' AND w.status = 'blocked'),
		(SELECT COUNT(*) FROM work_items w WHERE w.department_id = d.id AND w.kind = 'work' AND `+resultReady+`),
		(SELECT COUNT(*) FROM work_proposals p WHERE p.department_id = d.id AND p.status = 'proposed'),
		(SELECT COUNT(*) FROM judgments j WHERE j.department_id = d.id AND j.question_status IN ('open', 'manual')),
		(SELECT COUNT(*) FROM collab_requests c WHERE c.to_department_id = d.id AND c.status IN ('requested', 'question')),
		(SELECT COUNT(*) FROM auto_starts a JOIN work_items w ON w.id = a.work_item_id WHERE w.department_id = d.id AND a.status = 'review'),
		(SELECT COUNT(*) FROM event_deliveries e WHERE e.department_id = d.id AND e.status = 'failed'),
		(SELECT COUNT(*) FROM event_deliveries e WHERE e.department_id = d.id AND e.status = 'held')
		FROM departments d WHERE d.project_id = ? ORDER BY d.status, d.name`, projectID)
	if err != nil {
		return nil, err
	}
	out := []DepartmentLoad{}
	for rows.Next() {
		var l DepartmentLoad
		if err := rows.Scan(&l.DepartmentID, &l.Name, &l.Status, &l.Open, &l.Waiting, &l.Blocked, &l.ResultsToCheck, &l.Proposals,
			&l.Questions, &l.CollabIncoming, &l.AutoReviews, &l.FailedDeliveries, &l.HeldDeliveries); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	none := DepartmentLoad{Name: "부서 없음", Status: domain.StatusActive}
	err = db.sql.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(w.status = 'waiting'), 0), COALESCE(SUM(w.status = 'blocked'), 0),
		COALESCE(SUM(`+resultReady+`), 0)
		FROM work_items w WHERE w.project_id = ? AND w.department_id IS NULL AND w.kind = 'work' AND w.status NOT IN ('done', 'cancelled')`,
		projectID).Scan(&none.Open, &none.Waiting, &none.Blocked, &none.ResultsToCheck)
	if err != nil {
		return nil, err
	}
	if none.Open > 0 {
		out = append(out, none)
	}
	for i := range out {
		l := &out[i]
		if l.Attention, err = attentionWork(ctx, db.sql, projectID, l.DepartmentID); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func attentionWork(ctx context.Context, q *sql.DB, projectID, departmentID string) ([]WorkRef, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, title, status, status_reason FROM work_items
		WHERE project_id = ? AND COALESCE(department_id, '') = ? AND kind = 'work' AND status IN ('blocked', 'waiting')
		ORDER BY status = 'waiting', updated_at DESC LIMIT 5`, projectID, departmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []WorkRef{}
	for rows.Next() {
		var w WorkRef
		if err := rows.Scan(&w.ID, &w.Title, &w.Status, &w.Reason); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

// DeliveryTrouble is a delivery that failed or is held, for the overview.
type DeliveryTrouble struct {
	ID             string `json:"id"`
	DepartmentName string `json:"departmentName"`
	Status         string `json:"status"` // failed | held
	Reason         string `json:"reason"`
	EventTitle     string `json:"eventTitle"`
	UpdatedAt      string `json:"updatedAt"`
}

// DeliveryTroubles lists a service's failed and held deliveries, newest
// first, with how many there are in all.
func (db *DB) DeliveryTroubles(ctx context.Context, projectID string, limit int) ([]DeliveryTrouble, int, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, 0, err
	}
	var total int
	if err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM event_deliveries WHERE project_id = ? AND status IN ('failed', 'held')`,
		projectID).Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT dl.id, d.name, dl.status, CASE dl.status WHEN 'failed' THEN dl.error ELSE dl.hold_reason END,
		e.payload, dl.updated_at FROM event_deliveries dl JOIN departments d ON d.id = dl.department_id
		JOIN execution_events e ON e.sequence = dl.event_sequence
		WHERE dl.project_id = ? AND dl.status IN ('failed', 'held') ORDER BY dl.updated_at DESC LIMIT ?`, projectID, limit)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []DeliveryTrouble{}
	for rows.Next() {
		var t DeliveryTrouble
		var payload string
		if err := rows.Scan(&t.ID, &t.DepartmentName, &t.Status, &t.Reason, &payload, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		var ev WorkEvent
		json.Unmarshal([]byte(payload), &ev)
		t.EventTitle = ev.Title
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// ResultReview is work whose run finished and waits for the person to
// check the result.
type ResultReview struct {
	WorkItemID     string `json:"workItemId"`
	ProjectID      string `json:"projectId"`
	ProjectName    string `json:"projectName"`
	Title          string `json:"title"`
	DepartmentName string `json:"departmentName"`
	GoalID         string `json:"goalId"`
	RunID          string `json:"runId"`
	FinishedAt     string `json:"finishedAt"`
}

// ResultsToCheck lists work with a result to check, of one service or
// ("") of every service in use, oldest first.
func (db *DB) ResultsToCheck(ctx context.Context, projectID string) ([]ResultReview, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT w.id, w.project_id, p.name, w.title, COALESCE(d.name, ''), COALESCE(w.parent_id, ''),
		r.id, COALESCE(r.ended_at, r.started_at)
		FROM work_items w JOIN projects p ON p.id = w.project_id LEFT JOIN departments d ON d.id = w.department_id
		JOIN runs r ON r.id = (SELECT x.id FROM runs x WHERE x.work_item_id = w.id ORDER BY x.started_at DESC, x.rowid DESC LIMIT 1)
		WHERE p.status <> 'archived' AND (? = '' OR w.project_id = ?) AND w.kind = 'work' AND `+resultReady+`
		ORDER BY COALESCE(r.ended_at, r.started_at)`, projectID, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ResultReview{}
	for rows.Next() {
		var r ResultReview
		if err := rows.Scan(&r.WorkItemID, &r.ProjectID, &r.ProjectName, &r.Title, &r.DepartmentName, &r.GoalID, &r.RunID, &r.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GoalRef names a shared goal.
type GoalRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// GoalsOf maps work items (of any service) to their shared goal: the goal
// itself or its parent. Work outside a goal is left out.
func (db *DB) GoalsOf(ctx context.Context, workIDs []string) (map[string]GoalRef, error) {
	out := map[string]GoalRef{}
	workIDs = dedupe(workIDs)
	if len(workIDs) == 0 {
		return out, nil
	}
	args := make([]any, len(workIDs))
	for i, id := range workIDs {
		args[i] = id
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT w.id, g.id, g.title FROM work_items w
		JOIN work_items g ON g.id = CASE WHEN w.kind = 'goal' THEN w.id ELSE w.parent_id END
		WHERE w.id IN (`+strings.TrimSuffix(strings.Repeat("?, ", len(args)), ", ")+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		var g GoalRef
		if err := rows.Scan(&id, &g.ID, &g.Title); err != nil {
			return nil, err
		}
		out[id] = g
	}
	return out, rows.Err()
}
