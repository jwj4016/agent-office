package work

import (
	"context"
	"log"
	"time"

	"agent-office/internal/storage"
)

// Scheduler runs departments' regular checks (plan stage 8). For each
// schedule it looks only at the latest due slot, so a slot missed while
// the app was closed runs once and older ones are skipped. A slot runs at
// most once (its key is unique), so a restart, a changed schedule or a
// second look never asks twice.
type Scheduler struct {
	db   *storage.DB
	now  func() time.Time
	wake chan struct{}
	tick time.Duration
}

// NewScheduler makes a scheduler; now is time.Now unless a test sets it.
func NewScheduler(db *storage.DB, now func() time.Time) *Scheduler {
	if now == nil {
		now = time.Now
	}
	return &Scheduler{db: db, now: now, wake: make(chan struct{}, 1), tick: 30 * time.Second}
}

// Wake asks the scheduler to look again soon, e.g. after a schedule was
// saved. It never blocks.
func (s *Scheduler) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run looks for due slots until ctx ends.
func (s *Scheduler) Run(ctx context.Context) error {
	tick := time.NewTicker(s.tick)
	defer tick.Stop()
	for {
		if _, err := s.Drain(ctx); err != nil && ctx.Err() == nil {
			log.Printf("work schedule: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-s.wake:
		case <-tick.C:
		}
	}
}

// Drain runs every schedule's latest due slot that has not run and
// reports how many ran. A schedule that cannot be read (an unknown time
// zone) is skipped.
func (s *Scheduler) Drain(ctx context.Context) (int, error) {
	list, err := s.db.DueSchedules(ctx)
	if err != nil {
		return 0, err
	}
	now, ran := s.now(), 0
	for _, sch := range list {
		due, key, err := sch.LastSlot(now)
		if err != nil {
			continue
		}
		ok, err := s.db.RunScheduleSlot(ctx, sch.ID, key, due)
		if err != nil {
			return ran, err
		}
		if ok {
			ran++
		}
	}
	return ran, nil
}
