package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestMain lets the test binary act as a fake `codex app-server`
// (FAKE_CODEX) or Claude bridge (FAKE_CLAUDE), so adapters are tested
// against real child processes and stdio.
func TestMain(m *testing.M) {
	if mode := os.Getenv("FAKE_CODEX"); mode != "" {
		fakeCodex(mode)
		os.Exit(0)
	}
	if mode := os.Getenv("FAKE_CLAUDE"); mode != "" {
		fakeClaude(mode)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func fakeCodex(mode string) {
	in := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	send := func(v map[string]any) { out.Encode(v) }
	notify := func(method string, params any) { send(map[string]any{"method": method, "params": params}) }
	completeTurn := func(status string) {
		notify("turn/completed", map[string]any{"threadId": "th1", "turn": map[string]any{"id": "tu1", "status": status, "error": nil}})
	}
	read := func() map[string]any {
		if !in.Scan() {
			os.Exit(0)
		}
		var m map[string]any
		json.Unmarshal(in.Bytes(), &m)
		return m
	}
	// Handshake shared by every mode.
	for {
		m := read()
		switch m["method"] {
		case "initialize":
			if mode == "crash" {
				fmt.Fprintln(os.Stderr, "boom: not logged in")
				os.Exit(3)
			}
			send(map[string]any{"id": m["id"], "result": map[string]any{"userAgent": "fake", "platformOs": "macos"}})
			continue
		case "initialized":
			continue
		case "thread/start":
			p := m["params"].(map[string]any)
			if p["approvalPolicy"] == nil || p["sandbox"] == nil {
				fmt.Fprintln(os.Stderr, "missing policy")
				os.Exit(4)
			}
			send(map[string]any{"id": m["id"], "result": map[string]any{"thread": map[string]any{"id": "th1"}, "model": "fake-model"}})
			continue
		case "turn/start":
			if want := os.Getenv("FAKE_EXPECT_ROOT"); want != "" {
				p, _ := m["params"].(map[string]any)
				sp, _ := p["sandboxPolicy"].(map[string]any)
				roots, _ := sp["writableRoots"].([]any)
				if sp["type"] != "workspaceWrite" || len(roots) != 1 || roots[0] != want {
					fmt.Fprintf(os.Stderr, "bad sandboxPolicy: %v\n", p["sandboxPolicy"])
					os.Exit(6)
				}
			}
			send(map[string]any{"id": m["id"], "result": map[string]any{"turn": map[string]any{"id": "tu1", "status": "inProgress"}}})
		}
		break
	}

	switch mode {
	case "success":
		notify("item/agentMessage/delta", map[string]any{"threadId": "th1", "turnId": "old", "itemId": "x", "delta": "STALE"})
		notify("item/agentMessage/delta", map[string]any{"threadId": "th1", "turnId": "tu1", "itemId": "i1", "delta": "안녕"})
		notify("item/completed", map[string]any{"threadId": "th1", "turnId": "tu1", "item": map[string]any{"type": "agentMessage", "id": "i1", "text": "안녕하세요"}})
		notify("thread/tokenUsage/updated", map[string]any{"threadId": "th1", "turnId": "tu1", "tokenUsage": map[string]any{"total": map[string]any{"inputTokens": 10, "outputTokens": 5}}})
		completeTurn("completed")
	case "approval":
		send(map[string]any{"id": 7, "method": "item/commandExecution/requestApproval", "params": map[string]any{"threadId": "th1", "turnId": "tu1", "itemId": "c1", "command": "npm test"}})
		m := read()
		decision := m["result"].(map[string]any)["decision"]
		notify("item/completed", map[string]any{"threadId": "th1", "turnId": "tu1", "item": map[string]any{"type": "agentMessage", "id": "i2", "text": fmt.Sprint("decision=", decision)}})
		completeTurn("completed")
	case "interruptible":
		for {
			m := read()
			if m["method"] == "turn/interrupt" {
				completeTurn("interrupted")
			}
		}
	case "stuck":
		// Ignores interrupt and owns a grandchild process that cancel must also stop.
		child := exec.Command("sleep", "300")
		child.Start()
		os.WriteFile(os.Getenv("FAKE_PIDFILE"), []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		time.Sleep(time.Hour)
	case "unknown-request":
		send(map[string]any{"id": 9, "method": "item/tool/call", "params": map[string]any{}})
		m := read()
		if m["error"] == nil {
			fmt.Fprintln(os.Stderr, "expected error reply")
			os.Exit(5)
		}
		completeTurn("completed")
	case "failed-turn":
		notify("turn/completed", map[string]any{"threadId": "th1", "turn": map[string]any{"id": "tu1", "status": "failed", "error": map[string]any{"message": "usage limit"}}})
	}
	// Stay alive until stdin closes, like the real server.
	for in.Scan() {
	}
}

func fakeCodexProvider(t *testing.T, mode string, extraEnv ...string) *Codex {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "FAKE_CODEX="+mode)
	return &Codex{Executable: exe, Env: append(env, extraEnv...), InterruptGrace: 300 * time.Millisecond}
}

func codexReq() StartRequest {
	return StartRequest{ProjectID: "p1", RunID: "r1", StepAttemptID: "s1", Prompt: "인사해줘",
		Workspace: os.TempDir(), Policy: Policy{Sandbox: "read-only", AskApproval: true}}
}

func payload[T any](t *testing.T, ev Event) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(ev.Payload, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCodexSuccess(t *testing.T) {
	s, err := fakeCodexProvider(t, "success").Start(context.Background(), codexReq())
	if err != nil {
		t.Fatal(err)
	}
	evs := drain(t, s)
	var kinds []string
	for _, ev := range evs {
		kinds = append(kinds, ev.Kind)
		if ev.Kind == KindMessageDelta && payload[TextPayload](t, ev).Text == "STALE" {
			t.Fatal("delta from another turn leaked into this attempt")
		}
	}
	if got := strings.Join(kinds, ","); got != "started,message_delta,message,usage,completed" {
		t.Fatalf("kinds = %s", got)
	}
	if st := payload[StartedPayload](t, evs[0]); st.SessionID != "th1" || st.Model != "fake-model" {
		t.Fatalf("started = %+v", st)
	}
	c := completed(t, evs)
	if c.Status != StatusSucceeded || c.Text != "안녕하세요" {
		t.Fatalf("completed = %+v", c)
	}
}

func TestCodexApprovalRoundTrip(t *testing.T) {
	for _, decision := range []string{DecisionAccept, DecisionDecline} {
		t.Run(decision, func(t *testing.T) {
			ctx := context.Background()
			s, _ := fakeCodexProvider(t, "approval").Start(ctx, codexReq())
			next(t, s) // started
			ev := next(t, s)
			req := payload[RequestPayload](t, ev)
			if ev.Kind != KindApprovalRequest || req.Detail != "npm test" {
				t.Fatalf("got %s %+v", ev.Kind, req)
			}
			if err := s.Respond(ctx, Response{RequestID: req.RequestID, Decision: decision}); err != nil {
				t.Fatal(err)
			}
			if err := s.Respond(ctx, Response{RequestID: req.RequestID, Decision: decision}); err == nil {
				t.Fatal("duplicate response accepted")
			}
			evs := drain(t, s)
			if c := completed(t, evs); c.Text != "decision="+decision {
				t.Fatalf("server saw %q", c.Text)
			}
		})
	}
}

func TestCodexCancelInterruptsTurn(t *testing.T) {
	ctx := context.Background()
	s, _ := fakeCodexProvider(t, "interruptible").Start(ctx, codexReq())
	next(t, s)
	time.Sleep(50 * time.Millisecond) // let turn/start's reply land
	s.Cancel(ctx)
	if c := completed(t, drain(t, s)); c.Status != StatusCancelled {
		t.Fatalf("status = %s", c.Status)
	}
}

func TestCodexCrashReportsStderr(t *testing.T) {
	s, _ := fakeCodexProvider(t, "crash").Start(context.Background(), codexReq())
	c := completed(t, drain(t, s))
	if c.Status != StatusFailed || !strings.Contains(c.Error, "not logged in") {
		t.Fatalf("completed = %+v", c)
	}
}

func TestCodexRefusesUnknownServerRequest(t *testing.T) {
	s, _ := fakeCodexProvider(t, "unknown-request").Start(context.Background(), codexReq())
	evs := drain(t, s)
	if c := completed(t, evs); c.Status != StatusSucceeded {
		t.Fatalf("fake server did not get an error reply: %+v", c)
	}
	sawRefusal := false
	for _, ev := range evs {
		if ev.Kind == KindError && strings.Contains(payload[ErrorPayload](t, ev).Message, "item/tool/call") {
			sawRefusal = true
		}
	}
	if !sawRefusal {
		t.Fatal("refusal not surfaced as an event")
	}
}

func TestCodexFailedTurn(t *testing.T) {
	s, _ := fakeCodexProvider(t, "failed-turn").Start(context.Background(), codexReq())
	if c := completed(t, drain(t, s)); c.Status != StatusFailed || c.Error != "usage limit" {
		t.Fatalf("completed = %+v", c)
	}
}

// TestCodexReal runs one tiny turn against the installed codex CLI.
// It is opt-in because it uses the user's account and quota.
func TestCodexReal(t *testing.T) {
	exe := os.Getenv("AGENT_OFFICE_CODEX_IT")
	if exe == "" {
		t.Skip("set AGENT_OFFICE_CODEX_IT=/path/to/codex to run against the real CLI")
	}
	dir := t.TempDir()
	req := codexReq()
	req.Workspace = dir
	req.Prompt = "Reply with exactly the word: pong. Do not run any commands."
	req.Limits.Timeout = 2 * time.Minute
	s, err := (&Codex{Executable: exe, Version: "it"}).Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	var evs []Event
	timeout := time.After(3 * time.Minute)
	for done := false; !done; {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				done = true
				break
			}
			evs = append(evs, ev)
			t.Logf("%s %s", ev.Kind, truncate(string(ev.Payload), 200))
			if ev.Kind == KindApprovalRequest {
				s.Respond(context.Background(), Response{RequestID: payload[RequestPayload](t, ev).RequestID, Decision: DecisionDecline})
			}
		case <-timeout:
			t.Fatal("timed out")
		}
	}
	if c := completed(t, evs); c.Status != StatusSucceeded || !strings.Contains(strings.ToLower(c.Text), "pong") {
		t.Fatalf("completed = %+v", c)
	}
}

// The attempt's output folder is granted as the only extra writable root.
func TestCodexGrantsOutputFolder(t *testing.T) {
	out := t.TempDir()
	req := codexReq()
	req.Policy.Sandbox = "workspace-write"
	req.WritableDirs = []string{out}
	s, err := fakeCodexProvider(t, "success", "FAKE_EXPECT_ROOT="+out).Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if c := completed(t, drain(t, s)); c.Status != StatusSucceeded {
		t.Fatalf("completed = %+v", c)
	}
}
