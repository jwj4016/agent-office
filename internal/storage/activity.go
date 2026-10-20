package storage

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"agent-office/internal/providers"
)

type ActivityPage struct {
	Events  []EventRecord `json:"events"`
	HasMore bool          `json:"hasMore"`
}

// Activity returns the newest page before a sequence, restricted to a run or
// assist. Callers validate the source against the project before querying.
func (db *DB) Activity(ctx context.Context, projectID, runID, assistID string, before int64) (ActivityPage, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT e.sequence, e.id, e.project_id, COALESCE(e.run_id, ''), COALESCE(e.step_attempt_id, ''), e.kind, e.payload, e.created_at,
		COALESCE(s.step_id, ''), COALESCE(s.assignment_id, ''), COALESCE(s.attempt, 0)
		FROM execution_events e LEFT JOIN step_attempts s ON s.id = e.step_attempt_id
		WHERE e.project_id = ? AND (? = 0 OR e.sequence < ?)
		AND ((? <> '' AND e.run_id = ?) OR (? <> '' AND json_extract(e.payload, '$.assistId') = ?))
		ORDER BY e.sequence DESC LIMIT 201`, projectID, before, before, runID, runID, assistID, assistID)
	if err != nil {
		return ActivityPage{}, err
	}
	defer rows.Close()
	page := ActivityPage{Events: []EventRecord{}}
	for rows.Next() {
		var ev EventRecord
		var raw string
		if err := rows.Scan(&ev.Sequence, &ev.ID, &ev.ProjectID, &ev.RunID, &ev.StepAttemptID, &ev.Kind, &raw, &ev.CreatedAt, &ev.StepID, &ev.AssignmentID, &ev.Attempt); err != nil {
			return page, err
		}
		ev.Payload = json.RawMessage(raw)
		page.Events = append(page.Events, ev)
	}
	if len(page.Events) > 200 {
		page.HasMore = true
		page.Events = page.Events[:200]
	}
	for i, j := 0, len(page.Events)-1; i < j; i, j = i+1, j-1 {
		page.Events[i], page.Events[j] = page.Events[j], page.Events[i]
	}
	return page, rows.Err()
}

// OutputRecorder batches public streaming text into durable events. It flushes
// every second, before non-delta events and on close, including cancellation.
// Private reasoning is never part of the normalized provider text stream.
type OutputRecorder struct {
	mu      sync.Mutex
	pending strings.Builder
	write   func(string) error
	err     error
	stop    chan struct{}
	done    chan struct{}
}

func NewOutputRecorder(write func(string) error) *OutputRecorder {
	r := &OutputRecorder{write: write, stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		tick := time.NewTicker(time.Second)
		defer tick.Stop()
		for {
			select {
			case <-tick.C:
				r.Flush()
			case <-r.stop:
				r.Flush()
				return
			}
		}
	}()
	return r
}

func (r *OutputRecorder) Add(ev providers.Event) error {
	if ev.Kind != providers.KindMessageDelta {
		return r.Flush()
	}
	var p providers.TextPayload
	if err := json.Unmarshal(ev.Payload, &p); err != nil {
		return err
	}
	r.mu.Lock()
	r.pending.WriteString(p.Text)
	full := r.pending.Len() >= 16384
	r.mu.Unlock()
	if full {
		return r.Flush()
	}
	return nil
}

func (r *OutputRecorder) Flush() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.pending.Len() > 0 {
		if err := r.write(r.pending.String()); err != nil {
			r.err = err
			return err
		}
		r.pending.Reset()
	}
	return r.err
}

func (r *OutputRecorder) Close() error { close(r.stop); <-r.done; return r.err }
