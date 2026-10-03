package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

func TestCheckScope(t *testing.T) {
	f := newSchemaFixture(t)
	ctx := context.Background()
	ok := []Ref{{RefRun, "run-game"}, {RefStepAttempt, "sa-game"}, {RefAssignment, "a-game"}, {RefWorkflow, "wf-game"}, {RefVersion, "wv-game"}}
	if err := f.db.CheckScope(ctx, "game", ok...); err != nil {
		t.Fatalf("own refs rejected: %v", err)
	}

	cases := []struct {
		name    string
		project string
		refs    []Ref
	}{
		{"other project's run", "game", []Ref{{RefRun, "run-estate"}}},
		{"other project's attempt mixed in", "game", []Ref{{RefRun, "run-game"}, {RefStepAttempt, "sa-estate"}}},
		{"unknown id", "game", []Ref{{RefArtifact, "ar-nope"}}},
		{"empty id", "game", []Ref{{RefRun, ""}}},
		{"unknown project", "nope", nil},
		{"empty project", "", nil},
		{"sql in id", "game", []Ref{{RefRun, "run-game' OR '1'='1"}}},
	}
	for _, c := range cases {
		if err := f.db.CheckScope(ctx, c.project, c.refs...); !errors.Is(err, ErrNotInProject) {
			t.Errorf("%s: want ErrNotInProject, got %v", c.name, err)
		}
	}
	if err := f.db.CheckScope(ctx, "game", Ref{"projects; DROP TABLE runs", "x"}); err == nil || errors.Is(err, ErrNotInProject) {
		t.Errorf("unknown kind must be a programming error, got %v", err)
	}

	err := f.db.Write(ctx, func(tx *sql.Tx) error {
		return CheckScopeTx(ctx, tx, "estate", Ref{RefStepAttempt, "sa-game"})
	})
	if !errors.Is(err, ErrNotInProject) {
		t.Fatalf("tx variant: %v", err)
	}
}
