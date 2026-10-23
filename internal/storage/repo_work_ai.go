package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// AI job lanes and statuses (plan §4.3, §4.7).
const (
	LaneInteractive = "interactive"
	LaneBackground  = "background"

	JobPending       = "pending"
	JobWorking       = "working"
	JobWaitingAnswer = "waiting_answer"
	JobReady         = "ready"
	JobHeld          = "held"
	JobFailed        = "failed"

	JobPlan = "plan"
)

// AIJob is one logical AI request made outside a workflow run.
type AIJob struct {
	ID            string          `json:"id"`
	ProjectID     string          `json:"projectId"`
	Kind          string          `json:"kind"`
	Lane          string          `json:"lane"`
	RequestKey    string          `json:"requestKey"`
	WorkItemID    string          `json:"workItemId"`
	DeliveryID    string          `json:"deliveryId"`
	Round         int             `json:"round"`
	AssignmentID  string          `json:"assignmentId"`
	ConnectionID  string          `json:"connectionId"`
	Model         string          `json:"model"`
	Status        string          `json:"status"`
	InputSnapshot json.RawMessage `json:"inputSnapshot"`
	InputHash     string          `json:"inputHash"`
	Result        json.RawMessage `json:"result"`
	Error         string          `json:"error"`
	HoldReason    string          `json:"holdReason"`
	MaxAttempts   int             `json:"maxAttempts"`
	Attempts      int             `json:"attempts"`
	AppliedAt     string          `json:"appliedAt"`
	CreatedAt     string          `json:"createdAt"`
	UpdatedAt     string          `json:"updatedAt"`
}

const aiJobCols = `j.id, j.project_id, j.kind, j.lane, j.request_key, COALESCE(j.work_item_id, ''), COALESCE(j.delivery_id, ''), COALESCE(j.round, 0),
	COALESCE(j.assignment_id, ''), COALESCE(j.connection_id, ''), j.model, j.status, j.input_snapshot, j.input_hash, j.result, j.error, j.hold_reason,
	j.max_attempts, (SELECT COUNT(*) FROM work_ai_attempts a WHERE a.job_id = j.id), COALESCE(j.applied_at, ''), j.created_at, j.updated_at`

func scanAIJob(s interface{ Scan(...any) error }) (AIJob, error) {
	var j AIJob
	var snap, result string
	err := s.Scan(&j.ID, &j.ProjectID, &j.Kind, &j.Lane, &j.RequestKey, &j.WorkItemID, &j.DeliveryID, &j.Round, &j.AssignmentID, &j.ConnectionID,
		&j.Model, &j.Status, &snap, &j.InputHash, &result, &j.Error, &j.HoldReason, &j.MaxAttempts, &j.Attempts, &j.AppliedAt, &j.CreatedAt, &j.UpdatedAt)
	j.InputSnapshot, j.Result = json.RawMessage(snap), json.RawMessage(result)
	return j, err
}

func aiJobTx(ctx context.Context, q queryer, id string) (AIJob, error) {
	j, err := scanAIJob(q.QueryRowContext(ctx, `SELECT `+aiJobCols+` FROM work_ai_jobs j WHERE j.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return j, fmt.Errorf("ai job %q: %w", id, ErrNotInProject)
	}
	return j, err
}

// AIJob returns one job of the service.
func (db *DB) AIJob(ctx context.Context, projectID, id string) (AIJob, error) {
	j, err := aiJobTx(ctx, db.sql, id)
	if err == nil && j.ProjectID != projectID {
		return AIJob{}, fmt.Errorf("ai job %q: %w", id, ErrNotInProject)
	}
	return j, err
}

// AIJobs lists a work item's jobs of one kind, newest first.
func (db *DB) AIJobs(ctx context.Context, projectID, workItemID, kind string) ([]AIJob, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefWorkItem, workItemID}); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT `+aiJobCols+` FROM work_ai_jobs j WHERE j.project_id = ? AND j.work_item_id = ? AND j.kind = ?
		ORDER BY j.created_at DESC, j.rowid DESC`, projectID, workItemID, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AIJob{}
	for rows.Next() {
		j, err := scanAIJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// CreateAIJob queues a job. A second request with the same key (a double
// click, a repeated delivery) returns the job already queued instead of
// creating another; created reports which happened.
func (db *DB) CreateAIJob(ctx context.Context, j AIJob) (AIJob, bool, error) {
	if j.Lane != LaneInteractive && j.Lane != LaneBackground {
		return j, false, invalid("알 수 없는 AI 작업 대기열입니다")
	}
	if j.RequestKey == "" {
		return j, false, invalid("AI 작업 요청 키가 필요합니다")
	}
	var created bool
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, j.ProjectID); err != nil {
			return err
		}
		var existing string
		err := c.Tx.QueryRowContext(ctx, `SELECT id FROM work_ai_jobs WHERE project_id = ? AND request_key = ?`, j.ProjectID, j.RequestKey).Scan(&existing)
		if err == nil {
			j.ID = existing
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		j.ID, created = NewID("aij"), true
		now := Now()
		var round any
		if j.Round > 0 {
			round = j.Round
		}
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO work_ai_jobs (id, project_id, kind, lane, request_key, work_item_id, delivery_id, round,
			assignment_id, connection_id, model, status, input_snapshot, input_hash, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, NULLIF(?, ''), NULLIF(?, ''), ?, 'pending', ?, ?, ?, ?)`,
			j.ID, j.ProjectID, j.Kind, j.Lane, j.RequestKey, j.WorkItemID, j.DeliveryID, round, j.AssignmentID, j.ConnectionID, j.Model,
			rawOr(j.InputSnapshot, "{}"), j.InputHash, now, now); err != nil {
			return err
		}
		return emitJobUpdate(c, j.ProjectID, j.ID, JobPending)
	})
	if err != nil {
		return j, false, err
	}
	out, err := db.AIJob(ctx, j.ProjectID, j.ID)
	return out, created, err
}

// emitJobUpdate lets open screens refresh; it is not a work event.
func emitJobUpdate(c *Change, projectID, id, status string) error {
	return c.Emit(projectID, "", "", "ai_job.updated", map[string]string{"jobId": id, "status": status})
}

// NextAIJob returns the oldest pending job of a lane, or false.
func (db *DB) NextAIJob(ctx context.Context, lane string) (AIJob, bool, error) {
	j, err := scanAIJob(db.sql.QueryRowContext(ctx, `SELECT `+aiJobCols+` FROM work_ai_jobs j WHERE j.lane = ? AND j.status = 'pending'
		ORDER BY j.created_at, j.rowid LIMIT 1`, lane))
	if errors.Is(err, sql.ErrNoRows) {
		return j, false, nil
	}
	return j, err == nil, err
}

// ClaimAIJob moves a pending job to working, reporting false if it was
// taken meanwhile.
func (db *DB) ClaimAIJob(ctx context.Context, id string) (bool, error) {
	var ok bool
	_, err := db.Change(ctx, func(c *Change) error {
		j, err := aiJobTx(ctx, c.Tx, id)
		if err != nil {
			return err
		}
		res, err := c.Tx.ExecContext(ctx, `UPDATE work_ai_jobs SET status = 'working', updated_at = ? WHERE id = ? AND status = 'pending'`, Now(), id)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		if ok = n == 1; !ok {
			return nil
		}
		return emitJobUpdate(c, j.ProjectID, id, JobWorking)
	})
	return ok, err
}

// StartAIAttempt records the provider call about to be made for a
// working job and returns its id. It is written before the call so a
// crash during the call still leaves a trace and counts as an attempt.
func (db *DB) StartAIAttempt(ctx context.Context, j AIJob) (string, error) {
	id := NewID("aia")
	err := db.Write(ctx, func(tx *sql.Tx) error {
		var n int
		tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM work_ai_attempts WHERE job_id = ?`, j.ID).Scan(&n)
		_, err := tx.ExecContext(ctx, `INSERT INTO work_ai_attempts (id, project_id, job_id, number, connection_id, model, status, started_at)
			VALUES (?, ?, ?, ?, NULLIF(?, ''), ?, 'working', ?)`, id, j.ProjectID, j.ID, n+1, j.ConnectionID, j.Model, Now())
		return err
	})
	return id, err
}

// RecordAIUsage keeps the latest running total an attempt reported.
func (db *DB) RecordAIUsage(ctx context.Context, attemptID string, usage json.RawMessage) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE work_ai_attempts SET usage = ?, usage_reported = 1 WHERE id = ?`, string(usage), attemptID)
		return err
	})
}

// FinishAIAttempt ends a working attempt.
func (db *DB) FinishAIAttempt(ctx context.Context, attemptID, status, errText string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE work_ai_attempts SET status = ?, error = ?, ended_at = ? WHERE id = ? AND status = 'working'`,
			status, errText, Now(), attemptID)
		return err
	})
}

// FinishAIJob records a working job's outcome: ready with a result,
// held with a reason, failed with an error, or back to pending for an
// automatic retry.
func (db *DB) FinishAIJob(ctx context.Context, id, status string, result json.RawMessage, errText, hold string) error {
	if !slices.Contains([]string{JobPending, JobReady, JobHeld, JobFailed, JobWaitingAnswer}, status) {
		return invalid("AI 작업 결과로 쓸 수 없는 상태입니다: %s", status)
	}
	if status == JobFailed && errText == "" {
		errText = "알 수 없는 오류"
	}
	if status == JobHeld && hold == "" {
		hold = "보류됨"
	}
	_, err := db.Change(ctx, func(c *Change) error {
		j, err := aiJobTx(ctx, c.Tx, id)
		if err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE work_ai_jobs SET status = ?, result = ?, error = ?, hold_reason = ?, updated_at = ?
			WHERE id = ? AND status = 'working'`, status, rawOr(result, string(j.Result)), errText, hold, Now(), id); err != nil {
			return err
		}
		return emitJobUpdate(c, j.ProjectID, id, status)
	})
	return err
}

// RetryAIJob queues a failed or held job again. A failed job gets one
// more attempt; its earlier attempts and their cost stay recorded.
func (db *DB) RetryAIJob(ctx context.Context, projectID, id string) error {
	_, err := db.Change(ctx, func(c *Change) error {
		j, err := aiJobTx(ctx, c.Tx, id)
		if err != nil || j.ProjectID != projectID {
			return fmt.Errorf("ai job %q: %w", id, ErrNotInProject)
		}
		extra := 0
		switch j.Status {
		case JobFailed:
			extra = 1
		case JobHeld:
		default:
			return invalid("실패하거나 보류된 AI 작업만 다시 시도할 수 있습니다")
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE work_ai_jobs SET status = 'pending', error = '', hold_reason = '',
			max_attempts = MAX(max_attempts, ?) + ?, updated_at = ? WHERE id = ?`, j.Attempts, extra, Now(), id); err != nil {
			return err
		}
		return emitJobUpdate(c, projectID, id, JobPending)
	})
	return err
}

// MarkAIJobApplied records that a person used a ready job's result.
func (db *DB) MarkAIJobApplied(ctx context.Context, projectID, id string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE work_ai_jobs SET applied_at = ?, updated_at = ? WHERE id = ? AND project_id = ? AND status = 'ready'`,
			Now(), Now(), id, projectID)
		return err
	})
}

// RecoverAIJobs runs at start: calls cut off by the app stopping are
// interrupted attempts; their jobs go back to the queue, or fail when
// they have used every attempt.
func (db *DB) RecoverAIJobs(ctx context.Context) error {
	_, err := db.Change(ctx, func(c *Change) error {
		if _, err := c.Tx.ExecContext(ctx, `UPDATE work_ai_attempts SET status = 'interrupted', error = '앱이 다시 시작되어 중단되었습니다', ended_at = ?
			WHERE status = 'working'`, Now()); err != nil {
			return err
		}
		rows, err := c.Tx.QueryContext(ctx, `SELECT `+aiJobCols+` FROM work_ai_jobs j WHERE j.status = 'working'`)
		if err != nil {
			return err
		}
		var jobs []AIJob
		for rows.Next() {
			j, err := scanAIJob(rows)
			if err != nil {
				rows.Close()
				return err
			}
			jobs = append(jobs, j)
		}
		rows.Close()
		for _, j := range jobs {
			status, errText := JobPending, ""
			if j.Attempts >= j.MaxAttempts {
				status, errText = JobFailed, "앱이 다시 시작되어 중단되었고 시도 횟수를 모두 썼습니다"
			}
			if _, err := c.Tx.ExecContext(ctx, `UPDATE work_ai_jobs SET status = ?, error = ?, updated_at = ? WHERE id = ?`,
				status, errText, Now(), j.ID); err != nil {
				return err
			}
			if err := emitJobUpdate(c, j.ProjectID, j.ID, status); err != nil {
				return err
			}
		}
		return nil
	})
	return err
}

// AIAttemptUsage is the summed usage of a service's AI work outside runs.
type AIAttemptUsage struct {
	Attempts, UnknownCost     int
	InputTokens, OutputTokens int64
	CostUSD                   float64
}

// SumAIAttemptUsage adds up every attempt's reported usage. An attempt
// that reported no cost (or nothing at all) counts as unknown cost.
func SumAIAttemptUsage(ctx context.Context, q rowsQueryer, projectID string) (AIAttemptUsage, error) {
	var u AIAttemptUsage
	rows, err := q.QueryContext(ctx, `SELECT usage, usage_reported FROM work_ai_attempts WHERE project_id = ?`, projectID)
	if err != nil {
		return u, err
	}
	defer rows.Close()
	for rows.Next() {
		var raw string
		var reported int
		if err := rows.Scan(&raw, &reported); err != nil {
			return u, err
		}
		var p struct {
			InputTokens  int64    `json:"inputTokens"`
			OutputTokens int64    `json:"outputTokens"`
			CostUSD      *float64 `json:"costUsd"`
		}
		json.Unmarshal([]byte(raw), &p)
		u.Attempts++
		u.InputTokens += p.InputTokens
		u.OutputTokens += p.OutputTokens
		if reported == 0 || p.CostUSD == nil {
			u.UnknownCost++
		} else {
			u.CostUSD += *p.CostUSD
		}
	}
	return u, rows.Err()
}
