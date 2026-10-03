package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"agent-office/internal/domain"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrInvalid   = errors.New("invalid input")
	ErrConflict  = errors.New("changed by someone else; reload and retry")
	ErrRoleCycle = errors.New("role hierarchy would contain a cycle")
	ErrRoleInUse = errors.New("role is still assigned in a project")
	ErrArchived  = errors.New("project is archived")
)

func rawOr(r json.RawMessage, def string) string {
	if len(strings.TrimSpace(string(r))) == 0 {
		return def
	}
	return string(r)
}

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

func (db *DB) Organization(ctx context.Context) (domain.Organization, error) {
	var o domain.Organization
	var policy string
	err := db.sql.QueryRowContext(ctx, `SELECT id, name, instructions, policy FROM organizations WHERE id = ?`,
		domain.DefaultOrganizationID).Scan(&o.ID, &o.Name, &o.Instructions, &policy)
	o.Policy = json.RawMessage(policy)
	return o, err
}

func (db *DB) UpdateOrganization(ctx context.Context, name, instructions string) error {
	if strings.TrimSpace(name) == "" {
		return invalid("회사 이름이 필요합니다")
	}
	return db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE organizations SET name = ?, instructions = ? WHERE id = ?`,
			name, instructions, domain.DefaultOrganizationID)
		return err
	})
}

const roleCols = `id, organization_id, COALESCE(parent_role_id, ''), name, mission, instructions, output_defaults, policy, appearance, updated_at`

func scanRole(s interface{ Scan(...any) error }) (domain.Role, error) {
	var r domain.Role
	var out, pol, app string
	err := s.Scan(&r.ID, &r.OrganizationID, &r.ParentRoleID, &r.Name, &r.Mission, &r.Instructions, &out, &pol, &app, &r.UpdatedAt)
	r.OutputDefaults, r.Policy, r.Appearance = json.RawMessage(out), json.RawMessage(pol), json.RawMessage(app)
	return r, err
}

func (db *DB) Roles(ctx context.Context) ([]domain.Role, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+roleCols+` FROM roles WHERE organization_id = ? ORDER BY name`, domain.DefaultOrganizationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Role
	for rows.Next() {
		r, err := scanRole(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func roleTx(ctx context.Context, q queryer, id string) (domain.Role, error) {
	r, err := scanRole(q.QueryRowContext(ctx, `SELECT `+roleCols+` FROM roles WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return r, fmt.Errorf("role %q: %w", id, ErrNotFound)
	}
	return r, err
}

func (db *DB) Role(ctx context.Context, id string) (domain.Role, error) {
	return roleTx(ctx, db.sql, id)
}

// SaveRole creates (empty ID) or updates a company role. Parent links
// that would form a cycle are rejected (spec §2.3).
func (db *DB) SaveRole(ctx context.Context, r domain.Role) (domain.Role, error) {
	if strings.TrimSpace(r.Name) == "" {
		return r, invalid("역할 이름이 필요합니다")
	}
	err := db.Write(ctx, func(tx *sql.Tx) error {
		now := Now()
		if r.ID == "" {
			r.ID = NewID("role")
		} else if _, err := roleTx(ctx, tx, r.ID); err != nil {
			return err
		}
		if err := checkRoleParent(ctx, tx, r.ID, r.ParentRoleID); err != nil {
			return err
		}
		r.OrganizationID, r.UpdatedAt = domain.DefaultOrganizationID, now
		_, err := tx.ExecContext(ctx, `INSERT INTO roles (id, organization_id, parent_role_id, name, mission, instructions, output_defaults, policy, appearance, created_at, updated_at)
			VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET parent_role_id = excluded.parent_role_id, name = excluded.name, mission = excluded.mission,
				instructions = excluded.instructions, output_defaults = excluded.output_defaults, policy = excluded.policy,
				appearance = excluded.appearance, updated_at = excluded.updated_at`,
			r.ID, r.OrganizationID, r.ParentRoleID, r.Name, r.Mission, r.Instructions,
			rawOr(r.OutputDefaults, "[]"), rawOr(r.Policy, "{}"), rawOr(r.Appearance, "{}"), now, now)
		return err
	})
	if err != nil {
		return r, err
	}
	return db.Role(ctx, r.ID)
}

func checkRoleParent(ctx context.Context, tx *sql.Tx, id, parent string) error {
	for p, depth := parent, 0; p != ""; depth++ {
		if p == id || depth > 64 {
			return ErrRoleCycle
		}
		var next sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT parent_role_id FROM roles WHERE id = ?`, p).Scan(&next)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("parent role %q: %w", p, ErrNotFound)
		}
		if err != nil {
			return err
		}
		p = next.String
	}
	return nil
}

// DeleteRole removes an unassigned role; its children move up one level.
func (db *DB) DeleteRole(ctx context.Context, id string) error {
	return db.Write(ctx, func(tx *sql.Tx) error {
		r, err := roleTx(ctx, tx, id)
		if err != nil {
			return err
		}
		var n int
		tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM assignments WHERE role_id = ?`, id).Scan(&n)
		if n > 0 {
			return ErrRoleInUse
		}
		if _, err := tx.ExecContext(ctx, `UPDATE roles SET parent_role_id = NULLIF(?, '') WHERE parent_role_id = ?`, r.ParentRoleID, id); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM roles WHERE id = ?`, id)
		return err
	})
}

// roleChain returns the role and its ancestors, root first.
func roleChain(ctx context.Context, q queryer, id string) ([]domain.Role, error) {
	var chain []domain.Role
	for cur := id; cur != "" && len(chain) <= 64; {
		r, err := roleTx(ctx, q, cur)
		if err != nil {
			return nil, err
		}
		chain = append([]domain.Role{r}, chain...)
		cur = r.ParentRoleID
	}
	return chain, nil
}

const connCols = `id, name, provider, executable_path, secret_ref, config, verified_capabilities`

func scanConn(s interface{ Scan(...any) error }) (domain.ProviderConnection, error) {
	var c domain.ProviderConnection
	var cfg, caps string
	err := s.Scan(&c.ID, &c.Name, &c.Provider, &c.ExecutablePath, &c.SecretRef, &cfg, &caps)
	c.Config, c.VerifiedCapabilities = json.RawMessage(cfg), json.RawMessage(caps)
	return c, err
}

func (db *DB) Connections(ctx context.Context) ([]domain.ProviderConnection, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT `+connCols+` FROM provider_connections ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ProviderConnection
	for rows.Next() {
		c, err := scanConn(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (db *DB) Connection(ctx context.Context, id string) (domain.ProviderConnection, error) {
	c, err := scanConn(db.sql.QueryRowContext(ctx, `SELECT `+connCols+` FROM provider_connections WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, fmt.Errorf("connection %q: %w", id, ErrNotFound)
	}
	return c, err
}

// SaveConnection creates or updates a provider connection. Any change
// clears VerifiedCapabilities unless the caller sets them: a connection
// must be re-verified after its settings change.
func (db *DB) SaveConnection(ctx context.Context, c domain.ProviderConnection) (domain.ProviderConnection, error) {
	if strings.TrimSpace(c.Name) == "" || c.Provider == "" {
		return c, invalid("연결 이름과 종류가 필요합니다")
	}
	if c.SecretRef != "" && !strings.HasPrefix(c.SecretRef, "secret://") {
		return c, invalid("API 키는 DB에 저장하지 않습니다")
	}
	if c.ID == "" {
		c.ID = NewID("conn")
	}
	now := Now()
	err := db.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `INSERT INTO provider_connections (id, name, provider, executable_path, secret_ref, config, verified_capabilities, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET name = excluded.name, provider = excluded.provider, executable_path = excluded.executable_path,
				secret_ref = excluded.secret_ref, config = excluded.config, verified_capabilities = excluded.verified_capabilities, updated_at = excluded.updated_at`,
			c.ID, c.Name, c.Provider, c.ExecutablePath, c.SecretRef, rawOr(c.Config, "{}"), rawOr(c.VerifiedCapabilities, "{}"), now, now)
		return err
	})
	if err != nil {
		return c, err
	}
	return db.Connection(ctx, c.ID)
}
