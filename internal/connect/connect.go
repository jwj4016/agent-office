// Package connect finds provider executables and checks connections step
// by step: executable found, version, authentication, real call. Each
// step is reported separately so the UI never claims more than was
// actually verified (spec §9.5).
package connect

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Connection kinds.
const (
	KindTest      = "test"
	KindCodex     = "codex"
	KindClaude    = "claude" // Claude Agent SDK bridge
	KindClaudeAPI = "claude_api"
	KindOpenAIAPI = "openai_api"
)

// Auth modes for the Claude bridge.
const (
	AuthAPIKey     = "api_key"
	AuthLocalLogin = "local_login" // personal use only
)

// Config is the non-secret per-connection configuration (stored as JSON).
type Config struct {
	AuthMode string `json:"authMode,omitempty"`
	Node     string `json:"node,omitempty"`
	Script   string `json:"script,omitempty"`
	// BaseURL overrides the model API endpoint (proxies, tests).
	BaseURL string `json:"baseUrl,omitempty"`
	// Prices in USD per million tokens; nil means unknown.
	InputPerMTok  *float64 `json:"inputPerMTok,omitempty"`
	OutputPerMTok *float64 `json:"outputPerMTok,omitempty"`
}

func ParseConfig(raw json.RawMessage) Config {
	var c Config
	json.Unmarshal(raw, &c)
	return c
}

// Step results.
const (
	OK      = "ok"
	Failed  = "failed"
	Skipped = "skipped"
)

type Step struct {
	ID     string `json:"id"` // executable | version | auth | call
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type Report struct {
	Steps []Step `json:"steps"`
	// Ready is true only when every step, including a real call, passed.
	Ready   bool   `json:"ready"`
	Version string `json:"version,omitempty"`
}

func (r *Report) add(id, label, status, detail string) bool {
	r.Steps = append(r.Steps, Step{ID: id, Label: label, Status: status, Detail: detail})
	return status == OK
}

// searchDirs are checked in addition to PATH: apps started from Finder
// get a minimal PATH that misses Homebrew and npm installs (spec §10.2).
func searchDirs() []string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "windows":
		appdata := os.Getenv("APPDATA")
		return []string{filepath.Join(appdata, "npm"), filepath.Join(os.Getenv("ProgramFiles"), "nodejs")}
	default:
		return []string{"/opt/homebrew/bin", "/usr/local/bin", "/usr/bin",
			filepath.Join(home, ".local", "bin"), filepath.Join(home, ".npm-global", "bin"), filepath.Join(home, ".volta", "bin")}
	}
}

func exeNames(name string) []string {
	if runtime.GOOS == "windows" {
		return []string{name + ".exe", name + ".cmd", name + ".bat"}
	}
	return []string{name}
}

// FindExecutable looks a command up on PATH and in common install dirs.
func FindExecutable(name string) (string, bool) {
	if p, err := exec.LookPath(name); err == nil {
		abs, _ := filepath.Abs(p)
		return abs, true
	}
	for _, d := range searchDirs() {
		for _, n := range exeNames(name) {
			p := filepath.Join(d, n)
			if st, err := os.Stat(p); err == nil && !st.IsDir() {
				return p, true
			}
		}
	}
	return "", false
}

// FindBridge locates runners/claude/dist/main.js: an override, next to
// the app bundle, or (development) walking up from the executable/cwd.
func FindBridge() (string, bool) {
	if p := os.Getenv("AGENT_OFFICE_CLAUDE_BRIDGE"); p != "" {
		return p, fileExists(p)
	}
	var starts []string
	if exe, err := os.Executable(); err == nil {
		starts = append(starts, filepath.Dir(exe), filepath.Join(filepath.Dir(exe), "..", "Resources"))
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	for _, s := range starts {
		for dir, i := s, 0; i < 8; i, dir = i+1, filepath.Dir(dir) {
			for _, rel := range []string{filepath.Join("runners", "claude", "dist", "main.js"), filepath.Join("claude-runner", "dist", "main.js")} {
				p := filepath.Join(dir, rel)
				if fileExists(p) {
					abs, _ := filepath.Abs(p)
					return abs, true
				}
			}
		}
	}
	return "", false
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// runQuiet runs an executable with arguments (never through a shell) and
// returns trimmed combined output.
func runQuiet(ctx context.Context, exe string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = withPath(os.Environ(), filepath.Dir(exe))
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// withPath makes sure the executable's own dir (e.g. Homebrew's node for
// an npm-installed CLI) is on PATH for the child.
func withPath(env []string, dir string) []string {
	for i, kv := range env {
		if strings.HasPrefix(kv, "PATH=") {
			env[i] = kv + string(os.PathListSeparator) + dir
			return env
		}
	}
	return append(env, "PATH="+dir)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// Inputs for a check; secrets are passed in, never read here.
type Target struct {
	Kind           string
	ExecutablePath string
	Config         Config
	HasAPIKey      bool
}

// Check runs the free steps (executable, version, auth). The real call
// step is added by the caller only when the person asks for it.
func Check(ctx context.Context, t Target) Report {
	var r Report
	switch t.Kind {
	case KindTest:
		r.add("executable", "실행 파일", Skipped, "내장 테스트 연결")
		r.add("auth", "인증", Skipped, "필요 없음")
	case KindCodex:
		exe := t.ExecutablePath
		if exe == "" {
			exe, _ = FindExecutable("codex")
		}
		if !r.add("executable", "실행 파일", status(fileExists(exe)), orMissing(exe, "codex를 찾지 못했습니다")) {
			return r
		}
		out, err := runQuiet(ctx, exe, "--version")
		if !r.add("version", "버전", status(err == nil), firstLine(out)) {
			return r
		}
		r.Version = firstLine(out)
		out, err = runQuiet(ctx, exe, "login", "status")
		r.add("auth", "인증", status(err == nil && strings.Contains(out, "Logged in")), firstLine(out))
	case KindClaude:
		node := t.Config.Node
		if node == "" {
			node, _ = FindExecutable("node")
		}
		script := t.Config.Script
		if script == "" {
			script, _ = FindBridge()
		}
		if !r.add("executable", "Node.js", status(fileExists(node)), orMissing(node, "node를 찾지 못했습니다")) {
			return r
		}
		out, err := runQuiet(ctx, node, "--version")
		if !r.add("version", "Node 버전", status(err == nil && nodeMajor(out) >= 18), out+" (18 이상 필요)") {
			return r
		}
		if !r.add("bridge", "Claude 실행 프로그램", status(fileExists(script) && sdkInstalled(script)), orMissing(script, "runners/claude 빌드가 필요합니다 (npm install && npm run build)")) {
			return r
		}
		r.Version = "node " + out
		switch t.Config.AuthMode {
		case AuthLocalLogin:
			claude, ok := FindExecutable("claude")
			if !ok {
				r.add("auth", "인증 (이 PC의 Claude 로그인)", Failed, "claude CLI를 찾지 못했습니다")
				break
			}
			out, err := runQuiet(ctx, claude, "auth", "status", "--json")
			var st struct {
				LoggedIn   bool   `json:"loggedIn"`
				AuthMethod string `json:"authMethod"`
			}
			json.Unmarshal([]byte(out), &st)
			r.add("auth", "인증 (이 PC의 Claude 로그인)", status(err == nil && st.LoggedIn), "로그인 방식: "+orMissing(st.AuthMethod, "로그인 안 됨"))
		default:
			r.add("auth", "인증 (API 키)", status(t.HasAPIKey), keyDetail(t.HasAPIKey))
		}
	case KindClaudeAPI, KindOpenAIAPI:
		r.add("executable", "실행 파일", Skipped, "HTTPS로 직접 호출")
		r.add("auth", "인증 (API 키)", status(t.HasAPIKey), keyDetail(t.HasAPIKey))
	default:
		r.add("executable", "연결 종류", Failed, fmt.Sprintf("지원하지 않는 종류 %q", t.Kind))
	}
	return r
}

func sdkInstalled(script string) bool {
	root := filepath.Dir(filepath.Dir(script)) // runners/claude
	return fileExists(filepath.Join(root, "node_modules", "@anthropic-ai", "claude-agent-sdk", "package.json"))
}

func nodeMajor(v string) int {
	var major int
	fmt.Sscanf(strings.TrimPrefix(strings.TrimSpace(v), "v"), "%d", &major)
	return major
}

func status(ok bool) string {
	if ok {
		return OK
	}
	return Failed
}

func orMissing(v, missing string) string {
	if v == "" {
		return missing
	}
	return v
}

func keyDetail(has bool) string {
	if has {
		return "키 저장됨 (OS 비밀 저장소 또는 이번 세션 메모리)"
	}
	return "API 키가 없습니다"
}

// ErrNotChecked means a real call was not attempted because an earlier
// step failed.
var ErrNotChecked = errors.New("earlier checks failed")

// Passed reports whether all non-skipped steps passed.
func (r Report) Passed() bool {
	for _, s := range r.Steps {
		if s.Status == Failed {
			return false
		}
	}
	return true
}
