package connect

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeCLI writes a shell script standing in for codex/claude.
func fakeCLI(t *testing.T, dir, name, body string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("shell script fakes")
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func stepStatus(r Report, id string) string {
	for _, s := range r.Steps {
		if s.ID == id {
			return s.Status
		}
	}
	return ""
}

func TestCheckCodexSteps(t *testing.T) {
	dir := t.TempDir()
	codex := fakeCLI(t, dir, "codex", `case "$1" in
--version) echo "codex-cli 9.9.9";;
login) echo "Logged in using ChatGPT";;
esac`)
	r := Check(context.Background(), Target{Kind: KindCodex, ExecutablePath: codex})
	if stepStatus(r, "executable") != OK || stepStatus(r, "version") != OK || stepStatus(r, "auth") != OK || r.Version != "codex-cli 9.9.9" {
		t.Fatalf("report = %+v", r)
	}
	if r.Ready {
		t.Fatal("free checks alone must not mark the connection ready")
	}

	loggedOut := fakeCLI(t, dir, "codex2", `case "$1" in
--version) echo "codex-cli 9.9.9";;
login) echo "Not logged in"; exit 1;;
esac`)
	if r := Check(context.Background(), Target{Kind: KindCodex, ExecutablePath: loggedOut}); stepStatus(r, "auth") != Failed || r.Passed() {
		t.Fatalf("logged-out report = %+v", r)
	}
	if r := Check(context.Background(), Target{Kind: KindCodex, ExecutablePath: filepath.Join(dir, "missing")}); stepStatus(r, "executable") != Failed || len(r.Steps) != 1 {
		t.Fatalf("missing exe report = %+v", r)
	}
}

func TestCheckClaudeBridgeSteps(t *testing.T) {
	dir := t.TempDir()
	node := fakeCLI(t, dir, "node", `echo v20.1.0`)
	oldNode := fakeCLI(t, dir, "node16", `echo v16.0.0`)
	script := filepath.Join(dir, "runners", "claude", "dist", "main.js")
	os.MkdirAll(filepath.Dir(script), 0o700)
	os.WriteFile(script, []byte("//"), 0o600)
	sdk := filepath.Join(dir, "runners", "claude", "node_modules", "@anthropic-ai", "claude-agent-sdk")
	os.MkdirAll(sdk, 0o700)
	os.WriteFile(filepath.Join(sdk, "package.json"), []byte("{}"), 0o600)

	r := Check(context.Background(), Target{Kind: KindClaude, Config: Config{Node: node, Script: script, AuthMode: AuthAPIKey}, HasAPIKey: true})
	if !r.Passed() || stepStatus(r, "bridge") != OK || stepStatus(r, "auth") != OK {
		t.Fatalf("report = %+v", r)
	}
	if r := Check(context.Background(), Target{Kind: KindClaude, Config: Config{Node: node, Script: script, AuthMode: AuthAPIKey}}); stepStatus(r, "auth") != Failed {
		t.Fatalf("missing key accepted: %+v", r)
	}
	if r := Check(context.Background(), Target{Kind: KindClaude, Config: Config{Node: oldNode, Script: script}}); stepStatus(r, "version") != Failed {
		t.Fatalf("old node accepted: %+v", r)
	}
	os.RemoveAll(filepath.Join(dir, "runners", "claude", "node_modules"))
	if r := Check(context.Background(), Target{Kind: KindClaude, Config: Config{Node: node, Script: script}}); stepStatus(r, "bridge") != Failed {
		t.Fatalf("bridge without SDK accepted: %+v", r)
	}
}

func TestCheckAPIKinds(t *testing.T) {
	r := Check(context.Background(), Target{Kind: KindOpenAIAPI})
	if stepStatus(r, "auth") != Failed || !strings.Contains(r.Steps[1].Detail, "없습니다") {
		t.Fatalf("report = %+v", r)
	}
	if r := Check(context.Background(), Target{Kind: KindClaudeAPI, HasAPIKey: true}); !r.Passed() {
		t.Fatalf("report = %+v", r)
	}
}

func TestFindBridgeOverride(t *testing.T) {
	p := filepath.Join(t.TempDir(), "main.js")
	os.WriteFile(p, []byte("//"), 0o600)
	t.Setenv("AGENT_OFFICE_CLAUDE_BRIDGE", p)
	if got, ok := FindBridge(); !ok || got != p {
		t.Fatalf("FindBridge = %q %v", got, ok)
	}
}
