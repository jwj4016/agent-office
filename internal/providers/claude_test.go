package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClaude imitates runners/claude/dist/main.js over stdio.
func fakeClaude(mode string) {
	in := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)
	emit := func(kind string, payload any) {
		out.Encode(map[string]any{"type": "event", "kind": kind, "payload": payload})
	}
	read := func() map[string]any {
		if !in.Scan() {
			os.Exit(0)
		}
		var m map[string]any
		json.Unmarshal(in.Bytes(), &m)
		return m
	}
	start := read()
	if start["type"] != "start" {
		fmt.Fprintln(os.Stderr, "first command was not start")
		os.Exit(2)
	}
	switch mode {
	case "success":
		emit("started", map[string]any{"sessionId": "sess-1", "model": "claude-test"})
		emit("message", map[string]any{"text": fmt.Sprint("prompt=", start["prompt"])})
		emit("usage", map[string]any{"inputTokens": 3, "outputTokens": 2, "costUsd": 0.001})
		emit("completed", map[string]any{"status": "succeeded", "text": fmt.Sprint("ask=", start["askApproval"])})
	case "approval":
		emit("approval_request", map[string]any{"requestId": "claude-1", "action": "Bash", "detail": "npm test"})
		m := read()
		emit("request_resolved", map[string]any{"requestId": m["requestId"], "decision": m["decision"]})
		emit("completed", map[string]any{"status": "succeeded", "text": fmt.Sprint("decision=", m["decision"])})
	case "cancel-honored":
		emit("started", map[string]any{"sessionId": "sess-1"})
		for read()["type"] != "cancel" {
		}
		emit("completed", map[string]any{"status": "cancelled"})
	case "cancel-ignored":
		emit("started", map[string]any{"sessionId": "sess-1"})
		time.Sleep(time.Hour)
	case "env-check":
		key := "unset"
		if os.Getenv("ANTHROPIC_API_KEY") != "" {
			key = "set:" + strings.Repeat("*", len(os.Getenv("ANTHROPIC_API_KEY")))
		}
		emit("completed", map[string]any{"status": "succeeded", "text": fmt.Sprint(key, " dirs=", start["writableDirs"])})
	case "orphan":
		// Like the real SDK: a child process outlives the bridge's exit.
		child := exec.Command("sleep", "300")
		child.Start()
		os.WriteFile(os.Getenv("FAKE_PIDFILE"), []byte(fmt.Sprint(child.Process.Pid)), 0o600)
		emit("started", map[string]any{"sessionId": "sess-1"})
		for read()["type"] != "cancel" {
		}
		emit("completed", map[string]any{"status": "cancelled"})
		os.Exit(0)
	case "crash":
		fmt.Fprintln(os.Stderr, "Error: Claude Code executable not found")
		os.Exit(1)
	}
	for in.Scan() {
	}
}

func fakeClaudeProvider(t *testing.T, mode string) *ClaudeBridge {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return &ClaudeBridge{Node: exe, Script: "bridge", Env: append(os.Environ(), "FAKE_CLAUDE="+mode), CancelGrace: 300 * time.Millisecond}
}

func TestClaudeSuccess(t *testing.T) {
	r := codexReq()
	r.Policy.AskApproval = false
	s, err := fakeClaudeProvider(t, "success").Start(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	evs := drain(t, s)
	var kinds []string
	for _, ev := range evs {
		kinds = append(kinds, ev.Kind)
	}
	if got := strings.Join(kinds, ","); got != "started,message,usage,completed" {
		t.Fatalf("kinds = %s", got)
	}
	if m := payload[TextPayload](t, evs[1]); m.Text != "prompt=인사해줘" {
		t.Fatalf("prompt not passed: %q", m.Text)
	}
	if c := completed(t, evs); c.Status != StatusSucceeded || c.Text != "ask=false" {
		t.Fatalf("completed = %+v", c)
	}
	if u := payload[UsagePayload](t, evs[2]); u.CostUSD == nil || *u.CostUSD != 0.001 {
		t.Fatalf("usage = %+v", u)
	}
}

func TestClaudeApprovalRoundTrip(t *testing.T) {
	ctx := context.Background()
	s, _ := fakeClaudeProvider(t, "approval").Start(ctx, codexReq())
	ev := next(t, s)
	req := payload[RequestPayload](t, ev)
	if ev.Kind != KindApprovalRequest || req.RequestID != "claude-1" {
		t.Fatalf("got %s %+v", ev.Kind, req)
	}
	if err := s.Respond(ctx, Response{RequestID: "claude-9"}); err == nil {
		t.Fatal("unknown request accepted")
	}
	if err := s.Respond(ctx, Response{RequestID: "claude-1", Decision: DecisionDecline}); err != nil {
		t.Fatal(err)
	}
	if c := completed(t, drain(t, s)); c.Text != "decision=decline" {
		t.Fatalf("bridge saw %q", c.Text)
	}
}

func TestClaudeCancel(t *testing.T) {
	for _, mode := range []string{"cancel-honored", "cancel-ignored"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			s, _ := fakeClaudeProvider(t, mode).Start(ctx, codexReq())
			next(t, s)
			s.Cancel(ctx)
			evs := drain(t, s)
			if c := completed(t, evs); c.Status != StatusCancelled {
				t.Fatalf("completed = %+v", c)
			}
		})
	}
}

func TestClaudeCrashReportsStderr(t *testing.T) {
	s, _ := fakeClaudeProvider(t, "crash").Start(context.Background(), codexReq())
	if c := completed(t, drain(t, s)); c.Status != StatusFailed || !strings.Contains(c.Error, "executable not found") {
		t.Fatalf("completed = %+v", c)
	}
}

// TestClaudeReal runs one tiny turn through the real bridge and SDK.
// Opt-in: it uses the user's Claude credentials and quota.
func TestClaudeReal(t *testing.T) {
	if os.Getenv("AGENT_OFFICE_CLAUDE_IT") != "1" {
		t.Skip("set AGENT_OFFICE_CLAUDE_IT=1 (after `npm run build` in runners/claude) to run against the real SDK")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	script, _ := filepath.Abs("../../runners/claude/dist/main.js")
	req := codexReq()
	req.Workspace = t.TempDir()
	req.Prompt = "Reply with exactly the word: pong. Do not use any tools."
	req.Policy.AskApproval = false
	s, err := (&ClaudeBridge{Node: node, Script: script}).Start(context.Background(), req)
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
		case <-timeout:
			s.Cancel(context.Background())
			t.Fatal("timed out")
		}
	}
	if c := completed(t, evs); c.Status != StatusSucceeded || !strings.Contains(strings.ToLower(c.Text), "pong") {
		t.Fatalf("completed = %+v", c)
	}
}

func TestClaudeAuthAndWritableDirs(t *testing.T) {
	r := codexReq()
	r.WritableDirs = []string{"/data/out"}
	// Local login: an inherited key is removed so the CLI login is used.
	p := fakeClaudeProvider(t, "env-check")
	p.Env = append(p.Env, "ANTHROPIC_API_KEY=from-shell")
	s, _ := p.Start(context.Background(), r)
	if c := completed(t, drain(t, s)); c.Text != "unset dirs=[/data/out]" {
		t.Fatalf("local login: %q", c.Text)
	}
	// API key mode: only the configured key reaches the bridge.
	p = fakeClaudeProvider(t, "env-check")
	p.APIKey = "sk-ant-test"
	s, _ = p.Start(context.Background(), r)
	if c := completed(t, drain(t, s)); c.Text != "set:*********** dirs=[/data/out]" {
		t.Fatalf("api key: %q", c.Text)
	}
}
