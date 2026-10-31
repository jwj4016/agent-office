package main

import (
	"context"
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

	"github.com/zalando/go-keyring"
)

// TestRealVillaNoteRequests runs the company-requests plan's example
// (docs/plan/company-requests/plan.md §1, stage 6) with the real Claude of
// this PC's login, on a copy of real data. The copy's repository paths
// must already point inside the copy (the test refuses otherwise), so the
// person's own data is never written. Off by default:
//
//	AGENT_OFFICE_REAL_REQUESTS=sonnet AGENT_OFFICE_REAL_REQUESTS_DATA=<copied data dir> \
//	  [AGENT_OFFICE_REAL_REQUESTS_SERVICE=<service id>] [AGENT_OFFICE_REAL_REQUESTS_RUN=<run id>] \
//	  [AGENT_OFFICE_REAL_REQUESTS_OUT=<file>] go test . -run TestRealVillaNoteRequests -v -timeout 30m
//
// Mechanics must hold; what the desks decide is reported as QUALITY lines.
func TestRealVillaNoteRequests(t *testing.T) {
	model, data := os.Getenv("AGENT_OFFICE_REAL_REQUESTS"), os.Getenv("AGENT_OFFICE_REAL_REQUESTS_DATA")
	if model == "" || data == "" {
		t.Skip("set AGENT_OFFICE_REAL_REQUESTS=<model> and AGENT_OFFICE_REAL_REQUESTS_DATA=<copied data dir>")
	}
	if _, ok := connect.FindExecutable("claude"); !ok {
		t.Skip("claude CLI is not installed")
	}
	service := envOr("AGENT_OFFICE_REAL_REQUESTS_SERVICE", "prj-7olsrrxzajyaje7sqefa")
	runID := envOr("AGENT_OFFICE_REAL_REQUESTS_RUN", "run-f724riwj3z2cxry233na")
	var report []string
	note := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		report = append(report, line)
		t.Log(line)
	}
	t.Cleanup(func() {
		if out := os.Getenv("AGENT_OFFICE_REAL_REQUESTS_OUT"); out != "" {
			os.WriteFile(out, []byte(strings.Join(report, "\n")+"\n"), 0o644)
		}
	})

	keyring.MockInit()
	t.Setenv("AGENT_OFFICE_DATA_DIR", data)
	a := NewApp()
	a.emit = func(context.Context, string, ...interface{}) {}
	a.startup(context.Background())
	if st := a.SystemStatus(); st.Error != "" {
		t.Fatal(st.Error)
	}
	t.Cleanup(func() { a.shutdown(context.Background()) })
	ctx := a.ctx
	abs, _ := filepath.Abs(data)
	rows, err := a.db.Read().QueryContext(ctx, `SELECT repo FROM workspaces WHERE repo <> ''`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var repo string
		rows.Scan(&repo)
		if !strings.HasPrefix(repo, abs) {
			rows.Close()
			t.Fatalf("the copy still names a repository outside it: %s", repo)
		}
	}
	rows.Close()

	cfg, _ := json.Marshal(connect.Config{AuthMode: connect.AuthLocalLogin})
	conn := need(a.db.SaveConnection(ctx, domain.ProviderConnection{Name: "내 Claude (시험)", Provider: "claude", Config: cfg,
		VerifiedCapabilities: json.RawMessage(`{"call":true}`)}))
	people := need(a.ListAssignments(service))
	desk := need(a.SaveAssignment(domain.Assignment{ProjectID: service, RoleID: people[0].RoleID, ActorKind: domain.ActorAI,
		DisplayName: "접수 AI", ConnectionID: conn.ID, Model: model}))
	need(a.SaveIntakeDesk(domain.IntakeDesk{ProjectID: service, AssignmentID: desk.ID, Enabled: true}))
	need(a.SaveIntakeDesk(domain.IntakeDesk{ConnectionID: conn.ID, Model: model, Enabled: true}))

	settle := func(projectID, id string, limit time.Duration) domain.Request {
		t.Helper()
		deadline := time.Now().Add(limit)
		for {
			r := need(a.GetRequest(projectID, id))
			busy := r.Status == domain.ReqReceived || r.Status == domain.ReqTriaging
			for _, c := range r.Children {
				if c.Status == domain.ReqReceived || c.Status == domain.ReqTriaging {
					busy = true
				}
			}
			if !busy {
				return r
			}
			if time.Now().After(deadline) {
				t.Fatalf("request %s still %s after %s", id, r.Status, limit)
			}
			time.Sleep(time.Second)
		}
	}
	describe := func(r domain.Request) string {
		var b strings.Builder
		fmt.Fprintf(&b, "%s/%s", r.Status, r.Outcome)
		if r.Answer != "" {
			fmt.Fprintf(&b, " answer=%q", clip(r.Answer, 300))
		}
		if r.Question != "" {
			fmt.Fprintf(&b, " question=%q", r.Question)
		}
		if r.Reason != "" {
			fmt.Fprintf(&b, " reason=%q", clip(r.Reason, 300))
		}
		for _, it := range r.Items {
			fmt.Fprintf(&b, " | %s %s %s", it.Kind, it.Status, clip(string(it.Payload), 300))
		}
		return b.String()
	}
	start := time.Now()

	// 1. The company is asked for the code; it should reach 빌라노트, whose
	// desk should propose delivering the MVP run's code.
	r1 := need(a.SubmitRequest("", "빌라노트 코드 내 폴더로 보내 줘", ""))
	r1 = settle("", r1.ID, 10*time.Minute)
	note("1 company: %s", describe(r1))
	var deliver *domain.RequestItem
	for _, c := range r1.Children {
		note("1 → %s: %s", c.ProjectName, describe(c))
		for i, it := range c.Items {
			var p domain.ActionPayload
			json.Unmarshal(it.Payload, &p)
			if c.ProjectID == service && p.Action == domain.ActionDeliverCode && p.RunID == runID {
				deliver = &c.Items[i]
			}
		}
	}
	if r1.Outcome == domain.OutcomeRoute && len(r1.Children) > 0 && r1.Children[0].ProjectID == service {
		note("QUALITY ok: the company desk passed the request to 빌라노트")
	} else {
		note("QUALITY: the company desk did not pass the request to 빌라노트 alone")
	}
	if deliver == nil {
		note("QUALITY: 빌라노트's desk did not propose delivering run %s", runID)
	} else {
		note("QUALITY ok: 빌라노트's desk proposed deliver_code for the MVP run")
		folder := filepath.Join(t.TempDir(), "villanote")
		out, err := a.RunRequestAction(ActionInput{ProjectID: service, ItemID: deliver.ID, Method: engine.DeliverFolder, Folder: folder, UseAsWorkspace: true})
		if err != nil || out.Item.Status != domain.ItemDone {
			t.Fatalf("delivery: %+v %v", out.Item, err)
		}
		entries, _ := os.ReadDir(folder)
		note("1 delivered into the copy's folder: %d entries, work folder %q", len(entries), need(a.db.Project(ctx, service)).WorkspacePath)
	}

	// 2. Work from the reviews: the desk should propose work (or ask).
	r2 := need(a.SubmitRequest(service, "QA·전문가 검토에서 나온 문제 고쳐 줘", ""))
	r2 = settle(service, r2.ID, 10*time.Minute)
	note("2 service: %s", describe(r2))
	if r2.Outcome == domain.OutcomeQuestion && r2.Status == domain.ReqQuestion {
		need(0, a.AnswerRequest(service, r2.ID, "전문가 검토의 오류 1건과 기준 공백 4건을 먼저 고쳐 주세요. 공개 단계 법률 항목은 나중에 합니다."))
		r2 = settle(service, r2.ID, 10*time.Minute)
		note("2 after the answer: %s", describe(r2))
	}
	if r2.Outcome == domain.OutcomeWork || r2.Outcome == domain.OutcomeAction {
		note("QUALITY ok: the desk turned the reviews into %d items", len(r2.Items))
	} else {
		note("QUALITY: the desk did not propose work for the review findings")
	}

	// 3. A question its context can answer.
	r3 := need(a.SubmitRequest(service, "빌라노트 QA 결과를 한 문단으로 요약해 줘", ""))
	r3 = settle(service, r3.ID, 10*time.Minute)
	note("3 service: %s", describe(r3))
	if r3.Outcome == domain.OutcomeAnswer && strings.Contains(r3.Answer, "조건부") {
		note("QUALITY ok: the desk answered from the QA report's summary")
	} else {
		note("QUALITY: the answer did not come from the QA summary")
	}

	u := need(a.ProjectUsage(service))
	c := need(a.GetIntakeDesk("")).MonthUsage
	note("usage: service attempts %d (whole service), company desk %d calls $%.4f, %d unknown; took %s",
		u.Attempts, c.Attempts, c.CostUSD, c.UnknownCost, time.Since(start).Round(time.Second))
	var cost float64
	var calls, unknown int
	rows, _ = a.db.Read().QueryContext(ctx, `SELECT usage, usage_reported FROM request_attempts`)
	for rows.Next() {
		var raw string
		var rep int
		rows.Scan(&raw, &rep)
		var p struct {
			CostUSD *float64 `json:"costUsd"`
		}
		json.Unmarshal([]byte(raw), &p)
		calls++
		if rep == 0 || p.CostUSD == nil {
			unknown++
		} else {
			cost += *p.CostUSD
		}
	}
	rows.Close()
	note("request calls %d, reported cost $%.4f, unknown %d", calls, cost, unknown)
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func clip(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// TestRealDeskStaffing lets the real Claude staff the missing desks of a
// copy of real data while it takes the requests already waiting there
// (company-requests plan, 3rd revision). Nothing is approved: proposed
// actions are only reported. Off by default:
//
//	AGENT_OFFICE_REAL_STAFFING=1 AGENT_OFFICE_REAL_REQUESTS_DATA=<copied data dir> \
//	  [AGENT_OFFICE_REAL_REQUESTS_OUT=<file>] go test . -run TestRealDeskStaffing -v -timeout 30m
func TestRealDeskStaffing(t *testing.T) {
	data := os.Getenv("AGENT_OFFICE_REAL_REQUESTS_DATA")
	if os.Getenv("AGENT_OFFICE_REAL_STAFFING") == "" || data == "" {
		t.Skip("set AGENT_OFFICE_REAL_STAFFING=1 and AGENT_OFFICE_REAL_REQUESTS_DATA=<copied data dir>")
	}
	var report []string
	note := func(format string, args ...any) {
		line := fmt.Sprintf(format, args...)
		report = append(report, line)
		t.Log(line)
	}
	t.Cleanup(func() {
		if out := os.Getenv("AGENT_OFFICE_REAL_REQUESTS_OUT"); out != "" {
			os.WriteFile(out, []byte(strings.Join(report, "\n")+"\n"), 0o644)
		}
	})
	keyring.MockInit()
	t.Setenv("AGENT_OFFICE_DATA_DIR", data)
	a := NewApp()
	a.emit = func(context.Context, string, ...interface{}) {}
	a.startup(context.Background())
	if st := a.SystemStatus(); st.Error != "" {
		t.Fatal(st.Error)
	}
	t.Cleanup(func() { a.shutdown(context.Background()) })
	start := time.Now()
	deadline := start.Add(15 * time.Minute)
	var list []domain.Request
	for {
		list = need(a.ListRequests(storage.RequestFilter{All: true}))
		busy := false
		for _, r := range list {
			if r.Status == domain.ReqReceived || r.Status == domain.ReqTriaging || r.Status == domain.ReqWaitingDesk && r.StaffedDesk == "" &&
				!strings.Contains(r.HoldReason, "배치하지 못했습니다") {
				busy = true
			}
		}
		if !busy || time.Now().After(deadline) {
			break
		}
		time.Sleep(2 * time.Second)
	}
	for _, r := range list {
		note("request %q → %s/%s staffed=%q hold=%q reason=%q", clip(r.Body, 80), r.Status, r.Outcome, r.StaffedDesk, r.HoldReason, clip(r.Reason, 300))
		for _, it := range r.Items {
			note("  item %s %s %s", it.Kind, it.Status, clip(string(it.Payload), 400))
		}
		if r.ProjectID != "" {
			if v := need(a.GetIntakeDesk(r.ProjectID)); v.Assignee != nil {
				note("  desk %q (connection %s, model %q)\n%s", v.Assignee.DisplayName, v.Assignee.ConnectionID, v.Assignee.Model, v.Assignee.Overrides.Instructions)
			}
		}
	}
	rows, _ := a.db.Read().QueryContext(a.ctx, `SELECT kind, status, usage, usage_reported, error FROM desk_staffing`)
	for rows.Next() {
		var kind, status, usage, errText string
		var rep int
		rows.Scan(&kind, &status, &usage, &rep, &errText)
		note("staffing %s %s usage=%s %s", kind, status, usage, errText)
	}
	rows.Close()
	note("took %s", time.Since(start).Round(time.Second))
}
