package storage

import (
	"context"
	"database/sql"

	"agent-office/internal/toolpolicy"
)

// ToolRule is a kind of command a service lets AI steps run without
// asking.
type ToolRule struct {
	ID        string `json:"id"`
	ProjectID string `json:"projectId"`
	toolpolicy.Rule
	Key       string `json:"key"`
	CreatedAt string `json:"createdAt"`
}

// ToolRules lists a service's rules.
func (db *DB) ToolRules(ctx context.Context, projectID string) ([]ToolRule, error) {
	return toolRulesTx(ctx, db.sql, projectID)
}

func toolRulesTx(ctx context.Context, q rowsQueryer, projectID string) ([]ToolRule, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, project_id, program, sub, created_at FROM tool_rules WHERE project_id = ? ORDER BY program, sub`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ToolRule{}
	for rows.Next() {
		var r ToolRule
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Program, &r.Sub, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.Key = r.Rule.Key()
		out = append(out, r)
	}
	return out, rows.Err()
}

// ToolPolicyRules returns a service's rules for the checker.
func ToolPolicyRules(ctx context.Context, q rowsQueryer, projectID string) ([]toolpolicy.Rule, error) {
	list, err := toolRulesTx(ctx, q, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]toolpolicy.Rule, 0, len(list))
	for _, r := range list {
		out = append(out, r.Rule)
	}
	return out, nil
}

// AddToolRules keeps rules for a service (existing ones stay).
func (db *DB) AddToolRules(ctx context.Context, projectID string, rules []toolpolicy.Rule) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		for _, r := range rules {
			if r.Program == "" {
				continue
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO tool_rules (id, project_id, program, sub, created_at) VALUES (?, ?, ?, ?, ?)
				ON CONFLICT(project_id, program, sub) DO NOTHING`, NewID("trule"), projectID, r.Program, r.Sub, Now()); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteToolRule takes a rule away: that kind of command asks again.
func (db *DB) DeleteToolRule(ctx context.Context, projectID, id string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM tool_rules WHERE id = ? AND project_id = ?`, id, projectID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return ErrNotInProject
		}
		return nil
	})
}
