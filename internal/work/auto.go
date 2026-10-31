package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"slices"
	"strings"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/storage"
)

// AutoConfig wires the auto starter to the app.
type AutoConfig struct {
	DB *storage.DB
	// Hold returns why new AI work of a service must wait (its budget),
	// or "".
	Hold func(ctx context.Context, projectID string) (string, error)
	// Start runs a work item's saved plan and returns the run.
	Start func(ctx context.Context, projectID, workItemID string) (string, error)
	// Share shares a finished run's results as drafts continuing the
	// chain of the event at causeSeq, and says how many it shared.
	Share func(ctx context.Context, projectID, runID string, causeSeq int64) (int, error)
	// WakeAI asks an AI lane to look for jobs (a plan to write).
	WakeAI func(lane string)
}

// AutoStarter takes work made from events in auto-mode services through
// the automatic start (plan §4.6): it checks the three conditions (auto
// mode, the service's auto policy, the department's rules), has the
// department's AI write a plan when there is none, approves the start for
// exactly that plan, starts the run and, when the department allows it,
// shares the results as drafts. Anything that does not fit goes to a
// person for review with the reason.
type AutoStarter struct {
	cfg  AutoConfig
	wake chan struct{}
	tick time.Duration
}

func NewAutoStarter(cfg AutoConfig) *AutoStarter {
	return &AutoStarter{cfg: cfg, wake: make(chan struct{}, 1), tick: 2 * time.Second}
}

// Wake asks the starter to look soon; it never blocks.
func (s *AutoStarter) Wake() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Run moves auto starts along until ctx ends.
func (s *AutoStarter) Run(ctx context.Context) error {
	tick := time.NewTicker(s.tick)
	defer tick.Stop()
	for {
		if _, err := s.Drain(ctx); err != nil && ctx.Err() == nil {
			log.Printf("work auto: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-s.wake:
		case <-tick.C:
		}
	}
}

// Drain looks at every open auto start once and reports how many moved.
// Each step is safe to repeat: a start already moved by someone else is
// left alone.
func (s *AutoStarter) Drain(ctx context.Context) (int, error) {
	open, err := s.cfg.DB.OpenAutoStarts(ctx, 50)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range open {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		moved, err := s.step(ctx, a)
		if err != nil {
			log.Printf("work auto %s: %v", a.ID, err)
			continue
		}
		if moved {
			n++
		}
	}
	return n, nil
}

func (s *AutoStarter) move(ctx context.Context, a storage.AutoStart, u storage.AutoStartUpdate) (bool, error) {
	return s.cfg.DB.MoveAutoStart(ctx, a.ID, []string{a.Status}, u)
}

func (s *AutoStarter) review(ctx context.Context, a storage.AutoStart, reason string) (bool, error) {
	return s.move(ctx, a, storage.AutoStartUpdate{Status: storage.AutoReview, Reason: reason})
}

func (s *AutoStarter) step(ctx context.Context, a storage.AutoStart) (bool, error) {
	db := s.cfg.DB
	w, err := db.WorkItem(ctx, a.ProjectID, a.WorkItemID)
	if err != nil {
		return false, err
	}
	if domain.WorkClosed(w.Status) && a.Status != storage.AutoStarted {
		return s.move(ctx, a, storage.AutoStartUpdate{Status: storage.AutoCancelled, Note: "업무가 닫혀 자동으로 시작하지 않았습니다"})
	}
	switch a.Status {
	case storage.AutoPending:
		if issue, err := s.workIssue(ctx, w); err != nil || issue != "" {
			return s.reviewOrErr(ctx, a, issue, err)
		}
		plan, saved, err := db.WorkPlan(ctx, a.ProjectID, w.ID)
		if err != nil {
			return false, err
		}
		if saved && (plan.WorkflowVersionID != "" || plan.AssignmentID != "") {
			return s.approve(ctx, a, w)
		}
		p, err := planner(ctx, db, a.ProjectID, w.ID)
		if err != nil {
			return s.review(ctx, a, "계획을 쓸 AI가 없습니다: "+uiReason(err))
		}
		if issue, err := s.connectionIssue(ctx, w, p); err != nil || issue != "" {
			return s.reviewOrErr(ctx, a, issue, err)
		}
		in, err := planInputFor(ctx, db, a.ProjectID, w.ID, p.ID)
		if err != nil {
			return false, err
		}
		snap, _ := json.Marshal(in)
		// The automatic plan is keyed by the auto start, so a repeated step
		// reuses the same job.
		job, _, err := db.CreateAIJob(ctx, storage.AIJob{ProjectID: a.ProjectID, Kind: storage.JobPlan, Lane: storage.LaneBackground,
			RequestKey: "plan:" + w.ID + ":" + a.ID, WorkItemID: w.ID, AssignmentID: p.ID, ConnectionID: p.ConnectionID, Model: p.Model,
			InputSnapshot: snap, InputHash: in.hash()})
		if err != nil {
			return false, err
		}
		if s.cfg.WakeAI != nil {
			s.cfg.WakeAI(job.Lane)
		}
		return s.move(ctx, a, storage.AutoStartUpdate{Status: storage.AutoPlanning, PlanJobID: job.ID})
	case storage.AutoPlanning:
		job, err := db.AIJob(ctx, a.ProjectID, a.PlanJobID)
		if err != nil {
			return false, err
		}
		switch job.Status {
		case storage.JobReady:
			if _, err := ApplyPlan(ctx, db, a.ProjectID, job.ID); err != nil {
				return s.reviewOrErr(ctx, a, "AI가 쓴 계획을 적용하지 못했습니다: "+uiReason(err), nonInvalid(err))
			}
			return s.approve(ctx, a, w)
		case storage.JobFailed:
			return s.review(ctx, a, "AI 계획 제안이 실패했습니다: "+job.Error)
		case storage.JobHeld:
			return s.review(ctx, a, "AI 계획 제안이 보류되었습니다: "+job.HoldReason)
		}
		return false, nil
	case storage.AutoApproved:
		if issue, err := s.workIssue(ctx, w); err != nil || issue != "" {
			return s.reviewOrErr(ctx, a, issue, err)
		}
		if issue, err := s.planIssue(ctx, w); err != nil || issue != "" {
			return s.reviewOrErr(ctx, a, issue, err)
		}
		ap, err := db.WorkStartApproval(ctx, a.ProjectID, w.ID)
		if err != nil {
			return false, err
		}
		if ap.By != "auto" || !ap.Valid {
			return s.review(ctx, a, "자동 시작을 승인한 뒤 계획·완료 조건·자료·담당자가 바뀌어 승인이 무효가 되었습니다. 확인한 뒤 직접 시작하세요")
		}
		runID, err := s.cfg.Start(ctx, a.ProjectID, w.ID)
		if err != nil {
			return s.reviewOrErr(ctx, a, "자동으로 시작하지 못했습니다: "+uiReason(err), nil)
		}
		return s.move(ctx, a, storage.AutoStartUpdate{Status: storage.AutoStarted, RunID: runID, Note: "자동으로 시작했습니다"})
	case storage.AutoStarted:
		status, err := db.RunStatus(ctx, a.RunID)
		if err != nil {
			return false, err
		}
		if status == "running" || status == "waiting" || status == "paused" {
			return false, nil
		}
		note := fmt.Sprintf("실행이 끝났습니다(%s). 결과는 사람이 확인합니다", status)
		if status == "succeeded" {
			dept, err := db.Department(ctx, a.ProjectID, w.DepartmentID)
			if err != nil {
				return false, err
			}
			if domain.ParseAutomation(dept.Automation).AutoShare && s.cfg.Share != nil {
				n, err := s.cfg.Share(ctx, a.ProjectID, a.RunID, a.CauseSequence)
				switch {
				case err != nil:
					note = "실행이 끝났지만 결과를 자동으로 공유하지 못했습니다: " + uiReason(err)
				case n > 0:
					note = fmt.Sprintf("실행이 끝나 결과 %d건을 초안으로 공유했습니다. 결과는 사람이 확인합니다", n)
				}
			}
		}
		return s.move(ctx, a, storage.AutoStartUpdate{Status: storage.AutoFinished, Note: note})
	}
	return false, nil
}

// approve checks the plan against the rules and approves a start for
// exactly the current work and plan.
func (s *AutoStarter) approve(ctx context.Context, a storage.AutoStart, w domain.WorkItem) (bool, error) {
	if issue, err := s.planIssue(ctx, w); err != nil || issue != "" {
		return s.reviewOrErr(ctx, a, issue, err)
	}
	hash, err := s.cfg.DB.WorkStartHash(ctx, a.ProjectID, w.ID)
	if err != nil {
		return false, err
	}
	if err := s.cfg.DB.ApproveWorkStart(ctx, a.ProjectID, w.ID, "auto", hash); err != nil {
		return false, err
	}
	return s.move(ctx, a, storage.AutoStartUpdate{Status: storage.AutoApproved, Note: "자동 시작 조건을 모두 만족해 시작을 승인했습니다"})
}

func (s *AutoStarter) reviewOrErr(ctx context.Context, a storage.AutoStart, issue string, err error) (bool, error) {
	if err != nil {
		return false, err
	}
	return s.review(ctx, a, issue)
}

// workIssue checks what does not depend on the plan: the service is in
// use and in auto mode with a budget and room in it, the department starts work on
// its own for these categories, and the work does not ask for review.
func (s *AutoStarter) workIssue(ctx context.Context, w domain.WorkItem) (string, error) {
	db := s.cfg.DB
	p, err := db.Project(ctx, w.ProjectID)
	if err != nil {
		return "", err
	}
	if p.Status == "archived" {
		return "보관된 서비스의 업무는 자동으로 시작하지 않습니다", nil
	}
	if p.Mode != domain.ModeAuto {
		return "서비스가 자동 모드가 아니어서 사람이 시작합니다", nil
	}
	var b engine.Budget
	json.Unmarshal(p.Budget, &b)
	if b.MaxTokens == nil && b.MaxCostUSD == nil {
		return "자동 실행에는 서비스 예산(토큰 또는 비용)을 먼저 정해야 합니다", nil
	}
	if w.ReviewBeforeStart {
		return "이 업무는 ‘시작 전 검토’가 켜져 있어 사람이 시작합니다", nil
	}
	if w.DepartmentID == "" {
		return "담당 부서가 없어 자동으로 시작하지 않습니다", nil
	}
	dept, err := db.Department(ctx, w.ProjectID, w.DepartmentID)
	if err != nil {
		return "", err
	}
	rules := domain.ParseAutomation(dept.Automation)
	switch {
	case dept.Status != domain.StatusActive:
		return "사용하지 않는 부서의 업무는 자동으로 시작하지 않습니다", nil
	case !rules.AutoStart:
		return dept.Name + "의 자동화 규칙이 자동 시작을 허용하지 않습니다", nil
	case !rules.AllowsCategories(w.Categories):
		return fmt.Sprintf("%s의 자동화 규칙이 이 업무의 분류(%s)를 허용하지 않습니다", dept.Name, orNone(strings.Join(w.Categories, ", "))), nil
	}
	if s.cfg.Hold != nil {
		if hold, err := s.cfg.Hold(ctx, w.ProjectID); err != nil || hold != "" {
			return "예산 때문에 자동으로 시작하지 않았습니다: " + hold, err
		}
	}
	return "", nil
}

// connectionIssue checks that automatic work may use an AI's connection.
func (s *AutoStarter) connectionIssue(ctx context.Context, w domain.WorkItem, ai domain.Assignment) (string, error) {
	p, err := s.cfg.DB.Project(ctx, w.ProjectID)
	if err != nil {
		return "", err
	}
	dept, err := s.cfg.DB.Department(ctx, w.ProjectID, w.DepartmentID)
	if err != nil {
		return "", err
	}
	if !autoPolicyAllows(p, domain.ParseAutomation(dept.Automation), ai.ConnectionID) {
		return fmt.Sprintf("%s의 AI 연결이 자동 실행 허용 범위(서비스 자동 정책과 부서 규칙)에 없습니다", ai.DisplayName), nil
	}
	return "", nil
}

// planIssue checks the saved plan: its assignees are AIs whose
// connections automatic work may use, its results stay within the
// department's scope, and a linked workflow version is one the department
// allows.
func (s *AutoStarter) planIssue(ctx context.Context, w domain.WorkItem) (string, error) {
	db := s.cfg.DB
	plan, saved, err := db.WorkPlan(ctx, w.ProjectID, w.ID)
	if err != nil {
		return "", err
	}
	if !saved {
		return "작업 계획이 없습니다", nil
	}
	dept, err := db.Department(ctx, w.ProjectID, w.DepartmentID)
	if err != nil {
		return "", err
	}
	rules := domain.ParseAutomation(dept.Automation)
	type step struct {
		assignment string
		outputs    []domain.Output
	}
	var steps []step
	assignees := map[string]domain.Assignment{}
	if plan.WorkflowVersionID != "" {
		if !slices.Contains(rules.WorkflowVersionIDs, plan.WorkflowVersionID) {
			return dept.Name + "의 자동화 규칙이 이 업무 흐름 버전의 자동 실행을 허용하지 않습니다", nil
		}
		v, err := db.WorkflowVersion(ctx, w.ProjectID, plan.WorkflowVersionID)
		if err != nil {
			return "", err
		}
		for id, a := range v.Assignments {
			assignees[id] = a.Assignment
		}
		for _, n := range v.Spec.Nodes {
			if n.Kind == domain.KindTask || n.Kind == domain.KindReview {
				steps = append(steps, step{n.AssignmentID, n.Outputs})
			}
		}
	} else {
		team, err := db.Assignments(ctx, w.ProjectID)
		if err != nil {
			return "", err
		}
		for _, a := range team {
			assignees[a.ID] = a
		}
		outs := plan.Outputs
		if len(outs) == 0 {
			outs = domain.DefaultWorkOutputs()
		}
		steps = append(steps, step{plan.AssignmentID, outs})
	}
	for _, st := range steps {
		a, ok := assignees[st.assignment]
		if !ok {
			return "작업 계획의 담당자를 찾을 수 없습니다", nil
		}
		if a.ActorKind != domain.ActorAI {
			return a.DisplayName + "(사람)이 맡은 작업은 자동으로 시작하지 않습니다", nil
		}
		if issue, err := s.connectionIssue(ctx, w, a); err != nil || issue != "" {
			return issue, err
		}
		for _, o := range st.outputs {
			if !rules.AllowsOutput(o.Type) {
				return fmt.Sprintf("결과 형식 ‘%s’는 %s의 자동 작업 범위 밖입니다", o.Type, dept.Name), nil
			}
		}
	}
	return "", nil
}

// uiReason is an error as a person reads it.
func uiReason(err error) string {
	var nr *engine.NotRunnableError
	if errors.As(err, &nr) && len(nr.Issues) > 0 {
		return nr.Issues[0].Message
	}
	return strings.TrimPrefix(err.Error(), storage.ErrInvalid.Error()+": ")
}

// nonInvalid keeps errors that are not a refusal to be explained.
func nonInvalid(err error) error {
	if errors.Is(err, storage.ErrInvalid) {
		return nil
	}
	return err
}
