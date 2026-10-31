package storage_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

func subscribe(t *testing.T, c *company, d domain.Department, kinds []string, cats ...string) domain.Subscription {
	t.Helper()
	s, err := c.db.SaveSubscription(context.Background(), domain.Subscription{ProjectID: c.project.ID, DepartmentID: d.ID,
		EventKinds: kinds, Categories: cats, Guidance: d.Name + "의 관점", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// inbox returns "department:purpose:status" for an event's deliveries.
func inbox(t *testing.T, c *company, seq int64) []string {
	t.Helper()
	all, err := c.db.Deliveries(context.Background(), c.project.ID, storage.DeliveryFilter{})
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]string{c.mkt.ID: "mkt", c.dev.ID: "dev", c.fin.ID: "fin"}
	var out []string
	for _, d := range all {
		if d.EventSequence == seq {
			out = append(out, fmt.Sprintf("%s:%s:%s", names[d.DepartmentID], d.Purpose, d.Status))
		}
	}
	slices.Sort(out)
	return out
}

func lastEvent(t *testing.T, c *company, workID string) storage.WorkEventView {
	t.Helper()
	evs, err := c.db.WorkEvents(context.Background(), c.project.ID, workID)
	if err != nil || len(evs) == 0 {
		t.Fatalf("events of %s: %v %v", workID, evs, err)
	}
	return evs[len(evs)-1]
}

func TestWorkEventsReachSubscribedDepartments(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	created := []string{domain.EventWorkCreated}
	subscribe(t, c, c.dev, created)       // every category
	subscribe(t, c, c.fin, created, "비용") // costs only
	subscribe(t, c, c.mkt, created)       // marketing's own work is not sent back to it
	goal, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, Kind: domain.WorkGoal, Title: "캠페인", DepartmentID: c.mkt.ID})
	plan, err := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, ParentID: goal.ID, DepartmentID: c.mkt.ID,
		Title: "기획안", Categories: []string{"기획", "비용"}})
	if err != nil {
		t.Fatal(err)
	}
	ev := lastEvent(t, c, plan.ID)
	if ev.Kind != domain.EventWorkCreated || ev.Event.GoalID != goal.ID || ev.Event.DepartmentID != c.mkt.ID ||
		ev.Event.Automated || ev.Event.ChainDepth != 0 || ev.Event.ChainRootSequence != nil || ev.Event.ChainRoot(ev.Sequence) != ev.Sequence {
		t.Fatalf("event = %+v", ev)
	}
	if got := inbox(t, c, ev.Sequence); !slices.Equal(got, []string{"dev:judge:pending", "fin:judge:pending"}) {
		t.Fatalf("deliveries = %v", got)
	}
	// Without a matching category finance is left out.
	page, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, DepartmentID: c.mkt.ID, Title: "랜딩", Categories: []string{"개발"}})
	if got := inbox(t, c, lastEvent(t, c, page.ID).Sequence); !slices.Equal(got, []string{"dev:judge:pending"}) {
		t.Fatalf("deliveries = %v", got)
	}
}

func TestCompletedAndReopenedEvents(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	subscribe(t, c, c.fin, []string{domain.EventWorkCompleted, domain.EventWorkReopened})
	w, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, DepartmentID: c.dev.ID, Title: "구현 검토"})
	kinds := func() []string {
		evs, _ := c.db.WorkEvents(ctx, c.project.ID, w.ID)
		var out []string
		for _, e := range evs {
			out = append(out, e.Kind)
		}
		return out
	}
	c.db.SetWorkStatus(ctx, c.project.ID, w.ID, domain.WorkInProgress, "")
	c.db.SetWorkStatus(ctx, c.project.ID, w.ID, domain.WorkDone, "검토 끝")
	c.db.SetWorkStatus(ctx, c.project.ID, w.ID, domain.WorkReady, "다시 볼 것") // reopened
	c.db.SetWorkStatus(ctx, c.project.ID, w.ID, domain.WorkCancelled, "필요 없음")
	c.db.SetWorkStatus(ctx, c.project.ID, w.ID, domain.WorkReceived, "") // reopened again
	want := []string{domain.EventWorkCreated, domain.EventWorkCompleted, domain.EventWorkReopened, domain.EventWorkReopened}
	if got := kinds(); !slices.Equal(got, want) {
		t.Fatalf("events = %v", got)
	}
	evs, _ := c.db.WorkEvents(ctx, c.project.ID, w.ID)
	if evs[1].Event.Note != "검토 끝" || len(evs[1].Deliveries) != 1 || evs[1].Deliveries[0].DepartmentID != c.fin.ID || len(evs[0].Deliveries) != 0 {
		t.Fatalf("completed event = %+v", evs[1])
	}
}

func TestDirectCheckIgnoresSubscriptions(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	subscribe(t, c, c.fin, []string{domain.EventWorkCreated})
	rec, err := c.db.RequestDepartmentCheck(ctx, c.project.ID, c.dev.ID, "이번 주 할 일을 찾아 주세요")
	if err != nil {
		t.Fatal(err)
	}
	if got := inbox(t, c, rec.Sequence); !slices.Equal(got, []string{"dev:judge:pending"}) {
		t.Fatalf("deliveries = %v", got)
	}
	// An inactive department cannot be asked; nothing is stored.
	d := c.mkt
	d.Status = domain.StatusInactive
	c.db.SaveDepartment(ctx, d)
	if _, err := c.db.RequestDepartmentCheck(ctx, c.project.ID, c.mkt.ID, ""); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("inactive target: %v", err)
	}
	other, _ := c.db.CreateProject(ctx, domain.Project{Name: "다른 서비스"})
	if _, err := c.db.RequestDepartmentCheck(ctx, other.ID, c.dev.ID, ""); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("another service's department: %v", err)
	}
	all, _ := c.db.Deliveries(ctx, c.project.ID, storage.DeliveryFilter{})
	if len(all) != 1 {
		t.Fatalf("deliveries = %d", len(all))
	}
}

func TestInactiveAndOffDepartmentsGetNothing(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	created := []string{domain.EventWorkCreated}
	subscribe(t, c, c.dev, created)
	off := subscribe(t, c, c.fin, created)
	off.Enabled = false
	if _, err := c.db.SaveSubscription(ctx, off); err != nil {
		t.Fatal(err)
	}
	d := c.dev
	d.Status = domain.StatusInactive
	c.db.SaveDepartment(ctx, d)
	w, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, Title: "업무"})
	if got := inbox(t, c, lastEvent(t, c, w.ID).Sequence); len(got) != 0 {
		t.Fatalf("deliveries = %v", got)
	}
	// A subscription only names subscribable kinds, in its own service.
	if _, err := c.db.SaveSubscription(ctx, domain.Subscription{ProjectID: c.project.ID, DepartmentID: c.mkt.ID,
		EventKinds: []string{domain.EventDepartmentCheck}}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("direct kind subscribed: %v", err)
	}
	other, _ := c.db.CreateProject(ctx, domain.Project{Name: "다른 서비스"})
	if _, err := c.db.SaveSubscription(ctx, domain.Subscription{ProjectID: other.ID, DepartmentID: c.mkt.ID,
		EventKinds: created}); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("another service's department: %v", err)
	}
}

func TestJudgeFanoutIsLimited(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	for i := range domain.DefaultJudgeFanout + 1 {
		d, err := c.db.SaveDepartment(ctx, domain.Department{ProjectID: c.project.ID, Name: fmt.Sprintf("팀%d", i)})
		if err != nil {
			t.Fatal(err)
		}
		subscribe(t, c, d, []string{domain.EventWorkCreated})
	}
	w, _ := c.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: c.project.ID, Title: "많은 부서가 볼 일"})
	all, _ := c.db.Deliveries(ctx, c.project.ID, storage.DeliveryFilter{})
	held := 0
	for _, d := range all {
		if d.EventSequence != lastEvent(t, c, w.ID).Sequence {
			continue
		}
		if d.Status == domain.DeliveryHeld {
			held++
			if d.HoldReason == "" {
				t.Fatal("held without reason")
			}
		}
	}
	if len(all) != domain.DefaultJudgeFanout+1 || held != 1 {
		t.Fatalf("deliveries = %d, held = %d", len(all), held)
	}
}

func TestDeliveryLifecycle(t *testing.T) {
	c := newCompany(t)
	ctx := context.Background()
	rec, _ := c.db.RequestDepartmentCheck(ctx, c.project.ID, c.dev.ID, "")
	all, _ := c.db.Deliveries(ctx, c.project.ID, storage.DeliveryFilter{Status: "open"})
	d := all[0]
	if d.EventSequence != rec.Sequence || d.EventKind != domain.EventDepartmentCheck || d.Event.TargetDepartmentID != c.dev.ID {
		t.Fatalf("delivery = %+v", d)
	}
	// A delivery is claimed once.
	if ok, _ := c.db.ClaimDelivery(ctx, d.ID); !ok {
		t.Fatal("first claim failed")
	}
	if ok, _ := c.db.ClaimDelivery(ctx, d.ID); ok {
		t.Fatal("claimed twice")
	}
	// A crash leaves it processing; recovery puts it back.
	if n, _ := c.db.RecoverDeliveries(ctx); n != 1 {
		t.Fatalf("recovered %d", n)
	}
	c.db.ClaimDelivery(ctx, d.ID)
	if err := c.db.FinishDelivery(ctx, d.ID, domain.DeliveryFailed, "", "연결 없음"); err != nil {
		t.Fatal(err)
	}
	if err := c.db.SkipDelivery(ctx, "other-project", d.ID, ""); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("skip from another service: %v", err)
	}
	if err := c.db.RetryDelivery(ctx, c.project.ID, d.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.db.RetryDelivery(ctx, c.project.ID, d.ID); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("retry of a pending delivery: %v", err)
	}
	if err := c.db.SkipDelivery(ctx, c.project.ID, d.ID, "필요 없음"); err != nil {
		t.Fatal(err)
	}
	got, _ := c.db.Deliveries(ctx, c.project.ID, storage.DeliveryFilter{DepartmentID: c.dev.ID})
	if got[0].Status != domain.DeliverySkipped || got[0].Note != "필요 없음" || got[0].Error != "" {
		t.Fatalf("after skip = %+v", got[0])
	}
	if open, _ := c.db.Deliveries(ctx, c.project.ID, storage.DeliveryFilter{Status: "open"}); len(open) != 0 {
		t.Fatalf("open = %v", open)
	}
}
