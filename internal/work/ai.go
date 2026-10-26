package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// Kind turns one kind of AI job into a provider call and checks what
// came back (plan §4.7). Prepare errors fail the job without a call.
type Kind interface {
	Prepare(ctx context.Context, job storage.AIJob) (design.JSONCall, error)
	Accept(ctx context.Context, job storage.AIJob, raw []byte) (json.RawMessage, error)
}

// Completer is a Kind that stores its result with its own records in the
// same transaction that makes the job ready (a judgment and its
// proposals, for example).
type Completer interface {
	Complete(ctx context.Context, job storage.AIJob, result json.RawMessage) error
}

// Caller makes one structured call; tests replace it.
type Caller func(ctx context.Context, prov providers.Provider, c design.JSONCall) ([]byte, error)

// AIConfig wires the AI job runner to the app.
type AIConfig struct {
	DB        *storage.DB
	Providers func(domain.ProviderConnection) (providers.Provider, error)
	// Hold returns why new AI work of a service must wait (its budget),
	// or "".
	Hold  func(ctx context.Context, projectID string) (string, error)
	Kinds map[string]Kind
	Call  Caller // design.RunJSONCall when nil
}

// AI runs AI jobs outside workflow runs in two lanes: work a person waits
// for (interactive) and work nobody waits for (background). Each lane
// handles one job at a time, so at most two calls run at once, and a
// busy background lane never delays an interactive job.
type AI struct {
	cfg  AIConfig
	wake map[string]chan struct{}
	tick time.Duration
}

var lanes = []string{storage.LaneInteractive, storage.LaneBackground}

func NewAI(cfg AIConfig) *AI {
	if cfg.Call == nil {
		cfg.Call = design.RunJSONCall
	}
	a := &AI{cfg: cfg, wake: map[string]chan struct{}{}, tick: 2 * time.Second}
	for _, l := range lanes {
		a.wake[l] = make(chan struct{}, 1)
	}
	return a
}

// Wake asks a lane to look for pending jobs; it never blocks.
func (a *AI) Wake(lane string) {
	select {
	case a.wake[lane] <- struct{}{}:
	default:
	}
}

// Run recovers jobs cut off by a previous stop, then serves both lanes
// until ctx ends.
func (a *AI) Run(ctx context.Context) error {
	if err := a.cfg.DB.RecoverAIJobs(ctx); err != nil {
		return err
	}
	var wg sync.WaitGroup
	for _, l := range lanes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tick := time.NewTicker(a.tick)
			defer tick.Stop()
			for {
				if _, err := a.Drain(ctx, l); err != nil && ctx.Err() == nil {
					log.Printf("work ai %s: %v", l, err)
				}
				select {
				case <-ctx.Done():
					return
				case <-a.wake[l]:
				case <-tick.C:
				}
			}
		}()
	}
	wg.Wait()
	return nil
}

// Drain handles a lane's pending jobs until none are left.
func (a *AI) Drain(ctx context.Context, lane string) (int, error) {
	n := 0
	for ctx.Err() == nil {
		job, ok, err := a.cfg.DB.NextAIJob(ctx, lane)
		if err != nil || !ok {
			return n, err
		}
		claimed, err := a.cfg.DB.ClaimAIJob(ctx, job.ID)
		if err != nil {
			return n, err
		}
		if !claimed {
			continue
		}
		if err := a.process(ctx, job); err != nil {
			return n, err
		}
		n++
	}
	return n, ctx.Err()
}

// process runs one claimed job: service, budget and connection checks, one
// provider call recorded as an attempt, then ready, a retry or failure.
func (a *AI) process(ctx context.Context, job storage.AIJob) error {
	db := a.cfg.DB
	fail := func(err error) error { return db.FinishAIJob(ctx, job.ID, storage.JobFailed, nil, err.Error(), "") }
	if p, err := db.Project(ctx, job.ProjectID); err != nil {
		return err
	} else if p.Status == "archived" {
		return db.FinishAIJob(ctx, job.ID, storage.JobHeld, nil, "", "보관된 서비스여서 AI를 부르지 않았습니다")
	}
	if a.cfg.Hold != nil {
		hold, err := a.cfg.Hold(ctx, job.ProjectID)
		if err != nil {
			return err
		}
		if hold != "" {
			return db.FinishAIJob(ctx, job.ID, storage.JobHeld, nil, "", hold)
		}
	}
	kind, ok := a.cfg.Kinds[job.Kind]
	if !ok {
		return fail(fmt.Errorf("이 앱이 처리하지 못하는 AI 작업 종류입니다: %s", job.Kind))
	}
	conn, err := db.Connection(ctx, job.ConnectionID)
	if err != nil || !(conn.Provider == "test" || conn.Verified()) {
		return fail(errors.New("담당자의 AI 연결이 없거나 실제 호출 시험을 통과하지 않았습니다"))
	}
	prov, err := a.cfg.Providers(conn)
	if err != nil {
		return fail(err)
	}
	call, err := kind.Prepare(ctx, job)
	if err != nil {
		return fail(err)
	}
	attempt, err := db.StartAIAttempt(ctx, job)
	if err != nil {
		return err
	}
	if call.Timeout == 0 {
		call.Timeout = 10 * time.Minute
	}
	call.Model = job.Model
	call.OnEvent = func(ev providers.Event) error {
		if ev.Kind == providers.KindUsage {
			return db.RecordAIUsage(context.Background(), attempt, ev.Payload)
		}
		return nil
	}
	raw, err := a.cfg.Call(ctx, prov, call)
	if ctx.Err() != nil {
		return ctx.Err() // stopping: recovery ends the attempt on the next start
	}
	var result json.RawMessage
	if err == nil {
		result, err = kind.Accept(ctx, job, raw)
	}
	if err != nil {
		if ferr := db.FinishAIAttempt(ctx, attempt, "failed", err.Error()); ferr != nil {
			return ferr
		}
		if job.Attempts+1 < job.MaxAttempts {
			return db.FinishAIJob(ctx, job.ID, storage.JobPending, nil, "", "") // automatic retry
		}
		return fail(err)
	}
	if err := db.FinishAIAttempt(ctx, attempt, "succeeded", ""); err != nil {
		return err
	}
	if c, ok := kind.(Completer); ok {
		return c.Complete(ctx, job, result)
	}
	return db.FinishAIJob(ctx, job.ID, storage.JobReady, result, "", "")
}
