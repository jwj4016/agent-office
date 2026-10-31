package work

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// Intake takes the requests a person makes to the company or a service
// (docs/plan/company-requests/plan.md §4.3). One request is decided at a
// time, oldest first: its desk is checked (set up, AI connection, budget),
// the desk's AI is called once per attempt, and its checked decision is
// stored with the request. A request whose desk is missing or whose
// budget is used up waits and is looked at again on every pass.
type Intake struct {
	cfg  IntakeConfig
	wake chan struct{}
	tick time.Duration
}

// IntakeConfig wires the intake runner to the app.
type IntakeConfig struct {
	DB        *storage.DB
	Engine    *engine.Engine
	Providers func(domain.ProviderConnection) (providers.Provider, error)
	// Hold says why new AI work of a service must wait (its budget), or "".
	Hold func(ctx context.Context, projectID string) (string, error)
	Call Caller // design.RunJSONCall when nil
	Now  func() time.Time
}

// intakeQueueScan bounds how many waiting requests one pass looks at.
const intakeQueueScan = 50

func NewIntake(cfg IntakeConfig) *Intake {
	if cfg.Call == nil {
		cfg.Call = design.RunJSONCall
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Intake{cfg: cfg, wake: make(chan struct{}, 1), tick: 2 * time.Second}
}

// Wake asks the runner to look at the queue; it never blocks.
func (in *Intake) Wake() {
	select {
	case in.wake <- struct{}{}:
	default:
	}
}

// Run recovers requests cut off by a previous stop, then serves the queue
// until ctx ends.
func (in *Intake) Run(ctx context.Context) error {
	if err := in.cfg.DB.RecoverRequests(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	t := time.NewTicker(in.tick)
	defer t.Stop()
	for {
		if _, err := in.Drain(ctx); err != nil && ctx.Err() == nil {
			log.Printf("intake: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-in.wake:
		case <-t.C:
		}
	}
}

// Drain decides queued requests until none can go ahead.
func (in *Intake) Drain(ctx context.Context) (int, error) {
	db := in.cfg.DB
	n := 0
	for ctx.Err() == nil {
		queue, err := db.RequestQueue(ctx, intakeQueueScan)
		if err != nil {
			return n, err
		}
		took := false
		for _, r := range queue {
			desk, status, why, err := in.gate(ctx, r)
			if err != nil {
				return n, err
			}
			if why != "" {
				if err := db.WaitRequest(ctx, r.ID, status, why); err != nil {
					return n, err
				}
				continue
			}
			ok, err := db.ClaimRequest(ctx, r.ID)
			if err != nil {
				return n, err
			}
			if !ok {
				continue
			}
			r.Status = domain.ReqTriaging
			if err := in.process(ctx, r, desk); err != nil {
				return n, err
			}
			n++
			took = true
			break
		}
		if !took {
			return n, nil
		}
	}
	return n, ctx.Err()
}

// deskCall is the desk that decides a request and how to call its AI.
type deskCall struct {
	desk         domain.IntakeDesk
	conn         domain.ProviderConnection
	model        string
	assignmentID string
}

// gate checks that a request's desk can decide it now. It returns the
// waiting status and reason when it cannot.
func (in *Intake) gate(ctx context.Context, r domain.Request) (deskCall, string, string, error) {
	db := in.cfg.DB
	var dc deskCall
	if r.ProjectID != "" {
		p, err := db.Project(ctx, r.ProjectID)
		if err != nil {
			return dc, "", "", err
		}
		if p.Status == "archived" {
			return dc, domain.ReqHeld, "보관된 서비스여서 접수하지 않았습니다. 보관을 풀면 이어서 접수합니다", nil
		}
	}
	desk, ok, err := db.IntakeDesk(ctx, r.ProjectID)
	if err != nil {
		return dc, "", "", err
	}
	where := "회사 설정"
	if r.ProjectID != "" {
		where = "서비스의 ‘부서·담당자’ 화면"
	}
	if !ok {
		return dc, domain.ReqWaitingDesk, fmt.Sprintf("접수 담당이 없습니다. %s에서 접수 담당을 정하면 이어서 접수합니다", where), nil
	}
	if !desk.Enabled {
		return dc, domain.ReqWaitingDesk, fmt.Sprintf("접수 담당이 ‘사용 안 함’입니다. %s에서 켜면 이어서 접수합니다", where), nil
	}
	dc.desk = desk
	connID, model := desk.ConnectionID, desk.Model
	if r.ProjectID != "" {
		var a domain.Assignment
		found := false
		list, err := db.Assignments(ctx, r.ProjectID)
		if err != nil {
			return dc, "", "", err
		}
		for _, x := range list {
			if x.ID == desk.AssignmentID {
				a, found = x, true
			}
		}
		if !found || a.ActorKind != domain.ActorAI {
			return dc, domain.ReqWaitingDesk, "접수 담당으로 정한 AI 담당자가 없어졌거나 사람으로 바뀌었습니다. 접수 담당을 다시 정하세요", nil
		}
		connID, model, dc.assignmentID = a.ConnectionID, a.Model, a.ID
	}
	if connID == "" {
		return dc, domain.ReqWaitingDesk, fmt.Sprintf("접수 담당에 AI 연결이 없습니다. %s에서 연결을 정하세요", where), nil
	}
	conn, err := db.Connection(ctx, connID)
	if err != nil || !(conn.Provider == "test" || conn.Verified()) {
		return dc, domain.ReqWaitingDesk, "접수 담당의 AI 연결이 없거나 실제 호출 시험을 통과하지 않았습니다. 연결 화면에서 확인하세요", nil
	}
	dc.conn, dc.model = conn, model
	if r.ProjectID != "" {
		if in.cfg.Hold != nil {
			hold, err := in.cfg.Hold(ctx, r.ProjectID)
			if err != nil {
				return dc, "", "", err
			}
			if hold != "" {
				return dc, domain.ReqHeld, hold, nil
			}
		}
	} else if limit := desk.Budget.MonthlyCostUSD; limit != nil {
		now := in.cfg.Now()
		u, err := db.CompanyIntakeUsage(ctx, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()))
		if err != nil {
			return dc, "", "", err
		}
		if u.CostUSD >= *limit {
			return dc, domain.ReqHeld, fmt.Sprintf("회사 접수의 이번 달 예산 $%.2f 중 $%.4f(보고된 비용)를 써서 접수를 보류했습니다. 회사 설정에서 예산을 조정하세요", *limit, u.CostUSD), nil
		}
	}
	return dc, "", "", nil
}

// process makes one call for a claimed request and stores what came of
// it: the decision, or a failed attempt that is retried automatically
// until the round's attempts are used.
func (in *Intake) process(ctx context.Context, r domain.Request, dc deskCall) error {
	db := in.cfg.DB
	call, facts, err := in.prepare(ctx, r, dc)
	if err != nil {
		return db.FailRequest(ctx, r.ID, err.Error())
	}
	prov, err := in.cfg.Providers(dc.conn)
	if err != nil {
		return db.FailRequest(ctx, r.ID, err.Error())
	}
	attempt, err := db.StartRequestAttempt(ctx, r, dc.conn.ID, dc.model)
	if err != nil {
		return err
	}
	call.Model = dc.model
	call.OnEvent = func(ev providers.Event) error {
		if ev.Kind == providers.KindUsage {
			return db.RecordRequestUsage(context.Background(), attempt, ev.Payload)
		}
		return nil
	}
	raw, err := in.cfg.Call(ctx, prov, call)
	if ctx.Err() != nil {
		return ctx.Err() // stopping: recovery ends the attempt on the next start
	}
	var d domain.IntakeDecision
	if err == nil {
		d, err = acceptIntake(r, facts, raw)
	}
	if err != nil {
		if ferr := db.FinishRequestAttempt(ctx, attempt, "failed", raw, err.Error()); ferr != nil {
			return ferr
		}
		return db.ReleaseRequest(ctx, r.ID, err.Error())
	}
	if err := db.FinishRequestAttempt(ctx, attempt, "succeeded", raw, ""); err != nil {
		return err
	}
	auto := false
	if r.ProjectID != "" {
		p, err := db.Project(ctx, r.ProjectID)
		if err != nil {
			return err
		}
		auto = p.Mode == domain.ModeAuto
	}
	if err := db.ApplyIntakeDecision(ctx, r.ID, d, auto); err != nil {
		if !errors.Is(err, storage.ErrInvalid) && !errors.Is(err, storage.ErrNotInProject) && !errors.Is(err, storage.ErrArchived) {
			return err
		}
		return db.ReleaseRequest(ctx, r.ID, "접수 판단을 저장하지 못했습니다: "+err.Error())
	}
	return nil
}
