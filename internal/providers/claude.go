package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
)

// ClaudeBridge runs runners/claude (TypeScript, Claude Agent SDK) as a
// child process and relays its JSONL events. The bridge already emits
// normalized event kinds; this side stamps identity and enforces
// lifecycle (one completion, cancel, crash handling).
type ClaudeBridge struct {
	// Node is the node executable; Script is runners/claude/dist/main.js.
	Node   string
	Script string
	// Env for the bridge; nil inherits the app environment.
	Env []string
	// AllowedTools run without approval (e.g. Read, Grep).
	AllowedTools []string
	// APIKey, when set, is given to the bridge process only (as
	// ANTHROPIC_API_KEY). When empty the local Claude login is used and
	// any inherited ANTHROPIC_API_KEY is removed so the choice is explicit.
	APIKey string
	// CancelGrace bounds how long Cancel waits for the bridge to stop.
	CancelGrace time.Duration
}

func (*ClaudeBridge) Name() string { return "claude" }

func (*ClaudeBridge) Capabilities() Capabilities {
	return Capabilities{Streaming: true, ToolApproval: true, Question: true, Cancel: true, Usage: true, Coding: true}
}

// withoutEnv drops every entry for key from env.
func withoutEnv(env []string, key string) []string {
	out := make([]string, 0, len(env))
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return out
}

func (*ClaudeBridge) Resume(context.Context, StartRequest, string) (Session, error) {
	return nil, ErrUnsupported
}

type bridgeStart struct {
	Type         string   `json:"type"`
	Prompt       string   `json:"prompt"`
	Instructions string   `json:"instructions,omitempty"`
	Cwd          string   `json:"cwd,omitempty"`
	Model        string   `json:"model,omitempty"`
	AllowedTools []string `json:"allowedTools,omitempty"`
	AskApproval  bool     `json:"askApproval"`
	WritableDirs []string `json:"writableDirs,omitempty"`
	// ReadOnlyCwd keeps file-writing tools out of the working folder;
	// only WritableDirs stay writable (Policy.Sandbox "read-only").
	ReadOnlyCwd bool `json:"readOnlyCwd,omitempty"`
}

type bridgeLine struct {
	Type    string          `json:"type"`
	Kind    string          `json:"kind"`
	Payload json.RawMessage `json:"payload"`
}

func (c *ClaudeBridge) Start(ctx context.Context, req StartRequest) (Session, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	if c.Node == "" || c.Script == "" {
		return nil, errors.New("claude: node and bridge script paths are required")
	}
	env := c.Env
	if env == nil {
		env = os.Environ()
	}
	env = withoutEnv(env, "ANTHROPIC_API_KEY")
	if c.APIKey != "" {
		env = append(env, "ANTHROPIC_API_KEY="+c.APIKey)
	}
	p, err := startProc(context.Background(), c.Node, []string{c.Script}, req.Workspace, env)
	if err != nil {
		return nil, err
	}
	grace := c.CancelGrace
	if grace == 0 {
		grace = 10 * time.Second
	}
	s := &claudeSession{stream: newStream(req), proc: p, grace: grace, pending: map[string]bool{}, cancelCh: make(chan struct{})}
	if err := p.send(bridgeStart{
		Type: "start", Prompt: req.Prompt, Instructions: req.Instructions, Cwd: req.Workspace,
		Model: req.Model, AllowedTools: c.AllowedTools, AskApproval: req.Policy.AskApproval, WritableDirs: req.WritableDirs,
		ReadOnlyCwd: req.Policy.Sandbox == "read-only",
	}); err != nil {
		p.stop(time.Second)
		return nil, fmt.Errorf("claude: send start: %w", err)
	}
	go s.readLoop()
	return s, nil
}

type claudeSession struct {
	*stream
	proc  *proc
	grace time.Duration

	mu         sync.Mutex
	pending    map[string]bool
	cancelOnce sync.Once
	cancelCh   chan struct{}
}

func (s *claudeSession) Events() <-chan Event { return s.ch }

func (s *claudeSession) cancelled() bool {
	select {
	case <-s.cancelCh:
		return true
	default:
		return false
	}
}

func (s *claudeSession) readLoop() {
	for line := range s.proc.lines {
		var l bridgeLine
		if err := json.Unmarshal(line, &l); err != nil || l.Type != "event" || l.Kind == "" {
			s.emit(KindError, ErrorPayload{Message: "claude bridge: unreadable line: " + truncate(string(line), 200)})
			continue
		}
		switch l.Kind {
		case KindApprovalRequest, KindQuestion:
			var p RequestPayload
			json.Unmarshal(l.Payload, &p)
			s.mu.Lock()
			s.pending[p.RequestID] = true
			s.mu.Unlock()
		case KindCompleted:
			var p CompletedPayload
			json.Unmarshal(l.Payload, &p)
			if p.Status == "" {
				p.Status = StatusFailed
			}
			s.complete(p)
			go s.proc.stop(2 * time.Second)
			continue
		}
		s.emit(l.Kind, l.Payload)
	}
	// stdout closed: make sure nothing the bridge started is left running.
	go s.proc.stop(2 * time.Second)
	// Without a completion the bridge died.
	if s.cancelled() {
		s.complete(CompletedPayload{Status: StatusCancelled})
		return
	}
	msg := errProcExited.Error()
	if tail := strings.TrimSpace(s.proc.stderrTail()); tail != "" {
		msg += ": " + truncate(tail, 500)
	}
	s.complete(CompletedPayload{Status: StatusFailed, Error: msg})
}

func (s *claudeSession) Respond(_ context.Context, r Response) error {
	if s.closed() {
		return ErrSessionClosed
	}
	s.mu.Lock()
	ok := s.pending[r.RequestID]
	delete(s.pending, r.RequestID)
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownRequest, r.RequestID)
	}
	return s.proc.send(map[string]string{"type": "respond", "requestId": r.RequestID, "decision": r.Decision, "answer": r.Answer})
}

// Cancel asks the bridge to abort; if it does not finish within the grace
// period the process tree is killed and the attempt reported cancelled.
func (s *claudeSession) Cancel(context.Context) error {
	s.cancelOnce.Do(func() {
		close(s.cancelCh)
		s.proc.send(map[string]string{"type": "cancel"})
		go func() {
			select {
			case <-s.proc.exited:
			case <-time.After(s.grace):
				killProc(s.proc.cmd)
			}
		}()
	})
	return nil
}
