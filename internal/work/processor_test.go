package work_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
	"agent-office/internal/work"
)

type fixture struct {
	db      *storage.DB
	path    string
	project domain.Project
	dev     domain.Department
	fin     domain.Department
}

func open(t *testing.T, path string) *storage.DB {
	t.Helper()
	db, err := storage.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// newFixture is a service whose development and finance departments
// both subscribe to new work.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	f := &fixture{path: filepath.Join(t.TempDir(), "work.db")}
	f.db = open(t, f.path)
	t.Cleanup(func() { f.db.Close() })
	f.project, _ = f.db.CreateProject(ctx, domain.Project{Name: "캠페인"})
	for _, d := range []*domain.Department{&f.dev, &f.fin} {
		name := map[*domain.Department]string{&f.dev: "개발팀", &f.fin: "재무팀"}[d]
		var err error
		if *d, err = f.db.SaveDepartment(ctx, domain.Department{ProjectID: f.project.ID, Name: name}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.db.SaveSubscription(ctx, domain.Subscription{ProjectID: f.project.ID, DepartmentID: d.ID,
			EventKinds: []string{domain.EventWorkCreated}, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *fixture) newWork(t *testing.T, title string) {
	t.Helper()
	if _, err := f.db.CreateWorkItem(context.Background(), domain.WorkItem{ProjectID: f.project.ID, Title: title}); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) statuses(t *testing.T) map[string]int {
	t.Helper()
	all, err := f.db.Deliveries(context.Background(), f.project.ID, storage.DeliveryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]int{}
	for _, d := range all {
		out[d.Status]++
	}
	return out
}

func TestDrainHandlesEachDeliveryOnce(t *testing.T) {
	f := newFixture(t)
	f.newWork(t, "기획안")
	f.newWork(t, "예산안")
	var calls atomic.Int32
	count := func(ctx context.Context, d storage.Delivery) (work.Result, error) {
		calls.Add(1)
		return work.Acknowledge(ctx, d)
	}
	// Two processors woken at once (a duplicate notification) still
	// handle each delivery once.
	p1, p2 := work.New(f.db, count), work.New(f.db, count)
	var wg sync.WaitGroup
	for _, p := range []*work.Processor{p1, p2} {
		wg.Add(1)
		go func() { defer wg.Done(); p.Drain(context.Background()) }()
	}
	wg.Wait()
	if calls.Load() != 4 {
		t.Fatalf("handler calls = %d, want 4", calls.Load())
	}
	if got := f.statuses(t); got[domain.DeliveryDone] != 4 || len(got) != 1 {
		t.Fatalf("statuses = %v", got)
	}
	if n, _ := p1.Drain(context.Background()); n != 0 {
		t.Fatalf("drained again: %d", n)
	}
}

func TestRestartFinishesInterruptedDelivery(t *testing.T) {
	f := newFixture(t)
	f.newWork(t, "기획안")
	// The first processor stops in the middle of a delivery.
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	stuck := func(ctx context.Context, d storage.Delivery) (work.Result, error) {
		close(started)
		<-ctx.Done()
		return work.Result{}, ctx.Err()
	}
	done := make(chan error, 1)
	go func() { _, err := work.New(f.db, stuck).Drain(ctx); done <- err }()
	<-started
	cancel()
	<-done
	if got := f.statuses(t); got[domain.DeliveryProcessing] != 1 || got[domain.DeliveryPending] != 1 {
		t.Fatalf("after stop = %v", got)
	}

	// Reopen the database as a restarted app would; Run recovers first.
	f.db.Close()
	f.db = open(t, f.path)
	var calls atomic.Int32
	p := work.New(f.db, func(ctx context.Context, d storage.Delivery) (work.Result, error) {
		calls.Add(1)
		return work.Acknowledge(ctx, d)
	})
	runCtx, stop := context.WithCancel(context.Background())
	defer stop()
	go p.Run(runCtx)
	deadline := time.Now().Add(10 * time.Second)
	for f.statuses(t)[domain.DeliveryDone] != 2 {
		if time.Now().After(deadline) {
			t.Fatalf("not finished: %v", f.statuses(t))
		}
		time.Sleep(20 * time.Millisecond)
	}
	if calls.Load() != 2 {
		t.Fatalf("handler calls after restart = %d, want 2", calls.Load())
	}
	// New events are picked up when the processor is woken.
	f.newWork(t, "새 업무")
	p.Wake()
	for f.statuses(t)[domain.DeliveryDone] != 4 {
		if time.Now().After(deadline) {
			t.Fatalf("new event not handled: %v", f.statuses(t))
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestFailedDeliveryCanBeRetried(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	if _, err := f.db.RequestDepartmentCheck(ctx, f.project.ID, f.dev.ID, ""); err != nil {
		t.Fatal(err)
	}
	fail := true
	p := work.New(f.db, func(ctx context.Context, d storage.Delivery) (work.Result, error) {
		if fail {
			return work.Result{}, errors.New("대표 담당자의 AI 연결이 없습니다")
		}
		return work.Acknowledge(ctx, d)
	})
	p.Drain(ctx)
	all, _ := f.db.Deliveries(ctx, f.project.ID, storage.DeliveryFilter{})
	if all[0].Status != domain.DeliveryFailed || all[0].Error != "대표 담당자의 AI 연결이 없습니다" {
		t.Fatalf("failed = %+v", all[0])
	}
	// Failures are not retried on their own.
	if n, _ := p.Drain(ctx); n != 0 {
		t.Fatalf("retried by itself: %d", n)
	}
	fail = false
	if err := f.db.RetryDelivery(ctx, f.project.ID, all[0].ID); err != nil {
		t.Fatal(err)
	}
	p.Drain(ctx)
	all, _ = f.db.Deliveries(ctx, f.project.ID, storage.DeliveryFilter{})
	if all[0].Status != domain.DeliveryDone || all[0].Error != "" || all[0].Note == "" {
		t.Fatalf("after retry = %+v", all[0])
	}
}

func TestHandlerStatusIsChecked(t *testing.T) {
	f := newFixture(t)
	f.newWork(t, "기획안")
	work.New(f.db, func(context.Context, storage.Delivery) (work.Result, error) {
		return work.Result{Status: domain.DeliveryPending}, nil // not an outcome
	}).Drain(context.Background())
	if got := f.statuses(t); got[domain.DeliveryFailed] != 2 {
		t.Fatalf("statuses = %v", got)
	}
}
