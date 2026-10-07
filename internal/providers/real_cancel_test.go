//go:build !windows

package providers

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// realCancel starts a real provider on a task long enough to still be
// running, cancels it once it has started, and checks that it ends as
// cancelled and leaves no process behind. Opt-in: uses the person's quota.
func realCancel(t *testing.T, p Provider, marker string) {
	t.Helper()
	req := StartRequest{ProjectID: "p", RunID: "r", StepAttemptID: "s-" + marker, Workspace: t.TempDir(),
		Prompt: "Count slowly from 1 to 500, writing each number on its own line, and explain each number briefly. " + marker,
		Policy: Policy{Sandbox: "read-only"}}
	s, err := p.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	pgid := groupOf(t, s)
	deadline := time.After(2 * time.Minute)
	for started := false; !started; {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				t.Fatal("ended before it could be cancelled")
			}
			if ev.Kind == KindMessageDelta || ev.Kind == KindMessage || ev.Kind == KindUsage || ev.Kind == KindTool {
				started = true
			}
		case <-deadline:
			t.Fatal("never started producing output")
		}
	}
	begin := time.Now()
	s.Cancel(context.Background())
	var last CompletedPayload
	for ev := range s.Events() {
		if ev.Kind == KindCompleted {
			last = completedOf(ev)
		}
	}
	t.Logf("cancel took %v, status %s", time.Since(begin), last.Status)
	if last.Status != StatusCancelled && last.Status != StatusInterrupted {
		t.Fatalf("status after cancel = %+v", last)
	}
	// The child runs in its own process group: after cancel, nothing in
	// that group may be left (the CLI or anything it started).
	time.Sleep(2 * time.Second)
	if err := syscall.Kill(-pgid, 0); err == nil {
		out, _ := exec.Command("pgrep", "-g", strconv.Itoa(pgid)).Output()
		t.Fatalf("processes still running in group %d after cancel: %s", pgid, out)
	}
}

func groupOf(t *testing.T, s Session) int {
	t.Helper()
	var pr *proc
	switch v := s.(type) {
	case *codexSession:
		pr = v.proc
	case *claudeSession:
		pr = v.proc
	default:
		t.Fatalf("unexpected session %T", s)
	}
	return pr.cmd.Process.Pid // Setpgid: the group id is the child's pid
}

func TestCodexRealCancel(t *testing.T) {
	exe := os.Getenv("AGENT_OFFICE_CODEX_IT")
	if exe == "" {
		t.Skip("set AGENT_OFFICE_CODEX_IT=/path/to/codex")
	}
	realCancel(t, &Codex{Executable: exe, Version: "it"}, "AOCANCELCODEX")
}

func TestClaudeRealCancel(t *testing.T) {
	if os.Getenv("AGENT_OFFICE_CLAUDE_IT") != "1" {
		t.Skip("set AGENT_OFFICE_CLAUDE_IT=1")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Fatal(err)
	}
	script, _ := filepath.Abs("../../runners/claude/dist/main.js")
	realCancel(t, &ClaudeBridge{Node: node, Script: script}, "AOCANCELCLAUDE")
}

func completedOf(ev Event) CompletedPayload {
	var p CompletedPayload
	json.Unmarshal(ev.Payload, &p)
	return p
}
