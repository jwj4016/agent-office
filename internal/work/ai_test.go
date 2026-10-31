package work_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
	"agent-office/internal/work"
)

const planJSON = `{"assignmentId":"%ASG%","instructions":"기존 화면 구조를 먼저 본다","materials":"기획안 초안","outputs":[{"key":"review","type":"markdown"},{"key":"bad","type":"video"}],"notes":"화면 수를 3개로 가정"}`

func script(lines ...string) []providers.Step {
	var out []providers.Step
	for _, l := range lines {
		var s providers.Step
		if err := json.Unmarshal([]byte(l), &s); err != nil {
			panic(err)
		}
		out = append(out, s)
	}
	return out
}

type aiFixture struct {
	db   *storage.DB
	path string
	eng  *engine.Engine
	prov *providers.TestProvider
	ai   *work.AI
	p    domain.Project
	asg  domain.Assignment
	dept domain.Department
	item domain.WorkItem
}

func newAIFixture(t *testing.T, model string) *aiFixture {
	t.Helper()
	ctx := context.Background()
	f := &aiFixture{path: filepath.Join(t.TempDir(), "ai.db")}
	f.db = open(t, f.path)
	t.Cleanup(func() { f.db.Close() })
	f.p, _ = f.db.CreateProject(ctx, domain.Project{Name: "캠페인"})
	conn, _ := f.db.SaveConnection(ctx, domain.ProviderConnection{Name: "test", Provider: "test"})
	role, _ := f.db.SaveRole(ctx, domain.Role{Name: "개발자"})
	f.asg, _ = f.db.SaveAssignment(ctx, domain.Assignment{ProjectID: f.p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
		DisplayName: "개발 AI", ConnectionID: conn.ID, Model: model})
	f.dept, _ = f.db.SaveDepartment(ctx, domain.Department{ProjectID: f.p.ID, Name: "개발팀", Responsibility: "구현 검토",
		MemberIDs: []string{f.asg.ID}, LeadAssignmentID: f.asg.ID})
	f.item, _ = f.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: f.p.ID, DepartmentID: f.dept.ID, Title: "페이지 구현 검토", DoneCriteria: "일정 정리"})
	usage := `{"emit":"usage","payload":{"inputTokens":100,"outputTokens":40,"costUsd":0.01}}`
	f.prov = &providers.TestProvider{Scripts: map[string][]providers.Step{
		"plan-ok": script(usage,
			`{"write":{"key":"result","content":`+mustQuote(strings.ReplaceAll(planJSON, "%ASG%", f.asg.ID))+`}}`,
			`{"complete":{"status":"succeeded"}}`),
		"plan-fail": script(usage, `{"complete":{"status":"failed","error":"rate limited"}}`),
		"plan-slow": script(`{"sleep":"1500ms"}`, `{"write":{"key":"result","content":`+mustQuote(strings.ReplaceAll(planJSON, "%ASG%", f.asg.ID))+`}}`,
			`{"complete":{"status":"succeeded"}}`),
	}}
	f.eng = engine.New(engine.Config{DB: f.db, DataDir: t.TempDir()})
	f.ai = work.NewAI(work.AIConfig{
		DB:        f.db,
		Providers: func(domain.ProviderConnection) (providers.Provider, error) { return f.prov, nil },
		Hold: func(ctx context.Context, projectID string) (string, error) {
			u, err := f.eng.ProjectUsage(ctx, projectID)
			return u.HoldReason, err
		},
		Kinds: map[string]work.Kind{storage.JobPlan: work.PlanKind{DB: f.db}},
	})
	return f
}

func mustQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestPlanProposalIsAppliedAndPaidFor(t *testing.T) {
	f := newAIFixture(t, "plan-ok")
	ctx := context.Background()
	job, err := work.RequestPlan(ctx, f.db, f.p.ID, f.item.ID, "click-1")
	if err != nil {
		t.Fatal(err)
	}
	// The same click arriving twice is one job.
	again, _ := work.RequestPlan(ctx, f.db, f.p.ID, f.item.ID, "click-1")
	if again.ID != job.ID {
		t.Fatalf("duplicate click made job %s", again.ID)
	}
	if n, err := f.ai.Drain(ctx, storage.LaneInteractive); err != nil || n != 1 {
		t.Fatalf("drained %d: %v", n, err)
	}
	job, _ = f.db.AIJob(ctx, f.p.ID, job.ID)
	if job.Status != storage.JobReady || job.Attempts != 1 {
		t.Fatalf("job = %+v", job)
	}
	var proposed work.ProposedPlan
	json.Unmarshal(job.Result, &proposed)
	if len(proposed.Outputs) != 1 || proposed.Outputs[0].Key != "review" || proposed.Notes == "" {
		t.Fatalf("proposal = %+v", proposed) // the video output was dropped
	}
	// The call is in the service's usage.
	u, _ := f.eng.ProjectUsage(ctx, f.p.ID)
	if u.Attempts != 1 || u.InputTokens != 100 || u.CostUSD != 0.01 || u.UnknownCostAttempts != 0 {
		t.Fatalf("usage = %+v", u)
	}
	plan, err := work.ApplyPlan(ctx, f.db, f.p.ID, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if plan.AssignmentID != f.asg.ID || plan.Instructions != "기존 화면 구조를 먼저 본다" || plan.Outputs[0].Key != "review" {
		t.Fatalf("plan = %+v", plan)
	}
	if job, _ = f.db.AIJob(ctx, f.p.ID, job.ID); job.AppliedAt == "" {
		t.Fatal("not marked applied")
	}
}

func TestStalePlanProposalIsRefused(t *testing.T) {
	f := newAIFixture(t, "plan-ok")
	ctx := context.Background()
	job, _ := work.RequestPlan(ctx, f.db, f.p.ID, f.item.ID, "c1")
	f.ai.Drain(ctx, storage.LaneInteractive)
	w := f.item
	w.DoneCriteria = "일정과 비용 정리"
	if _, err := f.db.UpdateWorkItem(ctx, w); err != nil {
		t.Fatal(err)
	}
	if _, err := work.ApplyPlan(ctx, f.db, f.p.ID, job.ID); !errors.Is(err, storage.ErrInvalid) || !strings.Contains(err.Error(), "다시 요청") {
		t.Fatalf("stale apply: %v", err)
	}
	// A new click asks again on the new input.
	fresh, _ := work.RequestPlan(ctx, f.db, f.p.ID, f.item.ID, "c2")
	if fresh.ID == job.ID {
		t.Fatal("new click reused the old job")
	}
	f.ai.Drain(ctx, storage.LaneInteractive)
	if _, err := work.ApplyPlan(ctx, f.db, f.p.ID, fresh.ID); err != nil {
		t.Fatalf("fresh apply: %v", err)
	}
}

func TestFailedCallsAreRetriedAndPaidFor(t *testing.T) {
	f := newAIFixture(t, "plan-fail")
	ctx := context.Background()
	job, _ := work.RequestPlan(ctx, f.db, f.p.ID, f.item.ID, "c1")
	f.ai.Drain(ctx, storage.LaneInteractive)
	job, _ = f.db.AIJob(ctx, f.p.ID, job.ID)
	if job.Status != storage.JobFailed || job.Attempts != 3 || !strings.Contains(job.Error, "rate limited") {
		t.Fatalf("after automatic retries = %+v", job)
	}
	// A person's retry is one more attempt; earlier attempts stay paid.
	if err := f.db.RetryAIJob(ctx, f.p.ID, job.ID); err != nil {
		t.Fatal(err)
	}
	f.ai.Drain(ctx, storage.LaneInteractive)
	job, _ = f.db.AIJob(ctx, f.p.ID, job.ID)
	u, _ := f.eng.ProjectUsage(ctx, f.p.ID)
	if job.Status != storage.JobFailed || job.Attempts != 4 || u.Attempts != 4 || u.InputTokens != 400 {
		t.Fatalf("after manual retry: job=%+v usage=%+v", job, u)
	}
}

func TestBudgetHoldStopsTheCall(t *testing.T) {
	f := newAIFixture(t, "plan-ok")
	ctx := context.Background()
	p := f.p
	p.Budget = json.RawMessage(`{"maxTokens":100}`)
	if _, err := f.db.UpdateProject(ctx, p); err != nil {
		t.Fatal(err)
	}
	first, _ := work.RequestPlan(ctx, f.db, f.p.ID, f.item.ID, "c1")
	f.ai.Drain(ctx, storage.LaneInteractive) // 140 tokens: now over budget
	second, _ := work.RequestPlan(ctx, f.db, f.p.ID, f.item.ID, "c2")
	f.ai.Drain(ctx, storage.LaneInteractive)
	first, _ = f.db.AIJob(ctx, f.p.ID, first.ID)
	second, _ = f.db.AIJob(ctx, f.p.ID, second.ID)
	if first.Status != storage.JobReady || second.Status != storage.JobHeld || second.HoldReason == "" || second.Attempts != 0 {
		t.Fatalf("first=%+v second=%+v", first, second)
	}
}

func TestInterruptedCallIsRecovered(t *testing.T) {
	f := newAIFixture(t, "plan-slow")
	ctx, cancel := context.WithCancel(context.Background())
	job, _ := work.RequestPlan(context.Background(), f.db, f.p.ID, f.item.ID, "c1")
	done := make(chan struct{})
	go func() { f.ai.Drain(ctx, storage.LaneInteractive); close(done) }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	<-done
	if j, _ := f.db.AIJob(context.Background(), f.p.ID, job.ID); j.Status != storage.JobWorking || j.Attempts != 1 {
		t.Fatalf("after stop = %+v", j)
	}
	if err := f.db.RecoverAIJobs(context.Background()); err != nil {
		t.Fatal(err)
	}
	j, _ := f.db.AIJob(context.Background(), f.p.ID, job.ID)
	if j.Status != storage.JobPending {
		t.Fatalf("after recovery = %+v", j)
	}
	f.ai.Drain(context.Background(), storage.LaneInteractive)
	if j, _ = f.db.AIJob(context.Background(), f.p.ID, job.ID); j.Status != storage.JobReady || j.Attempts != 2 {
		t.Fatalf("after the second attempt = %+v", j)
	}
}

// A slow background job does not hold up an interactive one.
func TestLanesDoNotWaitForEachOther(t *testing.T) {
	f := newAIFixture(t, "plan-ok")
	ctx := context.Background()
	slow, _, _ := f.db.CreateAIJob(ctx, storage.AIJob{ProjectID: f.p.ID, Kind: storage.JobPlan, Lane: storage.LaneBackground,
		RequestKey: "plan:" + f.item.ID + ":bg", WorkItemID: f.item.ID, AssignmentID: f.asg.ID, ConnectionID: f.asg.ConnectionID,
		Model: "plan-slow", InputSnapshot: json.RawMessage(`{}`)})
	quick, _ := work.RequestPlan(ctx, f.db, f.p.ID, f.item.ID, "c1")
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	go f.ai.Run(runCtx)
	deadline := time.Now().Add(5 * time.Second)
	for {
		q, _ := f.db.AIJob(ctx, f.p.ID, quick.ID)
		s, _ := f.db.AIJob(ctx, f.p.ID, slow.ID)
		if q.Status == storage.JobReady {
			if s.Status == storage.JobReady {
				t.Fatal("interactive job waited for the slow background job")
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("interactive job not done: %+v", q)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
