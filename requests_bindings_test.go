package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/workspace"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// Requests through the bindings: without a desk a request waits and shows
// as a to-do; an approved action runs once, a closed save dialog runs
// nothing, and another service's item is refused.
func TestBindingsRequestActions(t *testing.T) {
	a := startApp(t)
	villa, _ := buildService(t, a, "빌라노트")
	other, _ := buildService(t, a, "블로그")
	r := need(a.SubmitRequest(villa.ID, "서비스 백업해 줘", ""))
	if _, err := a.SubmitRequest(villa.ID, "  ", ""); err == nil {
		t.Fatal("an empty request was taken")
	}
	ctx := context.Background()
	a.intake.Drain(ctx)
	d := need(a.PendingDecisions())
	if len(d.Requests) != 1 || d.Requests[0].Status != domain.ReqWaitingDesk {
		t.Fatalf("to-dos = %+v", d.Requests)
	}
	if list, _ := a.todos(ctx); len(list) == 0 || list[len(list)-1].key != "request:"+r.ID+":waiting_desk:1" {
		t.Fatalf("todo notifications = %+v", list)
	}

	// The desk decided on an export (as a desk's AI would).
	if ok, err := a.db.ClaimRequest(ctx, r.ID); !ok || err != nil {
		t.Fatal(ok, err)
	}
	if err := a.db.ApplyIntakeDecision(ctx, r.ID, domain.IntakeDecision{Outcome: domain.OutcomeAction, Reason: "백업",
		Actions: []domain.ActionPayload{{Action: domain.ActionExportService, Reason: "키 없이 내보냄"}}}, false); err != nil {
		t.Fatal(err)
	}
	item := need(a.GetRequest(villa.ID, r.ID)).Items[0]

	if _, err := a.RunRequestAction(ActionInput{ProjectID: other.ID, ItemID: item.ID}); err == nil {
		t.Fatal("ran another service's action")
	}
	a.saveDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) { return "", nil }
	if out := need(a.RunRequestAction(ActionInput{ProjectID: villa.ID, ItemID: item.ID})); !out.Cancelled || out.Item.Status != domain.ItemProposed {
		t.Fatalf("cancelled dialog = %+v", out)
	}
	dst := filepath.Join(t.TempDir(), "백업.zip")
	a.saveDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) { return dst, nil }
	out := need(a.RunRequestAction(ActionInput{ProjectID: villa.ID, ItemID: item.ID}))
	if out.Cancelled || out.Item.Status != domain.ItemDone {
		t.Fatalf("export = %+v", out)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Fatal("the archive was not written")
	}
	if got := need(a.GetRequest(villa.ID, r.ID)); got.Status != domain.ReqDone {
		t.Fatalf("request after the action = %+v", got)
	}
	if _, err := a.RunRequestAction(ActionInput{ProjectID: villa.ID, ItemID: item.ID}); err == nil {
		t.Fatal("the same action ran twice")
	}
	if len(need(a.PendingDecisions()).Requests) != 0 {
		t.Fatal("a finished request is still a to-do")
	}
}

// The representative example (plan §1) with the test provider: a company
// request goes to 빌라노트, whose desk proposes handing over the run's
// code; the person approves with a folder, which becomes the work folder,
// and the next run starts from that code.
func TestBindingsCompanyRequestDeliversCode(t *testing.T) {
	if workspaceGit() == nil {
		t.Skip("git is not installed")
	}
	a := startApp(t)
	villa, version := buildService(t, a, "빌라노트")
	conn := need(a.EnsureTestConnection())
	for _, as := range need(a.ListAssignments(villa.ID)) {
		if as.DisplayName == "백엔드 AI" {
			as.Model = "backend-file"
			need(a.SaveAssignment(as))
		}
	}
	// The version snapshots its assignees: confirm again after the change.
	wf := need(a.ListWorkflows(villa.ID))[0]
	version = need(a.ConfirmVersion(villa.ID, wf.ID, wf.Revision)).Version.ID
	a.testProvider.Scripts["backend-file"] = []providers.Step{
		{File: &providers.FileStep{Path: "api/api.go", Content: "package api\n"}},
		{Write: &providers.WriteStep{Key: "change", Content: `{"summary":"API"}`}},
		{Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded, Text: "완료"}},
	}
	runID := need(a.StartRun(villa.ID, version)).RunID
	ap := waitInbox(t, a, villa.ID, engine.InboxApproval)
	need(a.DecideApproval(villa.ID, ap.ApprovalID, ap.Generation, engine.ApprovalApproved, "진행", nil))
	rv := waitInbox(t, a, villa.ID, engine.InboxReview)
	need(a.SubmitReview(villa.ID, rv.AttemptID, rv.Generation, engine.ReviewPass, "통과", nil, nil))
	waitUntil(t, "run finished", func() bool { return need(a.GetRun(villa.ID, runID)).Status == engine.RunSucceeded })
	if fc := need(a.FinalCode(villa.ID, runID)); fc.Included || !fc.AppRepo {
		t.Fatalf("final code before delivery = %+v", fc)
	}

	// Desks: the company desk passes the request on; 빌라노트's desk (a new
	// AI assignee) proposes delivering the code.
	desk := need(a.SaveAssignment(domain.Assignment{ProjectID: villa.ID, RoleID: need(a.ListAssignments(villa.ID))[0].RoleID, ActorKind: domain.ActorAI,
		DisplayName: "접수 AI", ConnectionID: conn.ID, Model: "villa-desk"}))
	need(a.SaveIntakeDesk(domain.IntakeDesk{ProjectID: villa.ID, AssignmentID: desk.ID, Enabled: true}))
	need(a.SaveIntakeDesk(domain.IntakeDesk{ConnectionID: conn.ID, Model: "company-desk", Enabled: true}))
	result := func(v any) []providers.Step {
		b, _ := json.Marshal(v)
		return []providers.Step{{Write: &providers.WriteStep{Key: "result", Content: string(b)}},
			{Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded, Text: "완료"}}}
	}
	a.testProvider.Scripts["company-desk"] = result(map[string]any{"outcome": "route", "reason": "빌라노트의 일",
		"routes": []map[string]any{{"projectId": villa.ID, "body": "MVP 코드를 대표에게 넘겨 줘"}}})
	a.testProvider.Scripts["villa-desk"] = result(map[string]any{"outcome": "action", "reason": "코드가 기준 코드에 없음",
		"actions": []map[string]any{{"action": "deliver_code", "runId": runID}}})

	req := need(a.SubmitRequest("", "빌라노트 코드 내 폴더로 보내 줘", ""))
	var child domain.Request
	waitUntil(t, "service desk decided", func() bool {
		r := need(a.GetRequest("", req.ID))
		if len(r.Children) == 1 {
			child = r.Children[0]
		}
		return child.Status == domain.ReqProposed
	})
	if len(child.Items) != 1 || child.Items[0].Kind != domain.ItemAction {
		t.Fatalf("service request = %+v", child)
	}
	folder := filepath.Join(t.TempDir(), "villanote")
	out := need(a.RunRequestAction(ActionInput{ProjectID: villa.ID, ItemID: child.Items[0].ID, Method: engine.DeliverFolder,
		Folder: folder, UseAsWorkspace: true}))
	if out.Item.Status != domain.ItemDone {
		t.Fatalf("delivery = %+v", out.Item)
	}
	if _, err := os.Stat(filepath.Join(folder, "api", "api.go")); err != nil {
		t.Fatal("the code was not copied out")
	}
	if p := need(a.db.Project(context.Background(), villa.ID)); p.WorkspacePath != folder {
		t.Fatalf("work folder = %q", p.WorkspacePath)
	}
	if r := need(a.GetRequest("", req.ID)); r.Status != domain.ReqDone {
		t.Fatalf("company request = %+v", r)
	}
	if fc := need(a.FinalCode(villa.ID, runID)); !fc.Included || len(fc.Deliveries) != 1 || fc.Deliveries[0].RequestItemID != child.Items[0].ID {
		t.Fatalf("after delivery = %+v", fc)
	}
	// A version keeps the work folder it was confirmed with: rerun, it
	// works in the app repository, whose base is now the delivered code.
	commit := fc(t, a, villa.ID, runID)
	old := need(a.StartRun(villa.ID, version)).RunID
	if repo := need(a.GetRun(villa.ID, old)).Repo; repo == nil || repo.Base != commit {
		t.Fatalf("rerun of the old version = %+v, want base %s", repo, commit)
	}
	need(0, a.CancelRun(villa.ID, old))
	// A version confirmed now works in the new folder, from the same code.
	wf = need(a.ListWorkflows(villa.ID))[0]
	fresh := need(a.ConfirmVersion(villa.ID, wf.ID, wf.Revision)).Version.ID
	next := need(a.StartRun(villa.ID, fresh)).RunID
	if repo := need(a.GetRun(villa.ID, next)).Repo; repo == nil || !sameDirForTest(repo.Path, folder) || repo.Base != commit {
		t.Fatalf("next run repo = %+v, want %s at %s", repo, folder, commit)
	}
}

func fc(t *testing.T, a *App, projectID, runID string) string {
	t.Helper()
	return need(a.FinalCode(projectID, runID)).Commit
}

func waitUntil(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(e2eWait)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting: %s", what)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func sameDirForTest(a, b string) bool {
	ra, _ := filepath.EvalSymlinks(a)
	rb, _ := filepath.EvalSymlinks(b)
	return ra == rb
}

func workspaceGit() *workspace.Git { return workspace.FindGit() }

// Desks through the bindings: a desk assignee written by hand joins the
// shared desk role; staffing that the AI cannot answer reports why; the
// desk view names the connection a new desk would get.
func TestBindingsDeskAssignees(t *testing.T) {
	a := startApp(t)
	villa, _ := buildService(t, a, "빌라노트")
	conn := need(a.EnsureTestConnection())
	v := need(a.GetIntakeDesk(villa.ID))
	if v.Exists || v.SuggestedConnectionID != conn.ID {
		t.Fatalf("desk view before = %+v", v)
	}
	if _, err := a.StaffIntakeDesk(villa.ID); err == nil || !strings.Contains(err.Error(), "접수 담당") {
		t.Fatalf("staffing with a provider that writes no desk: %v", err)
	}
	need(a.CreateIntakeAssignee(NewDeskInput{ProjectID: villa.ID, Name: "빌라노트 접수 담당", ConnectionID: conn.ID, Instructions: "법률 판단은 대표에게"}))
	v = need(a.GetIntakeDesk(villa.ID))
	if !v.Exists || v.Assignee == nil || v.Assignee.DisplayName != "빌라노트 접수 담당" || v.Assignee.Overrides.Instructions != "법률 판단은 대표에게" {
		t.Fatalf("desk view after = %+v", v)
	}
	if v.LastStaffing == nil || v.LastStaffing.Status != "failed" {
		t.Fatalf("last staffing = %+v", v.LastStaffing)
	}
	roles := 0
	for _, r := range need(a.ListRoles()) {
		if r.Name == "접수 담당" {
			roles++
		}
	}
	if roles != 1 {
		t.Fatalf("desk roles = %d", roles)
	}
	if _, err := a.CreateIntakeAssignee(NewDeskInput{ProjectID: villa.ID, Name: " ", ConnectionID: conn.ID}); err == nil {
		t.Fatal("a nameless desk was made")
	}
}

// Organization proposals through the bindings: a provider that writes no
// organization leaves a failed proposal the screen can show; a proposal of
// one service cannot be applied in another.
func TestBindingsOrganization(t *testing.T) {
	a := startApp(t)
	villa, _ := buildService(t, a, "빌라노트")
	blog, _ := buildService(t, a, "블로그")
	if v := need(a.LatestOrgProposal(villa.ID)); v.Proposal != nil || len(v.Names) == 0 {
		t.Fatalf("before = %+v", v)
	}
	if _, err := a.ProposeOrganization(villa.ID); err == nil {
		t.Fatal("an empty organization was accepted")
	}
	v := need(a.LatestOrgProposal(villa.ID))
	if v.Proposal == nil || v.Proposal.Status != "failed" {
		t.Fatalf("after = %+v", v.Proposal)
	}
	if _, err := a.ApplyOrganization(blog.ID, v.Proposal.ID, nil); err == nil {
		t.Fatal("applied another service's proposal")
	}
	if _, err := a.LatestOrgProposal("prj-none"); err == nil {
		t.Fatal("read an unknown service")
	}
}
