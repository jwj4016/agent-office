// Package work processes department deliveries: events of service
// operations that a department should look at (plan §4.7). Deliveries
// are stored with their event, so the processor only has to find pending
// ones, claim each once and record what happened. It never runs inside
// the storage subscriber callback, which only wakes it.
package work

import (
	"context"
	"errors"
	"log"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// Result is what handling one delivery produced. Status processing means
// the delivery was handed to an AI job that will finish it; held keeps
// it for a person with Note as the reason.
type Result struct {
	Status string // done, skipped, waiting_answer, held or processing
	Note   string
}

// Handler decides what a department does with one delivery. An error
// marks the delivery failed with the error as its reason.
type Handler func(ctx context.Context, d storage.Delivery) (Result, error)

// Acknowledge records the delivery without asking anyone. Judge uses it
// for notifications.
func Acknowledge(context.Context, storage.Delivery) (Result, error) {
	return Result{Status: domain.DeliveryDone, Note: "전달을 기록했습니다."}, nil
}

// Processor works through pending deliveries one at a time.
type Processor struct {
	db     *storage.DB
	handle Handler
	wake   chan struct{}
	tick   time.Duration
}

func New(db *storage.DB, handle Handler) *Processor {
	return &Processor{db: db, handle: handle, wake: make(chan struct{}, 1), tick: 2 * time.Second}
}

// Wake asks the processor to look for pending deliveries soon. It never
// blocks, so it is safe in the storage subscriber callback.
func (p *Processor) Wake() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// Run returns deliveries left processing by a crash to the queue, then
// handles pending ones until ctx ends. A periodic tick covers a missed
// wake.
func (p *Processor) Run(ctx context.Context) error {
	if n, err := p.db.RecoverDeliveries(ctx); err != nil {
		return err
	} else if n > 0 {
		log.Printf("work: %d interrupted deliveries back in the queue", n)
	}
	tick := time.NewTicker(p.tick)
	defer tick.Stop()
	for {
		if _, err := p.Drain(ctx); err != nil && ctx.Err() == nil {
			log.Printf("work: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-p.wake:
		case <-tick.C:
		}
	}
}

// Drain handles pending deliveries until none are left and reports how
// many it handled. A delivery someone else claimed first is skipped.
func (p *Processor) Drain(ctx context.Context) (int, error) {
	handled := 0
	for ctx.Err() == nil {
		batch, err := p.db.PendingDeliveries(ctx, 20)
		if err != nil {
			return handled, err
		}
		if len(batch) == 0 {
			return handled, nil
		}
		for _, d := range batch {
			ok, err := p.db.ClaimDelivery(ctx, d.ID)
			if err != nil {
				return handled, err
			}
			if !ok {
				continue
			}
			if err := p.finish(ctx, d); err != nil {
				return handled, err
			}
			handled++
		}
	}
	return handled, ctx.Err()
}

func (p *Processor) finish(ctx context.Context, d storage.Delivery) error {
	res, err := p.handle(ctx, d)
	if ctx.Err() != nil {
		// Stopped mid-way: leave it processing; the next start puts it
		// back in the queue.
		return ctx.Err()
	}
	if err != nil {
		return p.db.FinishDelivery(ctx, d.ID, domain.DeliveryFailed, "", err.Error())
	}
	if res.Status == "" {
		res.Status = domain.DeliveryDone
	}
	if res.Status == domain.DeliveryProcessing {
		return nil // handed to an AI job, which finishes the delivery
	}
	err = p.db.FinishDelivery(ctx, d.ID, res.Status, res.Note, "")
	if errors.Is(err, storage.ErrInvalid) { // the handler returned a status it may not set
		return p.db.FinishDelivery(ctx, d.ID, domain.DeliveryFailed, "", err.Error())
	}
	return err
}
