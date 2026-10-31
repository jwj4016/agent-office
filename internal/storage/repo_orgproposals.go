package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"agent-office/internal/domain"
)

// OrgProposal is an AI's proposed organization for an existing service.
type OrgProposal struct {
	ID           string          `json:"id"`
	ProjectID    string          `json:"projectId"`
	Status       string          `json:"status"`
	ConnectionID string          `json:"connectionId"`
	Model        string          `json:"model"`
	Plan         domain.OrgPlan  `json:"plan"`
	Applied      *OrgApplied     `json:"applied,omitempty"`
	Error        string          `json:"error"`
	StartedAt    string          `json:"startedAt"`
	EndedAt      string          `json:"endedAt"`
	Usage        json.RawMessage `json:"usage"`
}

const orgProposalCols = `id, project_id, status, COALESCE(connection_id, ''), model, result, applied, error, started_at, COALESCE(ended_at, ''), usage`

func scanOrgProposal(s interface{ Scan(...any) error }) (OrgProposal, error) {
	var p OrgProposal
	var result, applied, usage string
	err := s.Scan(&p.ID, &p.ProjectID, &p.Status, &p.ConnectionID, &p.Model, &result, &applied, &p.Error, &p.StartedAt, &p.EndedAt, &usage)
	json.Unmarshal([]byte(result), &p.Plan)
	if applied != "" && applied != "{}" {
		var a OrgApplied
		if json.Unmarshal([]byte(applied), &a) == nil {
			p.Applied = &a
		}
	}
	p.Usage = json.RawMessage(usage)
	return p, err
}

// StartOrgProposal records a proposal call before it is made.
func (db *DB) StartOrgProposal(ctx context.Context, projectID, connectionID, model string) (string, error) {
	id := NewID("org")
	err := db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO org_proposals (id, project_id, status, connection_id, model, started_at)
			VALUES (?, ?, 'working', NULLIF(?, ''), ?, ?)`, id, projectID, connectionID, model, Now())
		return err
	})
	return id, err
}

// RecordOrgProposalUsage keeps the latest running total the call reported.
func (db *DB) RecordOrgProposalUsage(ctx context.Context, id string, usage json.RawMessage) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE org_proposals SET usage = ?, usage_reported = 1 WHERE id = ?`, string(usage), id)
		return err
	})
}

// FinishOrgProposal ends a working call with its checked plan, or why it
// failed.
func (db *DB) FinishOrgProposal(ctx context.Context, id string, plan *domain.OrgPlan, errText string) error {
	status, raw := "ready", []byte("{}")
	if errText != "" {
		status = "failed"
	} else {
		raw, _ = json.Marshal(plan)
	}
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE org_proposals SET status = ?, result = ?, error = ?, ended_at = ? WHERE id = ? AND status = 'working'`,
			status, string(raw), errText, Now(), id)
		return err
	})
}

// OrgProposal returns one proposal of a service.
func (db *DB) OrgProposal(ctx context.Context, projectID, id string) (OrgProposal, error) {
	p, err := scanOrgProposal(db.sql.QueryRowContext(ctx, `SELECT `+orgProposalCols+` FROM org_proposals WHERE id = ? AND project_id = ?`, id, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return p, fmt.Errorf("org proposal %q: %w", id, ErrNotInProject)
	}
	return p, err
}

// LatestOrgProposal returns a service's last proposal.
func (db *DB) LatestOrgProposal(ctx context.Context, projectID string) (OrgProposal, bool, error) {
	p, err := scanOrgProposal(db.sql.QueryRowContext(ctx, `SELECT `+orgProposalCols+` FROM org_proposals WHERE project_id = ?
		ORDER BY started_at DESC, rowid DESC LIMIT 1`, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	return p, err == nil, err
}

// CloseOrgProposal marks a ready proposal applied (with what it created)
// or discarded, once.
func (db *DB) CloseOrgProposal(ctx context.Context, projectID, id, status string, applied *OrgApplied) error {
	if status != "applied" && status != "discarded" {
		return invalid("조직 제안을 닫을 수 없는 상태입니다: %s", status)
	}
	raw := []byte("{}")
	if applied != nil {
		raw, _ = json.Marshal(applied)
	}
	return db.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `UPDATE org_proposals SET status = ?, applied = ? WHERE id = ? AND project_id = ? AND status = 'ready'`,
			status, string(raw), id, projectID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return invalid("이미 적용했거나 버린 조직 제안입니다")
		}
		return nil
	})
}
