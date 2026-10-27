package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"agent-office/internal/engine"
	"agent-office/internal/storage"
)

// EventOpenInbox asks the window to show the inbox (a notification was
// clicked).
const EventOpenInbox = "app.openInbox"

// todo is one thing in the person's inbox, named for a notification.
type todo struct{ key, title string }

// todoWatcher notices new to-dos. The first look only learns what is
// already there: to-dos waiting at start are not news.
type todoWatcher struct {
	mu      sync.Mutex
	seen    map[string]bool
	focused bool
	wake    chan struct{}
}

func newTodoWatcher() *todoWatcher {
	return &todoWatcher{focused: true, wake: make(chan struct{}, 1)}
}

// Wake asks for a look soon; safe in the storage subscriber callback.
func (w *todoWatcher) Wake() {
	if w == nil {
		return
	}
	select {
	case w.wake <- struct{}{}:
	default:
	}
}

// fresh remembers the current to-dos and returns those it had not seen.
func (w *todoWatcher) fresh(now []todo) []todo {
	w.mu.Lock()
	defer w.mu.Unlock()
	first := w.seen == nil
	var out []todo
	seen := make(map[string]bool, len(now))
	for _, t := range now {
		seen[t.key] = true
		if !first && !w.seen[t.key] {
			out = append(out, t)
		}
	}
	w.seen = seen
	return out
}

func (w *todoWatcher) setFocused(f bool) {
	w.mu.Lock()
	w.focused = f
	w.mu.Unlock()
}

func (w *todoWatcher) isFocused() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.focused
}

var inboxKindName = map[string]string{
	engine.InboxApproval: "승인", engine.InboxReview: "리뷰", engine.InboxTask: "작업", engine.InboxToolApproval: "도구 사용 승인",
	engine.InboxQuestion: "AI의 질문", engine.InboxBudget: "예산 확인",
}

// todos lists everything the inbox shows, each with a stable key.
func (a *App) todos(ctx context.Context) ([]todo, error) {
	items, err := a.eng.Inbox(ctx)
	if err != nil {
		return nil, err
	}
	var out []todo
	for _, it := range items {
		out = append(out, todo{key: fmt.Sprintf("run:%s:%s:%s:%s", it.Kind, it.AttemptID, it.ApprovalID, it.MessageID),
			title: fmt.Sprintf("‘%s’ %s", it.StepTitle, inboxKindName[it.Kind])})
	}
	d, err := a.PendingDecisions()
	if err != nil {
		return nil, err
	}
	for _, r := range d.Results {
		out = append(out, todo{"result:" + r.WorkItemID + ":" + r.RunID, fmt.Sprintf("‘%s’ 결과 검토", r.Title)})
	}
	for _, r := range d.Collabs {
		out = append(out, todo{"collab:" + r.ID, fmt.Sprintf("%s → %s 협업 요청", r.FromDepartmentName, r.ToDepartmentName)})
	}
	for _, r := range d.AIDecisions {
		out = append(out, todo{"ai:" + r.ID, fmt.Sprintf("%s 대표 AI의 협업 결정", r.ToDepartmentName)})
	}
	for _, n := range d.Notices {
		out = append(out, todo{"notice:" + n.ID, fmt.Sprintf("‘%s’ 영향 확인", n.WorkTitle)})
	}
	for _, s := range d.AutoReviews {
		out = append(out, todo{"auto:" + s.ID, fmt.Sprintf("‘%s’ 자동 시작 검토", s.WorkTitle)})
	}
	for _, j := range d.Questions {
		out = append(out, todo{"question:" + j.ID, fmt.Sprintf("%s의 질문", j.DepartmentName)})
	}
	for _, p := range d.Proposals {
		out = append(out, todo{"proposal:" + p.ID, fmt.Sprintf("%s 업무 제안 ‘%s’", p.DepartmentName, p.Title)})
	}
	return out, nil
}

// notifyTodosOn reports whether to-do notifications are on (the default).
func (a *App) notifyTodosOn(ctx context.Context) bool {
	s, err := a.db.Settings(ctx)
	return err != nil || s[storage.SettingNotifyTodos] != "off"
}

// checkTodos notifies about to-dos that arrived since the last look, unless
// notifications are off, unavailable (no window yet) or the person is
// looking at the app.
func (a *App) checkTodos(ctx context.Context) error {
	list, err := a.todos(ctx)
	if err != nil {
		return err
	}
	fresh := a.watcher.fresh(list)
	if len(fresh) == 0 || a.notify == nil || a.watcher.isFocused() || !a.notifyTodosOn(ctx) {
		return nil
	}
	body := fresh[0].title
	if len(fresh) > 1 {
		body += fmt.Sprintf(" 외 %d건", len(fresh)-1)
	}
	return a.notify(fmt.Sprintf("내 할 일 %d건이 기다립니다", len(list)), body+"이 생겼습니다")
}

// watchTodos looks for new to-dos after stored events (a little later,
// so a burst is one notification) and every half minute.
func (a *App) watchTodos(ctx context.Context) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		if err := a.checkTodos(ctx); err != nil && ctx.Err() == nil {
			log.Printf("todo notifications: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		case <-a.watcher.wake:
			select {
			case <-ctx.Done():
				return
			case <-time.After(1500 * time.Millisecond):
			}
		}
	}
}

// SetWindowFocused tells the app whether the person is looking at it; no
// notification is sent while they are.
func (a *App) SetWindowFocused(focused bool) {
	a.watcher.setFocused(focused)
}

// domReady starts to-do notifications once the window exists (the OS
// notification service needs it). A click on a notification opens the
// inbox.
func (a *App) domReady(ctx context.Context) {
	if a.initErr != nil || a.db == nil {
		return
	}
	a.notifyOnce.Do(func() {
		if err := runtime.InitializeNotifications(ctx); err != nil {
			log.Printf("notifications unavailable: %v", err)
			return
		}
		runtime.OnNotificationResponse(ctx, func(runtime.NotificationResult) {
			runtime.WindowUnminimise(ctx)
			runtime.Show(ctx)
			a.emit(ctx, EventOpenInbox)
		})
		authorized := false
		a.notify = func(title, body string) error {
			if !authorized {
				ok, err := runtime.CheckNotificationAuthorization(ctx)
				if err == nil && !ok {
					ok, err = runtime.RequestNotificationAuthorization(ctx)
				}
				if err != nil || !ok {
					return err
				}
				authorized = true
			}
			return runtime.SendNotification(ctx, runtime.NotificationOptions{ID: fmt.Sprintf("todo-%d", time.Now().UnixNano()), Title: title, Body: body})
		}
		go a.watchTodos(a.ectx)
	})
}
