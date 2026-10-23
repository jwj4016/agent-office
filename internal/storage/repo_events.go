package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"agent-office/internal/domain"
)

// WorkEvent is the payload of every work, document and department event
// (plan §9.2). DepartmentID is the department the event belongs to (the
// work's or document's department); a department does not judge its own
// events. A chain starts at an event that is not automated: its
// ChainRootSequence stays empty and ChainDepth is 0.
type WorkEvent struct {
	DepartmentID       string   `json:"departmentId"`
	TargetDepartmentID string   `json:"targetDepartmentId,omitempty"`
	CollabRequestID    string   `json:"collabRequestId,omitempty"`
	WorkItemID         string   `json:"workItemId,omitempty"`
	GoalID             string   `json:"goalId,omitempty"`
	Title              string   `json:"title,omitempty"`
	Note               string   `json:"note,omitempty"`
	Categories         []string `json:"categories"`
	CauseSequence      *int64   `json:"causeSequence"`
	ChainRootSequence  *int64   `json:"chainRootSequence"`
	ChainDepth         int      `json:"chainDepth"`
	Automated          bool     `json:"automated"`
	DocumentID         string   `json:"documentId,omitempty"`
	DocumentVersionID  string   `json:"documentVersionId,omitempty"`
}

// ChainRoot is the sequence of the event that started this event's chain.
func (e WorkEvent) ChainRoot(sequence int64) int64 {
	if e.ChainRootSequence != nil {
		return *e.ChainRootSequence
	}
	return sequence
}

// emitWorkEventTx stores a work event and, in the same transaction, the
// deliveries it causes.
func emitWorkEventTx(ctx context.Context, c *Change, projectID, kind string, ev WorkEvent) (EventRecord, error) {
	if ev.Categories == nil {
		ev.Categories = []string{}
	}
	rec, err := c.EmitEvent(projectID, "", "", kind, ev)
	if err != nil {
		return rec, err
	}
	return rec, routeEventTx(ctx, c.Tx, projectID, rec.Sequence, kind, ev)
}

type deliveryTarget struct{ department, subscription string }

// routeEventTx creates the deliveries for one event. A direct request
// goes to its target only, whatever that department subscribes to.
// Other events go to active departments of the same service whose
// enabled subscription names the kind and shares a category, except
// the department the event belongs to. Past the per-event limit the
// extra judge deliveries are held.
func routeEventTx(ctx context.Context, tx *sql.Tx, projectID string, seq int64, kind string, ev WorkEvent) error {
	now := Now()
	insert := func(t deliveryTarget, purpose, status, hold string) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO event_deliveries (id, project_id, event_sequence, department_id, subscription_id, purpose, status, hold_reason, created_at, updated_at)
			VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?)
			ON CONFLICT(event_sequence, department_id, purpose) DO NOTHING`,
			NewID("dlv"), projectID, seq, t.department, t.subscription, purpose, status, hold, now, now)
		return err
	}
	switch kind {
	case domain.EventDepartmentCheck, domain.EventWorkCollabRequested:
		d, err := departmentTx(ctx, tx, projectID, ev.TargetDepartmentID)
		if err != nil {
			return err
		}
		if d.Status != domain.StatusActive {
			return invalid("사용하지 않는 부서에는 요청할 수 없습니다")
		}
		purpose := domain.PurposeJudge
		if kind == domain.EventWorkCollabRequested {
			purpose = domain.PurposeCollabRequest
		}
		return insert(deliveryTarget{department: d.ID}, purpose, domain.DeliveryPending, "")
	}
	rows, err := tx.QueryContext(ctx, `SELECT s.id, s.department_id, s.event_kinds, s.categories FROM subscriptions s
		JOIN departments d ON d.id = s.department_id
		WHERE s.project_id = ? AND s.enabled = 1 AND d.status = 'active'
		ORDER BY s.created_at, s.id`, projectID)
	if err != nil {
		return err
	}
	var targets []deliveryTarget
	seen := map[string]bool{}
	for rows.Next() {
		var id, dept, kindsRaw, catsRaw string
		if err := rows.Scan(&id, &dept, &kindsRaw, &catsRaw); err != nil {
			rows.Close()
			return err
		}
		var kinds, cats []string
		json.Unmarshal([]byte(kindsRaw), &kinds)
		json.Unmarshal([]byte(catsRaw), &cats)
		if seen[dept] || dept == ev.DepartmentID || !slices.Contains(kinds, kind) {
			continue
		}
		if len(cats) > 0 && !slices.ContainsFunc(cats, func(c string) bool { return slices.Contains(ev.Categories, c) }) {
			continue
		}
		seen[dept] = true
		targets = append(targets, deliveryTarget{department: dept, subscription: id})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	var notify []string
	if strings.HasPrefix(kind, "document.") && ev.DocumentID != "" {
		doc, err := documentTx(ctx, tx, projectID, ev.DocumentID)
		if err != nil {
			return err
		}
		targets = slices.DeleteFunc(targets, func(t deliveryTarget) bool { return !doc.VisibleTo(t.department) })
		if notify, err = documentNotifyTx(ctx, tx, projectID, kind, ev, doc); err != nil {
			return err
		}
	}
	for i, t := range targets {
		status, hold := domain.DeliveryPending, ""
		if i >= domain.DefaultJudgeFanout {
			status = domain.DeliveryHeld
			hold = fmt.Sprintf("사건 하나에서 판단할 부서 수 한도(%d)를 넘었습니다", domain.DefaultJudgeFanout)
		}
		if err := insert(t, domain.PurposeJudge, status, hold); err != nil {
			return err
		}
	}
	for _, d := range notify {
		if slices.ContainsFunc(targets, func(t deliveryTarget) bool { return t.department == d }) {
			continue // the department is already asked to judge
		}
		if err := insert(deliveryTarget{department: d}, domain.PurposeNotify, domain.DeliveryPending, ""); err != nil {
			return err
		}
	}
	return nil
}

// documentNotifyTx lists the departments to notify about a document
// event: those whose work used an affected version (other than the
// document's own work), and for a confirmation also those that follow
// the document's shares. Only active departments that may see the
// document are listed (plan §4.2, §4.4).
func documentNotifyTx(ctx context.Context, tx *sql.Tx, projectID, kind string, ev WorkEvent, doc SharedDocument) ([]string, error) {
	var versions []string
	for _, v := range doc.Versions {
		switch kind {
		case domain.EventDocumentRevised:
			if v.ID != ev.DocumentVersionID {
				versions = append(versions, v.ID)
			}
		case domain.EventDocumentWithdrawn:
			if ev.DocumentVersionID == "" || v.ID == ev.DocumentVersionID {
				versions = append(versions, v.ID)
			}
		case domain.EventDocumentConfirmed:
			if v.ID == ev.DocumentVersionID {
				versions = append(versions, v.ID)
			}
		}
	}
	items, err := usersOfVersionsTx(ctx, tx, versions)
	if err != nil {
		return nil, err
	}
	var out []string
	add := func(dept string) error {
		if dept == "" || slices.Contains(out, dept) || !doc.VisibleTo(dept) {
			return nil
		}
		d, err := departmentTx(ctx, tx, projectID, dept)
		if err != nil {
			return err
		}
		if d.Status == domain.StatusActive {
			out = append(out, dept)
		}
		return nil
	}
	for _, id := range items {
		if id == doc.WorkItemID {
			continue
		}
		w, err := workTx(ctx, tx, projectID, id)
		if err != nil {
			return nil, err
		}
		if err := add(w.DepartmentID); err != nil {
			return nil, err
		}
	}
	if kind == domain.EventDocumentConfirmed {
		rows, err := tx.QueryContext(ctx, `SELECT department_id, event_kinds, categories FROM subscriptions WHERE project_id = ? AND enabled = 1`, projectID)
		if err != nil {
			return nil, err
		}
		var follow []string
		for rows.Next() {
			var dept, kindsRaw, catsRaw string
			if err := rows.Scan(&dept, &kindsRaw, &catsRaw); err != nil {
				rows.Close()
				return nil, err
			}
			var kinds, cats []string
			json.Unmarshal([]byte(kindsRaw), &kinds)
			json.Unmarshal([]byte(catsRaw), &cats)
			follows := slices.Contains(kinds, domain.EventDocumentShared) || slices.Contains(kinds, domain.EventDocumentRevised)
			if follows && (len(cats) == 0 || slices.ContainsFunc(cats, func(c string) bool { return slices.Contains(ev.Categories, c) })) {
				follow = append(follow, dept)
			}
		}
		rows.Close()
		for _, d := range follow {
			if d != ev.DepartmentID {
				if err := add(d); err != nil {
					return nil, err
				}
			}
		}
	}
	return out, nil
}

// workEventFor is the payload for an event about a work item.
func workEventFor(w domain.WorkItem) WorkEvent {
	goal := w.ParentID
	if w.Kind == domain.WorkGoal {
		goal = w.ID
	}
	return WorkEvent{DepartmentID: w.DepartmentID, WorkItemID: w.ID, GoalID: goal, Title: w.Title, Categories: w.Categories}
}

// RequestDepartmentCheck asks one department to look for work to do. It
// reaches the department whether or not it subscribes to anything.
func (db *DB) RequestDepartmentCheck(ctx context.Context, projectID, departmentID, note string) (EventRecord, error) {
	var rec EventRecord
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		d, err := departmentTx(ctx, c.Tx, projectID, departmentID)
		if err != nil {
			return err
		}
		rec, err = emitWorkEventTx(ctx, c, projectID, domain.EventDepartmentCheck, WorkEvent{
			TargetDepartmentID: d.ID, Title: d.Name + " 점검 요청", Note: strings.TrimSpace(note)})
		return err
	})
	return rec, err
}

// ---- subscriptions ----

const subscriptionCols = `id, project_id, department_id, event_kinds, categories, guidance, enabled, updated_at`

func scanSubscription(s interface{ Scan(...any) error }) (domain.Subscription, error) {
	var sub domain.Subscription
	var kinds, cats string
	var enabled int
	err := s.Scan(&sub.ID, &sub.ProjectID, &sub.DepartmentID, &kinds, &cats, &sub.Guidance, &enabled, &sub.UpdatedAt)
	json.Unmarshal([]byte(kinds), &sub.EventKinds)
	json.Unmarshal([]byte(cats), &sub.Categories)
	sub.EventKinds, sub.Categories, sub.Enabled = orEmpty(sub.EventKinds), orEmpty(sub.Categories), enabled == 1
	return sub, err
}

// Subscriptions lists a service's subscriptions, or one department's.
func (db *DB) Subscriptions(ctx context.Context, projectID, departmentID string) ([]domain.Subscription, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+subscriptionCols+` FROM subscriptions
		WHERE project_id = ? AND (? = '' OR department_id = ?) ORDER BY created_at, id`, projectID, departmentID, departmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Subscription{}
	for rows.Next() {
		s, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// SaveSubscription creates (empty ID) or updates a department's
// subscription. A subscription is turned off rather than deleted, since
// deliveries keep pointing at it.
func (db *DB) SaveSubscription(ctx context.Context, s domain.Subscription) (domain.Subscription, error) {
	s.EventKinds, s.Categories = dedupe(s.EventKinds), dedupe(s.Categories)
	if len(s.EventKinds) == 0 {
		return s, invalid("받을 사건 종류를 하나 이상 고르세요")
	}
	for _, k := range s.EventKinds {
		if !slices.Contains(domain.SubscribableEvents, k) {
			return s, invalid("구독할 수 없는 사건 종류입니다: %s", k)
		}
	}
	kinds, _ := json.Marshal(s.EventKinds)
	cats, _ := json.Marshal(s.Categories)
	enabled := 0
	if s.Enabled {
		enabled = 1
	}
	err := db.Write(ctx, func(tx *sql.Tx) error {
		if err := writableProjectTx(ctx, tx, s.ProjectID); err != nil {
			return err
		}
		if s.ID == "" {
			s.ID = NewID("sub")
		} else {
			var dept string
			err := tx.QueryRowContext(ctx, `SELECT department_id FROM subscriptions WHERE id = ? AND project_id = ?`, s.ID, s.ProjectID).Scan(&dept)
			if errors.Is(err, sql.ErrNoRows) {
				return fmt.Errorf("subscription %q: %w", s.ID, ErrNotInProject)
			}
			if err != nil {
				return err
			}
			s.DepartmentID = dept // a subscription never changes department
		}
		if _, err := departmentTx(ctx, tx, s.ProjectID, s.DepartmentID); err != nil {
			return err
		}
		now := Now()
		_, err := tx.ExecContext(ctx, `INSERT INTO subscriptions (id, project_id, department_id, event_kinds, categories, guidance, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET event_kinds = excluded.event_kinds, categories = excluded.categories,
				guidance = excluded.guidance, enabled = excluded.enabled, updated_at = excluded.updated_at`,
			s.ID, s.ProjectID, s.DepartmentID, string(kinds), string(cats), strings.TrimSpace(s.Guidance), enabled, now, now)
		return err
	})
	if err != nil {
		return s, err
	}
	return scanSubscription(db.sql.QueryRowContext(ctx, `SELECT `+subscriptionCols+` FROM subscriptions WHERE id = ?`, s.ID))
}

// ---- deliveries ----

// Delivery is one event waiting for (or handled by) one department.
type Delivery struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"projectId"`
	EventSequence  int64     `json:"eventSequence"`
	EventKind      string    `json:"eventKind"`
	Event          WorkEvent `json:"event"`
	EventAt        string    `json:"eventAt"`
	DepartmentID   string    `json:"departmentId"`
	SubscriptionID string    `json:"subscriptionId"`
	Purpose        string    `json:"purpose"`
	Status         string    `json:"status"`
	Note           string    `json:"note"`
	Error          string    `json:"error"`
	HoldReason     string    `json:"holdReason"`
	CreatedAt      string    `json:"createdAt"`
	UpdatedAt      string    `json:"updatedAt"`
}

const deliveryCols = `d.id, d.project_id, d.event_sequence, e.kind, e.payload, e.created_at, d.department_id, COALESCE(d.subscription_id, ''),
	d.purpose, d.status, d.note, d.error, d.hold_reason, d.created_at, d.updated_at`

const deliveryFrom = ` FROM event_deliveries d JOIN execution_events e ON e.sequence = d.event_sequence`

func scanDelivery(s interface{ Scan(...any) error }) (Delivery, error) {
	var d Delivery
	var payload string
	err := s.Scan(&d.ID, &d.ProjectID, &d.EventSequence, &d.EventKind, &payload, &d.EventAt, &d.DepartmentID, &d.SubscriptionID,
		&d.Purpose, &d.Status, &d.Note, &d.Error, &d.HoldReason, &d.CreatedAt, &d.UpdatedAt)
	json.Unmarshal([]byte(payload), &d.Event)
	d.Event.Categories = orEmpty(d.Event.Categories)
	return d, err
}

func queryDeliveries(ctx context.Context, q rowsQueryer, where string, args ...any) ([]Delivery, error) {
	rows, err := q.QueryContext(ctx, `SELECT `+deliveryCols+deliveryFrom+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Delivery{}
	for rows.Next() {
		d, err := scanDelivery(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeliveryFilter narrows a service's deliveries. Status may be a status
// or "open" (anything not done or skipped).
type DeliveryFilter struct {
	DepartmentID string `json:"departmentId"`
	Status       string `json:"status"`
	Limit        int    `json:"limit"`
}

// Deliveries lists a service's deliveries, newest event first.
func (db *DB) Deliveries(ctx context.Context, projectID string, f DeliveryFilter) ([]Delivery, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return nil, err
	}
	where := `d.project_id = ?`
	args := []any{projectID}
	if f.DepartmentID != "" {
		where += ` AND d.department_id = ?`
		args = append(args, f.DepartmentID)
	}
	switch f.Status {
	case "":
	case "open":
		where += ` AND d.status NOT IN ('done', 'skipped')`
	default:
		where += ` AND d.status = ?`
		args = append(args, f.Status)
	}
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 200
	}
	return queryDeliveries(ctx, db.sql, where+` ORDER BY d.event_sequence DESC, d.created_at LIMIT ?`, append(args, f.Limit)...)
}

// WorkEventView is one event of a work item with where it went.
type WorkEventView struct {
	Sequence   int64      `json:"sequence"`
	Kind       string     `json:"kind"`
	Event      WorkEvent  `json:"event"`
	CreatedAt  string     `json:"createdAt"`
	Deliveries []Delivery `json:"deliveries"`
}

// WorkEvents lists a work item's events, oldest first, with their
// deliveries.
func (db *DB) WorkEvents(ctx context.Context, projectID, workItemID string) ([]WorkEventView, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefWorkItem, workItemID}); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT sequence, kind, payload, created_at FROM execution_events
		WHERE project_id = ? AND kind >= 'work.' AND kind < 'work/' AND json_extract(payload, '$.workItemId') = ?
		ORDER BY sequence`, projectID, workItemID)
	if err != nil {
		return nil, err
	}
	out := []WorkEventView{}
	index := map[int64]int{}
	for rows.Next() {
		var v WorkEventView
		var payload string
		if err := rows.Scan(&v.Sequence, &v.Kind, &payload, &v.CreatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		json.Unmarshal([]byte(payload), &v.Event)
		v.Event.Categories = orEmpty(v.Event.Categories)
		v.Deliveries = []Delivery{}
		index[v.Sequence] = len(out)
		out = append(out, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	dels, err := queryDeliveries(ctx, db.sql, `d.project_id = ? AND d.event_sequence >= ? ORDER BY d.created_at`, projectID, out[0].Sequence)
	if err != nil {
		return nil, err
	}
	for _, d := range dels {
		if i, ok := index[d.EventSequence]; ok {
			out[i].Deliveries = append(out[i].Deliveries, d)
		}
	}
	return out, nil
}

// PendingDeliveries returns the oldest pending deliveries of every
// service, for the delivery processor.
func (db *DB) PendingDeliveries(ctx context.Context, limit int) ([]Delivery, error) {
	return queryDeliveries(ctx, db.sql, `d.status = 'pending' ORDER BY d.event_sequence, d.created_at LIMIT ?`, limit)
}

// ClaimDelivery moves a pending delivery to processing. It reports false
// when someone else already took it, so a delivery is handled once even
// if the processor is woken twice.
func (db *DB) ClaimDelivery(ctx context.Context, id string) (bool, error) {
	var claimed bool
	err := db.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE event_deliveries SET status = 'processing', updated_at = ? WHERE id = ? AND status = 'pending'`, Now(), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		claimed = n == 1
		return nil
	})
	return claimed, err
}

// FinishDelivery records the outcome of a delivery being processed.
func (db *DB) FinishDelivery(ctx context.Context, id, status, note, errText string) error {
	switch status {
	case domain.DeliveryDone, domain.DeliverySkipped, domain.DeliveryWaitingAnswer:
		errText = ""
	case domain.DeliveryFailed:
		if strings.TrimSpace(errText) == "" {
			errText = "알 수 없는 오류"
		}
	default:
		return invalid("처리 결과로 쓸 수 없는 상태입니다: %s", status)
	}
	return db.changeDelivery(ctx, "", id, []string{domain.DeliveryProcessing}, status, note, errText, "")
}

// RetryDelivery puts a failed or held delivery back in the queue.
func (db *DB) RetryDelivery(ctx context.Context, projectID, id string) error {
	return db.changeDelivery(ctx, projectID, id, []string{domain.DeliveryFailed, domain.DeliveryHeld}, domain.DeliveryPending, "", "", "")
}

// SkipDelivery closes a delivery a person decided not to act on.
func (db *DB) SkipDelivery(ctx context.Context, projectID, id, reason string) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "사용자가 건너뜀"
	}
	return db.changeDelivery(ctx, projectID, id,
		[]string{domain.DeliveryPending, domain.DeliveryFailed, domain.DeliveryHeld}, domain.DeliverySkipped, reason, "", "")
}

// changeDelivery moves a delivery from one of the allowed statuses to a
// new one and emits delivery.updated so open screens refresh. projectID
// "" skips the service check (the processor works across services).
func (db *DB) changeDelivery(ctx context.Context, projectID, id string, from []string, to, note, errText, hold string) error {
	_, err := db.Change(ctx, func(c *Change) error {
		var project, status string
		err := c.Tx.QueryRowContext(ctx, `SELECT project_id, status FROM event_deliveries WHERE id = ? AND (? = '' OR project_id = ?)`,
			id, projectID, projectID).Scan(&project, &status)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("delivery %q: %w", id, ErrNotInProject)
		}
		if err != nil {
			return err
		}
		if !slices.Contains(from, status) {
			return invalid("지금 상태(%s)에서는 할 수 없는 처리입니다", status)
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE event_deliveries SET status = ?, note = ?, error = ?, hold_reason = ?, updated_at = ? WHERE id = ?`,
			to, note, errText, hold, Now(), id); err != nil {
			return err
		}
		return c.Emit(project, "", "", "delivery.updated", map[string]string{"deliveryId": id, "status": to})
	})
	return err
}

// RecoverDeliveries returns deliveries left processing by a crash to the
// queue. It runs before the processor starts.
func (db *DB) RecoverDeliveries(ctx context.Context) (int, error) {
	var n int64
	err := db.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE event_deliveries SET status = 'pending', updated_at = ? WHERE status = 'processing'`, Now())
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		return nil
	})
	return int(n), err
}
