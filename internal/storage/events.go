package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
)

// EventRecord is a persisted execution event. Sequence is assigned by
// the database and is the order the UI must apply events in.
type EventRecord struct {
	Sequence      int64           `json:"sequence"`
	ID            string          `json:"id"`
	ProjectID     string          `json:"projectId"`
	RunID         string          `json:"runId,omitempty"`
	StepAttemptID string          `json:"stepAttemptId,omitempty"`
	Kind          string          `json:"kind"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     string          `json:"createdAt"`
}

// Change is one write transaction plus the events it produces. Events
// are stored in the same transaction as the state they describe and are
// published only after a successful commit.
type Change struct {
	Tx     *sql.Tx
	events []EventRecord
}

// Emit records an event inside the change's transaction.
func (c *Change) Emit(projectID, runID, stepAttemptID, kind string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("event %s payload: %w", kind, err)
	}
	ev := EventRecord{
		ID: NewID("ev"), ProjectID: projectID, RunID: runID, StepAttemptID: stepAttemptID,
		Kind: kind, Payload: raw, CreatedAt: Now(),
	}
	res, err := c.Tx.Exec(`INSERT INTO execution_events (id, project_id, run_id, step_attempt_id, kind, payload, created_at)
		VALUES (?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?, ?)`,
		ev.ID, ev.ProjectID, ev.RunID, ev.StepAttemptID, ev.Kind, string(ev.Payload), ev.CreatedAt)
	if err != nil {
		return fmt.Errorf("event %s: %w", kind, err)
	}
	ev.Sequence, _ = res.LastInsertId()
	c.events = append(c.events, ev)
	return nil
}

type subscribers struct {
	mu   sync.Mutex
	next int
	fns  map[int]func([]EventRecord)
}

// Subscribe registers fn to receive events after each commit, in
// sequence order. It returns an unsubscribe function. fn runs on the
// committing goroutine and must not block or write to the database.
func (db *DB) Subscribe(fn func([]EventRecord)) func() {
	db.subs.mu.Lock()
	defer db.subs.mu.Unlock()
	if db.subs.fns == nil {
		db.subs.fns = map[int]func([]EventRecord){}
	}
	id := db.subs.next
	db.subs.next++
	db.subs.fns[id] = fn
	return func() {
		db.subs.mu.Lock()
		defer db.subs.mu.Unlock()
		delete(db.subs.fns, id)
	}
}

// Change runs fn in one write transaction. On success the emitted events
// are returned and published to subscribers; on error nothing is stored
// or published.
func (db *DB) Change(ctx context.Context, fn func(c *Change) error) ([]EventRecord, error) {
	var events []EventRecord
	// Publishing happens while still holding the write lock so that
	// subscribers always observe events in commit (sequence) order.
	err := db.write(ctx, func(tx *sql.Tx) error {
		c := &Change{Tx: tx}
		if err := fn(c); err != nil {
			return err
		}
		events = c.events
		return nil
	}, func() {
		if len(events) == 0 {
			return
		}
		db.subs.mu.Lock()
		fns := make([]func([]EventRecord), 0, len(db.subs.fns))
		for _, fn := range db.subs.fns {
			fns = append(fns, fn)
		}
		db.subs.mu.Unlock()
		for _, fn := range fns {
			fn(events)
		}
	})
	if err != nil {
		return nil, err
	}
	return events, nil
}

// EventsAfter returns a run's events with sequence > after, for UI
// catch-up after a reload or reconnect.
func (db *DB) EventsAfter(ctx context.Context, projectID string, after int64, limit int) ([]EventRecord, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT sequence, id, project_id, COALESCE(run_id, ''), COALESCE(step_attempt_id, ''), kind, payload, created_at
		FROM execution_events WHERE project_id = ? AND sequence > ? ORDER BY sequence LIMIT ?`, projectID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EventRecord
	for rows.Next() {
		var ev EventRecord
		var payload string
		if err := rows.Scan(&ev.Sequence, &ev.ID, &ev.ProjectID, &ev.RunID, &ev.StepAttemptID, &ev.Kind, &payload, &ev.CreatedAt); err != nil {
			return nil, err
		}
		ev.Payload = json.RawMessage(payload)
		out = append(out, ev)
	}
	return out, rows.Err()
}
