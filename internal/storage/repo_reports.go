package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"agent-office/internal/domain"
)

// Completion reports (company requests, 5th revision): once all the work a
// request registered is closed, its desk writes a report, summary first.
// Each call is kept in request_reports with the usage it reported.

// RequestsAwaitingReport lists service requests whose registered work is
// all closed, some of it done, and that have no report yet; nothing else
// may be waiting on the person and no report call may be going.
func (db *DB) RequestsAwaitingReport(ctx context.Context, limit int) ([]domain.Request, error) {
	return db.queryRequests(ctx, `r.status = 'in_progress' AND r.report IS NULL AND r.project_id IS NOT NULL
		AND NOT EXISTS (SELECT 1 FROM request_items i WHERE i.request_id = r.id AND i.status IN ('proposed', 'failed', 'check', 'running'))
		AND NOT EXISTS (SELECT 1 FROM request_items i JOIN work_items w ON w.id = i.work_item_id
			WHERE i.request_id = r.id AND w.status NOT IN ('done', 'cancelled'))
		AND EXISTS (SELECT 1 FROM request_items i JOIN work_items w ON w.id = i.work_item_id WHERE i.request_id = r.id AND w.status = 'done')
		AND NOT EXISTS (SELECT 1 FROM request_reports p WHERE p.request_id = r.id AND p.status = 'working')
		ORDER BY r.updated_at, r.rowid LIMIT ?`, limit)
}

// ReportTries counts the report calls made for a request.
func (db *DB) ReportTries(ctx context.Context, requestID string) (int, error) {
	var n int
	err := db.sql.QueryRowContext(ctx, `SELECT COUNT(*) FROM request_reports WHERE request_id = ?`, requestID).Scan(&n)
	return n, err
}

// StartReport records a report call before it is made.
func (db *DB) StartReport(ctx context.Context, r domain.Request, connectionID, model string) (string, error) {
	id := NewID("rpt")
	err := db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO request_reports (id, request_id, project_id, organization_id, status, connection_id, model, started_at)
			VALUES (?, ?, NULLIF(?, ''), ?, 'working', NULLIF(?, ''), ?, ?)`, id, r.ID, r.ProjectID, r.OrganizationID, connectionID, model, Now())
		return err
	})
	return id, err
}

// RecordReportUsage keeps the latest running total a report call reported.
func (db *DB) RecordReportUsage(ctx context.Context, id string, usage json.RawMessage) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE request_reports SET usage = ?, usage_reported = 1 WHERE id = ?`, string(usage), id)
		return err
	})
}

// FinishReport ends a working report call.
func (db *DB) FinishReport(ctx context.Context, id, errText string) error {
	status := "succeeded"
	if errText != "" {
		status = "failed"
	}
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE request_reports SET status = ?, error = ?, ended_at = ? WHERE id = ? AND status = 'working'`,
			status, errText, Now(), id)
		return err
	})
}

// CompleteReport keeps a request's completion report; the request is then
// over (and the company request it came from, when all its parts are).
func (db *DB) CompleteReport(ctx context.Context, id, report string) error {
	report = strings.TrimSpace(report)
	if report == "" {
		return invalid("완료 보고가 비어 있습니다")
	}
	return db.requestChange(ctx, func(c *Change) error {
		res, err := c.Tx.ExecContext(ctx, `UPDATE requests SET report = ?, updated_at = ? WHERE id = ? AND report IS NULL`, report, Now(), id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return nil
		}
		return refreshRequestTx(ctx, c, id)
	})
}

// MarkReportRead records that the person read a request's report.
func (db *DB) MarkReportRead(ctx context.Context, projectID, id string) error {
	return db.requestChange(ctx, func(c *Change) error {
		r, err := scopedRequestTx(ctx, c.Tx, projectID, id)
		if err != nil {
			return err
		}
		if r.Report == "" {
			return invalid("아직 완료 보고가 없습니다")
		}
		_, err = c.Tx.ExecContext(ctx, `UPDATE requests SET report_read_at = COALESCE(report_read_at, ?) WHERE id = ?`, Now(), id)
		return err
	})
}
