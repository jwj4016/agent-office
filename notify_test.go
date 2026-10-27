package main

import (
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// New to-dos are announced once, not those waiting at start, not while the
// person looks at the app, and not when notifications are off.
func TestTodoNotifications(t *testing.T) {
	a := startApp(t)
	var sent []string
	a.notify = func(title, body string) error { sent = append(sent, title+" | "+body); return nil }
	p := need(a.CreateProject(ProjectInput{Name: "캠페인"}))
	mkt := aiDepartment(t, a, p.ID, "마케팅팀", "auto")
	dev := aiDepartment(t, a, p.ID, "개발팀", "auto")
	plan := need(a.CreateWorkItem(domain.WorkItem{ProjectID: p.ID, DepartmentID: mkt.ID, Title: "캠페인 기획"}))
	ask := func(q string) {
		t.Helper()
		need(a.RequestCollab(p.ID, storage.NewCollabRequest{FromWorkItemID: plan.ID, ToDepartmentID: dev.ID, Question: q}))
	}
	check := func() {
		t.Helper()
		if err := a.checkTodos(a.ctx); err != nil {
			t.Fatal(err)
		}
	}
	ask("이미 있던 요청")
	a.SetWindowFocused(false)
	check() // the first look learns what is there
	if len(sent) != 0 {
		t.Fatalf("notified about to-dos waiting at start: %v", sent)
	}
	ask("페이지 검토")
	check()
	check() // the same to-do is not announced twice
	if len(sent) != 1 || !strings.Contains(sent[0], "내 할 일 2건") || !strings.Contains(sent[0], "마케팅팀 → 개발팀 협업 요청") {
		t.Fatalf("sent = %v", sent)
	}
	ask("배너 검토")
	ask("문구 검토")
	check()
	if len(sent) != 2 || !strings.Contains(sent[1], "외 1건") {
		t.Fatalf("two new to-dos in one notification: %v", sent)
	}

	a.SetWindowFocused(true)
	ask("보고 있을 때")
	check()
	a.SetWindowFocused(false)
	check() // seen while looking: not news later either
	need(0, a.SetSetting(storage.SettingNotifyTodos, "off"))
	ask("알림을 끈 뒤")
	check()
	if len(sent) != 2 {
		t.Fatalf("notified while looking or with notifications off: %v", sent)
	}
	need(0, a.SetSetting(storage.SettingNotifyTodos, "on"))
	ask("다시 켠 뒤")
	check()
	if len(sent) != 3 {
		t.Fatalf("notifications back on: %v", sent)
	}
}

// Before the window exists there is nothing to send with: the look still
// learns the to-dos, and nothing breaks.
func TestTodoNotificationsWithoutWindow(t *testing.T) {
	a := startApp(t)
	a.SetWindowFocused(false)
	if err := a.checkTodos(a.ctx); err != nil {
		t.Fatal(err)
	}
}
