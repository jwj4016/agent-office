package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-office/internal/connect"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/storage"
	"agent-office/internal/work"
)

// TestRealServiceOperations runs the service operations plan's example
// (docs/plan/service-operations/plan.md §1, §7) through the app with the
// real Claude of this PC's login, so departments judge, decide and plan
// with a real AI. It uses the account's quota and is off by default:
//
//	AGENT_OFFICE_REAL_OPERATIONS=sonnet [AGENT_OFFICE_REAL_OPERATIONS_OUT=<file>] \
//	  go test . -run TestRealServiceOperations -v -timeout 60m
//
// Mechanics (who is asked, what is stored, what is blocked) must hold;
// what the AI decides is reported as QUALITY lines, not failed on.
func TestRealServiceOperations(t *testing.T) {
	model := os.Getenv("AGENT_OFFICE_REAL_OPERATIONS")
	if model == "" {
		t.Skip("set AGENT_OFFICE_REAL_OPERATIONS=<model, e.g. sonnet> to run with the real Claude")
	}
	if _, ok := connect.FindExecutable("claude"); !ok {
		t.Skip("claude CLI is not installed")
	}
	a := startApp(t)
	ctx := a.ctx
	var report []string
	note := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		report = append(report, line)
		t.Log(line)
	}
	t.Cleanup(func() {
		if out := os.Getenv("AGENT_OFFICE_REAL_OPERATIONS_OUT"); out != "" {
			os.WriteFile(out, []byte(strings.Join(report, "\n")+"\n"), 0o644)
		}
	})
	waitFor := func(what string, limit time.Duration, done func() bool) bool {
		t.Helper()
		deadline := time.Now().Add(limit)
		for !done() {
			if time.Now().After(deadline) {
				note("TIMEOUT: %s (%s)", what, limit)
				return false
			}
			time.Sleep(time.Second)
		}
		return true
	}
	start := time.Now()

	// A service with a work folder and a budget, and a Claude connection
	// that uses this PC's login.
	folder := t.TempDir()
	p := need(a.CreateProject(ProjectInput{Name: "신규 고객 캠페인 서비스", Mode: "review"}))
	tokens := int64(5_000_000)
	p = need(a.UpdateProject(ProjectInput{ID: p.ID, Name: p.Name, Mode: "review", WorkspacePath: folder, Budget: engine.Budget{MaxTokens: &tokens}}))
	cfg, _ := json.Marshal(connect.Config{AuthMode: connect.AuthLocalLogin})
	conn := need(a.db.SaveConnection(ctx, domain.ProviderConnection{Name: "내 Claude", Provider: "claude", Config: cfg,
		VerifiedCapabilities: json.RawMessage(`{"call":true}`)}))
	dept := func(name, responsibility string) domain.Department {
		role := need(a.SaveRole(domain.Role{Name: name + " 대표"}))
		as := need(a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI, DisplayName: name + " AI",
			ConnectionID: conn.ID, Model: model}))
		return need(a.SaveDepartment(domain.Department{ProjectID: p.ID, Name: name, Responsibility: responsibility,
			MemberIDs: []string{as.ID}, LeadAssignmentID: as.ID}))
	}
	mkt := dept("마케팅팀", "고객 캠페인을 기획하고 홍보 문구와 일정을 정한다.")
	dev := dept("개발팀", "웹사이트·랜딩 페이지·앱 기능을 구현하고 기술 검토를 한다.")
	fin := dept("재무팀", "비용과 예산을 검토하고 지출을 승인한다.")
	leg := dept("법무팀", "약관과 개인정보 처리 등 법적 검토를 한다.")
	need(a.SaveSubscription(domain.Subscription{ProjectID: p.ID, DepartmentID: dev.ID, EventKinds: []string{domain.EventDocumentShared, domain.EventDocumentRevised},
		Guidance: "공유된 자료에 개발(페이지·기능 구현)이 필요한 부분이 있는지 본다", Enabled: true}))
	need(a.SaveSubscription(domain.Subscription{ProjectID: p.ID, DepartmentID: fin.ID, EventKinds: []string{domain.EventDocumentShared, domain.EventDocumentRevised},
		Guidance: "공유된 자료에 비용이나 예산 확인이 필요한 부분이 있는지 본다", Enabled: true}))

	goal := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, Kind: domain.WorkGoal, DepartmentID: mkt.ID, Title: "신규 고객 캠페인 준비"}))
	plan := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, ParentID: goal.ID, DepartmentID: mkt.ID, Title: "캠페인 기획"}))

	judgmentsFor := func(eventTitle string) map[string]storage.Judgment {
		out := map[string]storage.Judgment{}
		for _, j := range need(a.ListJudgments(p.ID, "")) {
			if j.Event.Title == eventTitle || strings.HasPrefix(j.Event.Title, eventTitle) {
				out[j.DepartmentName] = j
			}
		}
		return out
	}
	share := func(file, title, body, documentID string) storage.DocumentVersion {
		t.Helper()
		if err := os.WriteFile(filepath.Join(folder, file), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return need(a.ShareWorkspaceFile(p.ID, file, storage.NewDocumentVersion{DocumentID: documentID, Title: title, OwnerDepartmentID: mkt.ID,
			WorkItemID: plan.ID, Categories: []string{"기획"}}))
	}

	// 1-5. Marketing shares the draft plan; development and finance judge it,
	// legal (no subscription) is not asked.
	v1 := share("campaign-plan.md", "캠페인 기획안", `# 신규 고객 캠페인 기획안 (초안)

- 기간: 다음 달 1일부터 4주
- 목표: 신규 가입 3,000명
- 랜딩 페이지를 새로 만들어 가입 이벤트(첫 구매 20% 쿠폰)를 안내한다. 가입 폼과 쿠폰 발급이 필요하다.
- 온라인 광고 예산 3,000만 원(검색 광고 60%, SNS 40%), 쿠폰 비용 별도 추정 필요
- 홍보 문구 초안은 마케팅팀이 작성한다.
`, "")
	if !waitFor("development and finance judge the plan", 10*time.Minute, func() bool { return len(judgmentsFor("캠페인 기획안")) >= 2 }) {
		t.Fatal("judgments did not finish")
	}
	js := judgmentsFor("캠페인 기획안")
	for _, name := range []string{"개발팀", "재무팀"} {
		j := js[name]
		titles := []string{}
		for _, pr := range need(a.ListProposals(p.ID, "")) {
			if pr.JudgmentID == j.ID {
				titles = append(titles, pr.Title)
			}
		}
		note("QUALITY 기획안 공유 → %s: %s (%s) 제안 %v", name, j.Outcome, j.Reason, titles)
	}
	if _, asked := js["법무팀"]; asked {
		t.Fatal("legal, which follows nothing, judged the plan")
	}
	if ds := need(a.ListDeliveries(p.ID, storage.DeliveryFilter{DepartmentID: leg.ID})); len(ds) != 0 {
		t.Fatalf("legal got deliveries: %+v", ds)
	}

	// Register what was proposed; each registers once, and the work knows why it started.
	var devWork, finWork domain.WorkItem
	for _, pr := range need(a.ListProposals(p.ID, "proposed")) {
		w := need(a.AcceptProposal(p.ID, pr.ID, storage.ProposalEdit{}))
		if _, err := a.AcceptProposal(p.ID, pr.ID, storage.ProposalEdit{}); err == nil {
			t.Fatal("a proposal registered twice")
		}
		if pr.DepartmentID == dev.ID && devWork.ID == "" {
			devWork = w
		}
		if pr.DepartmentID == fin.ID && finWork.ID == "" {
			finWork = w
		}
		l := need(a.GetWorkLineage(p.ID, w.ID))
		if len(l.Causes) != 1 || l.Causes[0].DocumentTitle != "캠페인 기획안" || l.Causes[0].FromWorkItemID != plan.ID {
			t.Fatalf("lineage of %q = %+v", w.Title, l)
		}
	}
	note("등록한 업무: 개발 %q, 재무 %q", devWork.Title, finWork.Title)

	// Another plan only development needs: finance should find nothing to do.
	share("app-notice.md", "앱 알림 문구 개선안", `# 앱 푸시 알림 문구 개선안

- 장바구니 알림 문구를 짧게 바꾸고, 알림을 눌렀을 때 장바구니 화면으로 바로 이동하게 한다(앱 화면 이동 기능 수정 필요).
- 추가 비용 없음. 광고나 외부 발송 비용은 쓰지 않는다.
`, "")
	waitFor("the second plan is judged", 10*time.Minute, func() bool { return len(judgmentsFor("앱 알림 문구 개선안")) >= 2 })
	for name, j := range judgmentsFor("앱 알림 문구 개선안") {
		note("QUALITY 알림 개선안 공유 → %s: %s (%s)", name, j.Outcome, j.Reason)
	}

	// 6-8. Development uses version 1; a revision asks only work that used it.
	if devWork.ID != "" {
		need(0, a.LinkDocument(p.ID, devWork.ID, v1.ID))
	}
	share("campaign-plan.md", "캠페인 기획안", `# 신규 고객 캠페인 기획안 (2판)

- 기간: 다음 달 1일부터 4주
- 목표: 신규 가입 3,000명
- 랜딩 페이지와 가입 폼은 기존 회원가입 화면을 재사용한다(새 페이지 대신 배너만 추가).
- 온라인 광고 예산 2,500만 원으로 줄임, 쿠폰은 10%로 조정
`, v1.DocumentID)
	if devWork.ID != "" {
		waitFor("an impact check for development's work", 2*time.Minute, func() bool {
			return len(need(a.ListImpactChecks(p.ID, devWork.ID, "open"))) == 1
		})
		if checks := need(a.ListImpactChecks(p.ID, plan.ID, "open")); len(checks) != 0 {
			t.Fatalf("the plan's own work was asked about its revision: %+v", checks)
		}
	}
	waitFor("the revision is judged", 10*time.Minute, func() bool { return len(judgmentsFor("캠페인 기획안 2판")) >= 2 })
	for name, j := range judgmentsFor("캠페인 기획안 2판") {
		note("QUALITY 기획안 2판 → %s: %s (%s)", name, j.Outcome, j.Reason)
	}

	// 9. Required collaboration keeps the goal open until it is met.
	req := need(a.RequestCollab(p.ID, storage.NewCollabRequest{FromWorkItemID: plan.ID, ToDepartmentID: fin.ID, Question: "광고 예산 집행을 승인해 주세요",
		Requirement: storage.CollabRequired}))
	if _, err := a.SetWorkStatus(p.ID, plan.ID, domain.WorkDone, "기획 확정"); err == nil {
		t.Fatal("the plan was completed with an unmet required request")
	}
	req = need(a.AcceptCollab(p.ID, req.ID, storage.CollabAcceptance{Title: "광고 예산 집행 승인"}))
	need(a.SetWorkStatus(p.ID, req.ResultWorkItemID, domain.WorkDone, "승인함"))
	need(a.SetWorkStatus(p.ID, plan.ID, domain.WorkDone, "기획 확정"))
	note("필수 협업: 미충족일 때 완료 거절, 수행 업무 완료 뒤 기획 완료")

	// Auto mode: development's lead AI decides a reference request, plans
	// the work, the run goes and its result is shared as a draft.
	p = need(a.UpdateProject(ProjectInput{ID: p.ID, Name: p.Name, Mode: "auto", WorkspacePath: folder, Budget: engine.Budget{MaxTokens: &tokens}}))
	need(0, a.SetAutoPolicy(p.ID, []string{conn.ID}))
	need(a.SetDepartmentAutomation(p.ID, dev.ID, domain.DepartmentAutomation{AutoStart: true, AutoShare: true, AutoCollab: true}))
	copyWork := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, ParentID: goal.ID, DepartmentID: mkt.ID, Title: "랜딩 배너 문구"}))
	ref := need(a.RequestCollab(p.ID, storage.NewCollabRequest{FromWorkItemID: copyWork.ID, ToDepartmentID: dev.ID,
		Question: "가입 이벤트 배너를 기존 회원가입 화면에 넣을 때 필요한 화면 변경 사항을 짧은 문서(마크다운)로 정리해 주세요"}))
	waitFor("development's lead AI decides", 10*time.Minute, func() bool {
		r := need(a.ListCollabRequests(p.ID, storage.CollabFilter{WorkItemID: copyWork.ID}))[0]
		return r.Status != storage.CollabRequested || r.AIStatus != ""
	})
	r := need(a.ListCollabRequests(p.ID, storage.CollabFilter{WorkItemID: copyWork.ID}))[0]
	note("QUALITY 참고용 협업 → 개발팀 대표 AI: %s by %s (%s) %s", r.Status, r.DecidedBy, r.DecisionReason, r.AIProblem)
	if r.Status == storage.CollabAccepted {
		if r.DecidedBy != "ai" {
			t.Fatalf("decided by %q", r.DecidedBy)
		}
		var st storage.AutoStart
		waitFor("the automatic start finishes", 25*time.Minute, func() bool {
			list := need(a.ListAutoStarts(p.ID, storage.AutoStartFilter{WorkItemID: r.ResultWorkItemID}))
			if len(list) == 1 {
				st = list[0]
			}
			return st.Status == storage.AutoFinished || st.Status == storage.AutoReview || st.Status == storage.AutoCancelled
		})
		note("자동 시작: %s (%s %s)", st.Status, st.Reason, st.Note)
		if st.Status == storage.AutoFinished {
			found := false
			for _, res := range need(a.PendingDecisions()).Results {
				found = found || res.WorkItemID == r.ResultWorkItemID
			}
			if !found {
				t.Fatal("the finished run's result is not waiting to be checked")
			}
		}
		need(a.ReviewCollabDecision(p.ID, ref.ID))
	}

	// A regular check of finance, judged in the background.
	sched := need(a.SaveDepartmentSchedule(domain.DepartmentSchedule{ProjectID: p.ID, DepartmentID: fin.ID, Frequency: domain.ScheduleDaily,
		TimeOfDay: "09:00", Timezone: "Asia/Seoul", Note: "이번 주 공유된 자료와 열린 업무에서 비용 확인이 필요한 일을 찾는다", Enabled: true}))
	next, _ := sched.NextSlot(time.Now())
	if n := need(work.NewScheduler(a.db, func() time.Time { return next }).Drain(ctx)); n != 1 {
		t.Fatalf("regular check ran %d slots", n)
	}
	waitFor("finance's regular check is judged", 10*time.Minute, func() bool { return len(judgmentsFor("재무팀 정기 점검")) == 1 })
	for name, j := range judgmentsFor("재무팀 정기 점검") {
		note("QUALITY 정기 점검 → %s: %s (%s)", name, j.Outcome, j.Reason)
	}

	// What it cost and what the overview shows.
	ops := need(a.OperationsOverview(p.ID))
	note("사용량: 시도 %d회, 토큰 %d+%d, 보고된 비용 $%.4f, 비용 모름 %d회, 예산 보류 %q",
		ops.Usage.Attempts, ops.Usage.InputTokens, ops.Usage.OutputTokens, ops.Usage.CostUSD, ops.Usage.UnknownCostAttempts, ops.Usage.HoldReason)
	for _, l := range ops.Departments {
		note("현황 %s: 진행 %d, 결과 확인 %d, 막힘 %d, 제안 %d, 질문 %d, 받은 협업 %d, 전달 실패 %d·보류 %d",
			l.Name, l.Open, l.ResultsToCheck, l.Blocked, l.Proposals, l.Questions, l.CollabIncoming, l.FailedDeliveries, l.HeldDeliveries)
	}
	for _, tr := range ops.Troubles {
		note("전달 문제: %s %s %s: %s", tr.DepartmentName, tr.Status, tr.EventTitle, tr.Reason)
	}
	if ops.Usage.Attempts == 0 {
		t.Fatal("no AI attempt was recorded")
	}
	note("걸린 시간: %s", time.Since(start).Round(time.Second))
}
