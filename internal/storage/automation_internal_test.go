package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"agent-office/internal/domain"
)

// Automatic processing stops registering work when a chain gets too deep
// or has already registered its share of work (plan §4.7).
func TestChainHoldLimits(t *testing.T) {
	db := openTemp(t, filepath.Join(t.TempDir(), "chain.db"))
	defer db.Close()
	ctx := context.Background()
	p, _ := db.CreateProject(ctx, domain.Project{Name: "캠페인"})
	emit := func(kind string, ev WorkEvent) int64 {
		var seq int64
		if _, err := db.Change(ctx, func(c *Change) error {
			rec, err := c.EmitEvent(p.ID, "", "", kind, ev)
			seq = rec.Sequence
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return seq
	}
	root := emit(domain.EventWorkCollabRequested, WorkEvent{})
	if hold, err := db.ChainHold(ctx, root); err != nil || hold != "" {
		t.Fatalf("a fresh chain is held: %q %v", hold, err)
	}
	deep := emit(domain.EventDocumentShared, WorkEvent{Automated: true, ChainRootSequence: &root, ChainDepth: domain.DefaultChainDepth})
	if hold, _ := db.ChainHold(ctx, deep); !strings.Contains(hold, "단계") {
		t.Fatalf("depth hold = %q", hold)
	}
	for i := 0; i < domain.DefaultChainWork; i++ {
		ev := WorkEvent{}
		if err := continueChainTx(ctx, db.sql, &ev, root); err != nil {
			t.Fatal(err)
		}
		emit(domain.EventWorkCreated, ev)
	}
	if hold, _ := db.ChainHold(ctx, root); !strings.Contains(hold, "한도") {
		t.Fatalf("work-count hold = %q", hold)
	}
	other := emit(domain.EventWorkCollabRequested, WorkEvent{})
	if hold, _ := db.ChainHold(ctx, other); hold != "" {
		t.Fatalf("another chain is held: %q", hold)
	}
}
