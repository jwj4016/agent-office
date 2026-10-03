package storage

import (
	"context"
	"errors"
	"sync"
	"testing"
)

func TestChangeCommitsStateAndEventsTogether(t *testing.T) {
	f := newSchemaFixture(t)
	ctx := context.Background()
	var got []EventRecord
	unsub := f.db.Subscribe(func(evs []EventRecord) { got = append(got, evs...) })
	defer unsub()

	evs, err := f.db.Change(ctx, func(c *Change) error {
		if _, err := c.Tx.Exec(`UPDATE step_attempts SET status = 'succeeded' WHERE id = 'sa-game'`); err != nil {
			return err
		}
		if err := c.Emit("game", "run-game", "sa-game", "step.succeeded", map[string]string{"stepId": "plan"}); err != nil {
			return err
		}
		return c.Emit("game", "run-game", "", "run.progress", map[string]int{"done": 1})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 2 || len(got) != 2 || got[0].Sequence >= got[1].Sequence || got[0].Kind != "step.succeeded" {
		t.Fatalf("returned %+v, published %+v", evs, got)
	}
	stored, _ := f.db.EventsAfter(ctx, "game", 0, 10)
	if len(stored) != 2 || stored[1].RunID != "run-game" || stored[1].StepAttemptID != "" {
		t.Fatalf("stored = %+v", stored)
	}
	if other, _ := f.db.EventsAfter(ctx, "estate", 0, 10); len(other) != 0 {
		t.Fatalf("events leaked to another project: %+v", other)
	}
}

func TestChangeRollbackStoresAndPublishesNothing(t *testing.T) {
	f := newSchemaFixture(t)
	ctx := context.Background()
	published := 0
	f.db.Subscribe(func(evs []EventRecord) { published += len(evs) })

	boom := errors.New("boom")
	_, err := f.db.Change(ctx, func(c *Change) error {
		c.Tx.Exec(`UPDATE step_attempts SET status = 'succeeded' WHERE id = 'sa-game'`)
		c.Emit("game", "run-game", "sa-game", "step.succeeded", nil)
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatal(err)
	}
	var status string
	f.db.Read().QueryRow(`SELECT status FROM step_attempts WHERE id = 'sa-game'`).Scan(&status)
	stored, _ := f.db.EventsAfter(ctx, "game", 0, 10)
	if status != "running" || len(stored) != 0 || published != 0 {
		t.Fatalf("status=%s stored=%d published=%d", status, len(stored), published)
	}
}

func TestEmitRejectsCrossProjectEvent(t *testing.T) {
	f := newSchemaFixture(t)
	_, err := f.db.Change(context.Background(), func(c *Change) error {
		return c.Emit("estate", "run-game", "", "x", nil)
	})
	if err == nil {
		t.Fatal("event for another project's run accepted")
	}
}

func TestSubscribersSeeCommitOrderUnderConcurrency(t *testing.T) {
	f := newSchemaFixture(t)
	var mu sync.Mutex
	var seqs []int64
	f.db.Subscribe(func(evs []EventRecord) {
		mu.Lock()
		defer mu.Unlock()
		for _, ev := range evs {
			seqs = append(seqs, ev.Sequence)
		}
	})
	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f.db.Change(context.Background(), func(c *Change) error {
				return c.Emit("game", "run-game", "", "tick", nil)
			})
		}()
	}
	wg.Wait()
	if len(seqs) != 40 {
		t.Fatalf("got %d events", len(seqs))
	}
	for i := 1; i < len(seqs); i++ {
		if seqs[i] <= seqs[i-1] {
			t.Fatalf("published out of order at %d: %v", i, seqs)
		}
	}
}

func TestNewIDIsPrefixedAndUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := NewID("prj")
		if seen[id] || len(id) != len("prj-")+20 {
			t.Fatalf("bad id %q", id)
		}
		seen[id] = true
	}
}
