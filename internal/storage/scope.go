package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// ErrNotInProject means an id does not exist inside the requested
// project. It is returned both for unknown ids and for ids that belong to
// another project, so callers cannot probe other projects' data.
var ErrNotInProject = errors.New("not found in this project")

// RefKind names a project-owned table that ids can be checked against.
type RefKind string

const (
	RefAssignment  RefKind = "assignment"
	RefWorkflow    RefKind = "workflow"
	RefVersion     RefKind = "workflow_version"
	RefRun         RefKind = "run"
	RefStepAttempt RefKind = "step_attempt"
	RefArtifact    RefKind = "artifact"
	RefMessage     RefKind = "message"
	RefApproval    RefKind = "approval"
	RefWorkspace   RefKind = "workspace"
)

// scopeTables is the allow-list of tables; kinds never reach SQL as text.
var scopeTables = map[RefKind]string{
	RefAssignment:  "assignments",
	RefWorkflow:    "workflows",
	RefVersion:     "workflow_versions",
	RefRun:         "runs",
	RefStepAttempt: "step_attempts",
	RefArtifact:    "artifacts",
	RefMessage:     "messages",
	RefApproval:    "approvals",
	RefWorkspace:   "workspaces",
}

// Ref is one id that must belong to the project.
type Ref struct {
	Kind RefKind
	ID   string
}

type queryer interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// CheckScope verifies the project exists and every ref belongs to it.
// Every binding and engine entry point that accepts ids calls this (or
// CheckScopeTx inside a write) before touching the data.
func (db *DB) CheckScope(ctx context.Context, projectID string, refs ...Ref) error {
	return checkScope(ctx, db.sql, projectID, refs)
}

// CheckScopeTx is CheckScope inside a write transaction, so the check
// and the write see the same state.
func CheckScopeTx(ctx context.Context, tx *sql.Tx, projectID string, refs ...Ref) error {
	return checkScope(ctx, tx, projectID, refs)
}

func checkScope(ctx context.Context, q queryer, projectID string, refs []Ref) error {
	if projectID == "" {
		return fmt.Errorf("project: %w", ErrNotInProject)
	}
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM projects WHERE id = ?`, projectID).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("project %q: %w", projectID, ErrNotInProject)
	}
	if err != nil {
		return err
	}
	for _, r := range refs {
		table, ok := scopeTables[r.Kind]
		if !ok {
			return fmt.Errorf("unknown ref kind %q", r.Kind)
		}
		if r.ID == "" {
			return fmt.Errorf("%s: empty id: %w", r.Kind, ErrNotInProject)
		}
		err := q.QueryRowContext(ctx, `SELECT 1 FROM `+table+` WHERE id = ? AND project_id = ?`, r.ID, projectID).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%s %q: %w", r.Kind, r.ID, ErrNotInProject)
		}
		if err != nil {
			return err
		}
	}
	return nil
}
