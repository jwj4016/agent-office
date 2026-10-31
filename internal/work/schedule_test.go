package work_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
	"agent-office/internal/work"
)

// checkEvents counts the department checks the scheduler asked of a
// department.
func checkEvents(t *testing.T, f *judgeFixture, dept string) int {
	t.Helper()
	list, err := f.db.Deliveries(context.Background(), f.p.ID, storage.DeliveryFilter{DepartmentID: dept})
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, d := range list {
		if d.EventKind == domain.EventDepartmentCheck && d.Event.ScheduleSlot != "" {
			n++
		}
	}
	return n
}

// Stage 8: a regular check runs once per slot through the same judgment
// as a person's request, in the background lane. Changing, turning off
// and on, or looking again never runs a slot twice; after the app was
// closed only the latest missed slot runs; stopped departments and
// archived services run nothing.
func TestRegularChecksRunOncePerSlot(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	f.answer = func(dept, prompt string) (string, error) {
		if !strings.Contains(prompt, "광고 문구를 살핀다") {
			t.Errorf("the schedule's note is not in the prompt:\n%s", prompt)
		}
		return propose(dept + " 정기 과제"), nil
	}
	var now time.Time
	sched := work.NewScheduler(f.db, func() time.Time { return now })
	save := func(s domain.DepartmentSchedule) domain.DepartmentSchedule {
		t.Helper()
		s.ProjectID, s.DepartmentID, s.Timezone, s.Note = f.p.ID, f.leg.ID, "Asia/Seoul", "광고 문구를 살핀다"
		saved, err := f.db.SaveDepartmentSchedule(ctx, s)
		if err != nil {
			t.Fatal(err)
		}
		return saved
	}
	drain := func(want int) {
		t.Helper()
		n, err := sched.Drain(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if n != want {
			t.Fatalf("ran %d slots, want %d", n, want)
		}
	}

	daily := save(domain.DepartmentSchedule{Frequency: domain.ScheduleDaily, TimeOfDay: "09:00", Enabled: true})
	// A slot due before the save does not run because of the save.
	now = time.Now()
	drain(0)
	first, _ := daily.NextSlot(time.Now())
	now = first.Add(time.Minute)
	drain(1)
	drain(0) // looking again
	f.settle(t)
	if got := proposalTitles(t, f, "proposed"); len(got) != 1 || got[0] != "법무팀:법무팀 정기 과제" {
		t.Fatalf("proposals = %v", got)
	}
	var lane string
	if err := f.db.Read().QueryRowContext(ctx, `SELECT lane FROM work_ai_jobs WHERE kind = 'judgment'`).Scan(&lane); err != nil || lane != storage.LaneBackground {
		t.Fatalf("lane = %q, %v", lane, err)
	}

	// Moving the time later the same day keeps the day's slot: no second run.
	save(domain.DepartmentSchedule{Frequency: domain.ScheduleDaily, TimeOfDay: "18:00", Enabled: true})
	now = time.Date(first.Year(), first.Month(), first.Day(), 18, 30, 0, 0, first.Location())
	drain(0)
	// Off and on again: the slot of that day still ran once.
	save(domain.DepartmentSchedule{Frequency: domain.ScheduleDaily, TimeOfDay: "18:00", Enabled: false})
	drain(0)
	save(domain.DepartmentSchedule{Frequency: domain.ScheduleDaily, TimeOfDay: "18:00", Enabled: true})
	drain(0)
	// Closed for five days: only the latest slot runs.
	now = now.AddDate(0, 0, 5)
	drain(1)
	drain(0)
	f.settle(t)
	if n := checkEvents(t, f, f.leg.ID); n != 2 {
		t.Fatalf("regular checks = %d", n)
	}

	// A stopped department runs nothing, and its waiting deliveries are skipped.
	if _, err := f.db.RequestDepartmentCheck(ctx, f.p.ID, f.leg.ID, "직접 요청"); err != nil {
		t.Fatal(err)
	}
	leg := f.leg
	leg.Status = domain.StatusInactive
	if _, err := f.db.SaveDepartment(ctx, leg); err != nil {
		t.Fatal(err)
	}
	calls := len(f.calls)
	now = now.AddDate(0, 0, 1)
	drain(0)
	f.settle(t)
	if len(f.calls) != calls {
		t.Fatalf("a stopped department was asked: %v", f.calls)
	}
	list, _ := f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{DepartmentID: f.leg.ID, Status: domain.DeliverySkipped})
	if len(list) != 1 || !strings.Contains(list[0].Note, "사용하지 않는 부서") {
		t.Fatalf("skipped deliveries = %+v", list)
	}

	// An archived service runs nothing either.
	leg.Status = domain.StatusActive
	if _, err := f.db.SaveDepartment(ctx, leg); err != nil {
		t.Fatal(err)
	}
	// A judgment already queued is held, not called, once the service is archived.
	if _, err := f.db.RequestDepartmentCheck(ctx, f.p.ID, f.leg.ID, "보관 직전 요청"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.proc.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if err := f.db.SetProjectArchived(ctx, f.p.ID, true); err != nil {
		t.Fatal(err)
	}
	calls = len(f.calls)
	f.settle(t)
	var held int
	f.db.Read().QueryRowContext(ctx, `SELECT COUNT(*) FROM work_ai_jobs WHERE status = 'held' AND hold_reason LIKE '보관된 서비스%'`).Scan(&held)
	if len(f.calls) != calls || held != 1 {
		t.Fatalf("archived service: calls %v, held jobs %d", f.calls[calls:], held)
	}
	now = now.AddDate(0, 0, 1)
	drain(0)
	if views, _ := f.db.DepartmentSchedules(ctx, f.p.ID, now); len(views) != 1 || views[0].NextDueAt != "" || views[0].LastSlotAt == "" {
		t.Fatalf("schedule view of an archived service = %+v", views)
	}
}

// Stage 8 acceptance: a department added while the service runs gets
// only later events; it reviews earlier documents on request, then takes
// part in later events and its regular check.
func TestNewDepartmentJoinsLaterEvents(t *testing.T) {
	f := newJudgeFixture(t)
	ctx := context.Background()
	f.answer = func(dept, _ string) (string, error) { return propose(dept + " 검토"), nil }
	old := f.share(t, "캠페인 기획안", "h1")
	f.settle(t)

	role, _ := f.db.SaveRole(ctx, domain.Role{Name: "홍보 담당"})
	conn, _ := f.db.Connections(ctx)
	a, _ := f.db.SaveAssignment(ctx, domain.Assignment{ProjectID: f.p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
		DisplayName: "홍보 AI", ConnectionID: conn[0].ID, Model: "m"})
	pr, err := f.db.SaveDepartment(ctx, domain.Department{ProjectID: f.p.ID, Name: "홍보팀", MemberIDs: []string{a.ID}, LeadAssignmentID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.SaveSubscription(ctx, domain.Subscription{ProjectID: f.p.ID, DepartmentID: pr.ID,
		EventKinds: []string{domain.EventDocumentShared}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	if list, _ := f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{DepartmentID: pr.ID}); len(list) != 0 {
		t.Fatalf("a new department got earlier events: %+v", list)
	}

	// Earlier material only on request.
	if _, err := f.db.RequestDocumentReview(ctx, f.p.ID, pr.ID, []string{old.ID}, "지난 기획안을 본다"); err != nil {
		t.Fatal(err)
	}
	f.settle(t)
	// Then later events and the regular check.
	f.share(t, "출시 일정", "h2")
	sched, err := f.db.SaveDepartmentSchedule(ctx, domain.DepartmentSchedule{ProjectID: f.p.ID, DepartmentID: pr.ID,
		Frequency: domain.ScheduleDaily, TimeOfDay: "10:00", Timezone: "Asia/Seoul", Enabled: true})
	if err != nil {
		t.Fatal(err)
	}
	next, _ := sched.NextSlot(time.Now())
	if n, err := work.NewScheduler(f.db, func() time.Time { return next }).Drain(ctx); err != nil || n != 1 {
		t.Fatalf("regular check: %d, %v", n, err)
	}
	f.settle(t)
	list, _ := f.db.Deliveries(ctx, f.p.ID, storage.DeliveryFilter{DepartmentID: pr.ID})
	var titles []string
	for _, d := range list {
		titles = append(titles, d.Event.Title+"="+d.Status)
	}
	if len(list) != 3 {
		t.Fatalf("new department's deliveries = %v", titles)
	}
	if got := proposalTitles(t, f, "proposed"); !strings.Contains(strings.Join(got, ","), "홍보팀:홍보팀 검토") {
		t.Fatalf("proposals = %v", got)
	}
}
