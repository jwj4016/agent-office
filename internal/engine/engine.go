package engine

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"sync"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

var (
	// ErrStale rejects a decision or submission for an attempt, approval
	// or generation that is no longer current (spec §7.3 stale_request).
	ErrStale = errors.New("stale_request: this item is no longer current")
	// ErrNotRunnable means a run cannot start; Issues explain why.
	ErrNotRunnable = errors.New("workflow version cannot run")
	// ErrRevisionLimit means a rework would exceed the revision limit; a
	// person must change scope, criteria or the limit (spec §7.4).
	ErrRevisionLimit = errors.New("revision limit reached")
	ErrInvalid       = errors.New("invalid request")
	// ErrEngineRunning means another engine already owns this database.
	// Starting a second one would mark the first one's live work
	// interrupted during recovery.
	ErrEngineRunning = errors.New("another engine is already running on this database")
)

// NotRunnableError lists the issues blocking a run (T04).
type NotRunnableError struct{ Issues []domain.Issue }

func (e *NotRunnableError) Error() string { return ErrNotRunnable.Error() }
func (e *NotRunnableError) Unwrap() error { return ErrNotRunnable }

// ProviderFor returns the provider for a connection.
type ProviderFor func(conn domain.ProviderConnection) (providers.Provider, error)

type Config struct {
	DB *storage.DB
	// DataDir holds per-project work dirs and step outputs.
	DataDir   string
	Providers ProviderFor
	// MaxActive bounds concurrently running AI attempts app-wide (default 2).
	MaxActive int
	// ProjectMaxActive bounds them per project (default 2).
	ProjectMaxActive int
	// DefaultMaxRevisions applies when a node sets no limit (default 3).
	DefaultMaxRevisions int
	// ConsultTimeout bounds how long an AI may take to answer another
	// AI's question before the question goes to the person (default 5m).
	ConsultTimeout time.Duration
	// MaxConsults bounds the questions one attempt may send to other AIs
	// (default 3); more go to the person (T18).
	MaxConsults int
	// MaxPairQuestions bounds questions between the same two assignees in
	// one run, in either direction (default 3); more go to the person.
	MaxPairQuestions int
	// MeetingTurnTimeout bounds one participant's turn in a meeting
	// (default 10m).
	MeetingTurnTimeout time.Duration
	// Ephemeral receives high-volume provider events (message deltas)
	// that are shown live but not stored. Optional.
	Ephemeral func(projectID string, ev providers.Event)
	// Logf receives diagnostics. Optional.
	Logf func(format string, args ...any)
}

// Engine schedules runs. The database is the source of truth: every
// decision re-reads state inside the transaction that changes it, so a
// late provider event or a second click cannot advance a stale attempt.
type Engine struct {
	cfg Config
	db  *storage.DB

	mu     sync.Mutex // guards active and serializes scheduling passes
	active map[string]*activeAttempt
	ctx    context.Context
	wake   chan struct{}
	wg     sync.WaitGroup
	// consultSlot queues AI answers to other AIs' questions: one at a
	// time, besides the step slots (spec §8 "대기열에서 처리").
	consultSlot chan struct{}
}

// activeAttempt is an attempt with live provider work. A meeting runs
// several sessions one after another; session is the current one.
type activeAttempt struct {
	id, projectID, runID, stepID string
	generation                   int
	// questions maps a question message id to the provider request id.
	questions map[string]string
	toolReqs  map[string]string // approval id -> provider request id
	// ctx ends when the attempt is cancelled; side sessions (meeting
	// turns, answers to its questions) stop with it.
	ctx  context.Context
	stop context.CancelFunc

	mu      sync.Mutex
	session providers.Session
	stopped bool
}

func (e *Engine) newActive(id string, st *runState, stepID string, generation int) *activeAttempt {
	ctx, stop := context.WithCancel(e.ctx)
	return &activeAttempt{id: id, projectID: st.run.ProjectID, runID: st.run.ID, stepID: stepID, generation: generation,
		questions: map[string]string{}, toolReqs: map[string]string{}, ctx: ctx, stop: stop}
}

// current returns the session that should receive responses, or nil.
func (a *activeAttempt) current() providers.Session {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.session
}

// setSession makes s the current session. If the attempt was cancelled
// meanwhile, s is cancelled too and false is returned.
func (a *activeAttempt) setSession(s providers.Session) bool {
	a.mu.Lock()
	stopped := a.stopped
	if !stopped {
		a.session = s
	}
	a.mu.Unlock()
	if stopped {
		s.Cancel(context.Background())
	}
	return !stopped
}

// cancel stops the attempt's current session and any side sessions.
func (a *activeAttempt) cancel() {
	a.mu.Lock()
	a.stopped = true
	s := a.session
	a.mu.Unlock()
	a.stop()
	if s != nil {
		s.Cancel(context.Background())
	}
}

func (a *activeAttempt) isStopped() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.stopped
}

// respond forwards a person's or another AI's response to the session.
func (a *activeAttempt) respond(ctx context.Context, r providers.Response) error {
	s := a.current()
	if s == nil {
		return providers.ErrSessionClosed
	}
	return s.Respond(ctx, r)
}

func New(cfg Config) *Engine {
	if cfg.MaxActive <= 0 {
		cfg.MaxActive = 2
	}
	if cfg.ProjectMaxActive <= 0 {
		cfg.ProjectMaxActive = 2
	}
	if cfg.DefaultMaxRevisions <= 0 {
		cfg.DefaultMaxRevisions = 3
	}
	if cfg.ConsultTimeout <= 0 {
		cfg.ConsultTimeout = 5 * time.Minute
	}
	if cfg.MaxConsults <= 0 {
		cfg.MaxConsults = 3
	}
	if cfg.MaxPairQuestions <= 0 {
		cfg.MaxPairQuestions = 3
	}
	if cfg.MeetingTurnTimeout <= 0 {
		cfg.MeetingTurnTimeout = 10 * time.Minute
	}
	if cfg.Logf == nil {
		cfg.Logf = log.Printf
	}
	return &Engine{cfg: cfg, db: cfg.DB, active: map[string]*activeAttempt{}, wake: make(chan struct{}, 1), ctx: context.Background(),
		consultSlot: make(chan struct{}, 1)}
}

// Wake asks the scheduler to run a pass soon.
func (e *Engine) Wake() {
	select {
	case e.wake <- struct{}{}:
	default:
	}
}

// Run recovers interrupted work, then schedules until ctx is done. On
// exit it cancels live provider sessions and waits for them briefly.
func (e *Engine) Run(ctx context.Context) error {
	release, err := e.db.ClaimEngine()
	if err != nil {
		return ErrEngineRunning
	}
	defer release()
	e.mu.Lock()
	e.ctx = ctx
	e.mu.Unlock()
	if err := e.recover(ctx); err != nil {
		return err
	}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	e.Wake()
	for {
		select {
		case <-ctx.Done():
			e.shutdown()
			return nil
		case <-e.wake:
		case <-tick.C:
		}
		e.schedule(ctx)
	}
}

func (e *Engine) shutdown() {
	e.mu.Lock()
	for _, a := range e.active {
		a.cancel()
	}
	e.mu.Unlock()
	done := make(chan struct{})
	go func() { e.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		e.cfg.Logf("engine: provider sessions still running at shutdown")
	}
}

func (e *Engine) projectDir(projectID string) string {
	return filepath.Join(e.cfg.DataDir, "projects", projectID)
}

func (e *Engine) attemptDir(projectID, runID, attemptID string) string {
	return filepath.Join(e.projectDir(projectID), "runs", runID, "attempts", attemptID)
}

func (e *Engine) activeCounts(projectID string) (app, project int) {
	for _, a := range e.active {
		app++
		if a.projectID == projectID {
			project++
		}
	}
	return
}

func (e *Engine) lookupActive(attemptID string) *activeAttempt {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.active[attemptID]
}

// hasLiveAttempt reports whether an older attempt of the step is still
// running; a new one must not start until it has stopped (spec §7.4).
func (e *Engine) hasLiveAttempt(runID, stepID string) bool {
	for _, a := range e.active {
		if a.runID == runID && a.stepID == stepID {
			return true
		}
	}
	return false
}
