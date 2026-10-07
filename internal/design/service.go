package design

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/storage"
)

// Design request statuses.
const (
	StatusDrafting  = "drafting"
	StatusReady     = "ready"
	StatusFailed    = "failed"
	StatusApplied   = "applied"
	StatusDiscarded = "discarded"
)

var (
	ErrNotReady = errors.New("적용할 수 있는 상태의 설계안이 아닙니다")
	ErrInvalid  = errors.New("invalid design request")
)

// Service runs the designer and applies its proposals.
type Service struct {
	DB        *storage.DB
	Engine    *engine.Engine
	Providers engine.ProviderFor
	// Connections lists connections the designer may propose (no secrets).
	Connections func(ctx context.Context) ([]ConnectionInfo, error)
	Logf        func(string, ...any)

	applyMu sync.Mutex
	wg      sync.WaitGroup
}

// StoredInput is what a design record keeps as its input.
type StoredInput struct {
	Request
	Model  string        `json:"model"`
	Budget engine.Budget `json:"budget"` // for a new service
}

func (s *Service) logf(format string, args ...any) {
	if s.Logf != nil {
		s.Logf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

// Wait blocks until running designers finish (tests, shutdown).
func (s *Service) Wait() { s.wg.Wait() }

// Start records a design request and runs the designer in the
// background. The record moves to ready or failed.
func (s *Service) Start(ctx context.Context, in StoredInput, connectionID string) (storage.DesignRecord, error) {
	in.Goal = strings.TrimSpace(in.Goal)
	if in.Goal == "" {
		return storage.DesignRecord{}, fmt.Errorf("%w: 목표를 입력하세요", ErrInvalid)
	}
	if in.Mode != "review" && in.Mode != "auto" {
		return storage.DesignRecord{}, fmt.Errorf("%w: 모드는 review 또는 auto입니다", ErrInvalid)
	}
	var tasks []string
	for _, t := range in.HumanTasks {
		if t = strings.TrimSpace(t); t != "" {
			tasks = append(tasks, t)
		}
	}
	in.HumanTasks = tasks
	if in.ProjectID != "" {
		p, err := s.DB.Project(ctx, in.ProjectID)
		if err != nil {
			return storage.DesignRecord{}, err
		}
		if p.Status != "active" {
			return storage.DesignRecord{}, storage.ErrArchived
		}
	}
	conn, err := s.DB.Connection(ctx, connectionID)
	if err != nil {
		return storage.DesignRecord{}, err
	}
	if !(conn.Provider == "test" || conn.Verified()) {
		return storage.DesignRecord{}, fmt.Errorf("%w: 설계에 쓸 연결이 실제 호출 시험을 통과하지 않았습니다", ErrInvalid)
	}
	raw, _ := json.Marshal(in)
	rec, err := s.DB.CreateDesign(ctx, storage.DesignRecord{ProjectID: in.ProjectID, Goal: in.Goal, Input: raw, Mode: in.Mode, ConnectionID: connectionID})
	if err != nil {
		return rec, err
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.draft(context.Background(), rec.ID, in, conn)
	}()
	return rec, nil
}

func (s *Service) context(ctx context.Context, in StoredInput) (Context, *domain.Project, error) {
	c := Context{HumanTasks: in.HumanTasks}
	var err error
	if c.Roles, err = s.DB.Roles(ctx); err != nil {
		return c, nil, err
	}
	if c.Connections, err = s.Connections(ctx); err != nil {
		return c, nil, err
	}
	if in.ProjectID == "" {
		return c, nil, nil
	}
	p, err := s.DB.Project(ctx, in.ProjectID)
	if err != nil {
		return c, nil, err
	}
	c.Assignments, err = s.DB.Assignments(ctx, in.ProjectID)
	return c, &p, err
}

func (s *Service) draft(ctx context.Context, id string, in StoredInput, conn domain.ProviderConnection) {
	fail := func(err error) {
		if _, uerr := s.DB.UpdateDesign(ctx, id, StatusFailed, nil, nil, nil, err.Error(), "", StatusDrafting); uerr != nil {
			s.logf("design %s: %v", id, uerr)
		}
	}
	c, project, err := s.context(ctx, in)
	if err != nil {
		fail(err)
		return
	}
	prov, err := s.Providers(conn)
	if err != nil {
		fail(err)
		return
	}
	raw, err := Run(ctx, prov, in.Model, BuildPrompt(in.Request, c, project))
	if err != nil {
		fail(err)
		return
	}
	p, err := Parse(raw)
	if err != nil {
		fail(err)
		return
	}
	res := Check(p, c, in.ProjectID == "")
	proposal, _ := json.Marshal(res.Proposal)
	check, _ := json.Marshal(res)
	if _, err := s.DB.UpdateDesign(ctx, id, StatusReady, proposal, check, nil, "", "", StatusDrafting); err != nil {
		s.logf("design %s: %v", id, err)
	}
}

// Recheck re-validates a ready proposal against the current organization
// and connections (they may have changed since drafting).
func (s *Service) Recheck(ctx context.Context, id string) (Result, StoredInput, error) {
	rec, err := s.DB.Design(ctx, id)
	if err != nil {
		return Result{}, StoredInput{}, err
	}
	var in StoredInput
	json.Unmarshal(rec.Input, &in)
	if rec.Status != StatusReady {
		return Result{}, in, ErrNotReady
	}
	p, err := Parse(rec.Proposal)
	if err != nil {
		return Result{}, in, err
	}
	c, _, err := s.context(ctx, in)
	if err != nil {
		return Result{}, in, err
	}
	return Check(p, c, in.ProjectID == ""), in, nil
}

// ApplyResult reports what applying did.
type ApplyResult struct {
	Applied *storage.DesignApplied `json:"applied"`
	// Issues explain why applying (or, in auto mode, starting) was refused.
	Issues  []domain.Issue `json:"issues"`
	RunID   string         `json:"runId,omitempty"`
	Version int            `json:"version,omitempty"`
}

// Apply writes a ready proposal. In auto mode the pre-approved scope is
// checked first; only then is the design applied, versioned and started.
// Human steps and approvals still wait for the person.
func (s *Service) Apply(ctx context.Context, id string, auto bool) (ApplyResult, error) {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	res, in, err := s.Recheck(ctx, id)
	if err != nil {
		return ApplyResult{}, err
	}
	if !res.CanApply {
		return ApplyResult{Issues: res.Issues}, nil
	}
	if auto {
		if in.Mode != "auto" {
			return ApplyResult{}, fmt.Errorf("%w: auto 모드로 요청한 설계가 아닙니다", ErrInvalid)
		}
		policy, hasBudget := in.AutoPolicy, in.Budget.MaxTokens != nil || in.Budget.MaxCostUSD != nil
		if in.ProjectID != "" {
			p, err := s.DB.Project(ctx, in.ProjectID)
			if err != nil {
				return ApplyResult{}, err
			}
			if p.Mode != domain.ModeAuto {
				return ApplyResult{Issues: []domain.Issue{{Code: "auto_mode_off", Message: "이 서비스는 자동 구성·실행 모드가 아닙니다", Severity: domain.SevRun}}}, nil
			}
			policy = AutoPolicy{}
			json.Unmarshal(p.AutoPolicy, &policy)
			var b engine.Budget
			json.Unmarshal(p.Budget, &b)
			hasBudget = b.MaxTokens != nil || b.MaxCostUSD != nil
		}
		if issues := AutoCheck(res, policy, hasBudget); len(issues) > 0 {
			return ApplyResult{Issues: issues}, nil
		}
	}

	apply := storage.DesignApply{ProjectID: in.ProjectID, ExistingRoles: map[string]string{}, Workflow: res.Proposal.Workflow}
	if in.ProjectID == "" {
		budget, _ := json.Marshal(in.Budget)
		mode := domain.ModeReview
		if in.Mode == "auto" {
			mode = domain.ModeAuto
		}
		pp := res.Proposal.Project
		apply.NewProject = &domain.Project{Name: firstNonEmpty(strings.TrimSpace(in.ProjectName), pp.Name), Goal: firstNonEmpty(pp.Goal, in.Goal),
			Instructions: pp.Instructions, Budget: budget, Mode: mode}
		apply.AutoPolicy, _ = json.Marshal(in.AutoPolicy)
	}
	for _, r := range res.Diff.ReuseRoles {
		apply.ExistingRoles[r.Ref] = r.ID
	}
	for _, r := range res.Diff.NewRoles {
		apply.NewRoles = append(apply.NewRoles, storage.DesignRole{Ref: r.Ref, Name: r.Name, Mission: r.Mission, Instructions: r.Instructions, ParentRef: r.ParentRef})
	}
	for _, a := range res.Proposal.Assignments {
		apply.Assignments = append(apply.Assignments, storage.DesignAssignment{Ref: a.Ref, RoleRef: a.RoleRef, ActorKind: a.ActorKind,
			DisplayName: a.DisplayName, ConnectionID: a.ConnectionID, Model: a.Model, Instructions: a.Instructions})
	}
	applied, err := s.DB.ApplyDesign(ctx, apply)
	if err != nil {
		return ApplyResult{}, err
	}
	appliedJSON, _ := json.Marshal(applied)
	if ok, err := s.DB.UpdateDesign(ctx, id, StatusApplied, nil, nil, appliedJSON, "", applied.ProjectID, StatusReady); err != nil || !ok {
		return ApplyResult{Applied: &applied}, fmt.Errorf("설계 기록 갱신 실패: %v", err)
	}
	out := ApplyResult{Applied: &applied, Issues: []domain.Issue{}}
	if !auto {
		return out, nil
	}
	v, vres, err := s.DB.ConfirmVersion(ctx, applied.ProjectID, applied.WorkflowID, 1)
	if err != nil {
		out.Issues = vres.Issues
		return out, nil
	}
	out.Version = v.Number
	runID, err := s.Engine.StartRun(ctx, applied.ProjectID, v.ID)
	var nr *engine.NotRunnableError
	if errors.As(err, &nr) {
		out.Issues = nr.Issues
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.RunID = runID
	return out, nil
}

// Discard drops a draft that will not be applied.
func (s *Service) Discard(ctx context.Context, id string) error {
	ok, err := s.DB.UpdateDesign(ctx, id, StatusDiscarded, nil, nil, nil, "", "", StatusReady, StatusFailed, StatusDrafting)
	if err == nil && !ok {
		return ErrNotReady
	}
	return err
}
