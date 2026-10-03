package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Codex drives `codex app-server` over local stdio JSON-RPC.
// Protocol checked against codex-cli 0.159.2 (see docs/decisions.md).
type Codex struct {
	// Executable is the user-configured codex binary path.
	Executable string
	// Env for the child; nil inherits the app environment so the CLI can
	// find its own login.
	Env []string
	// Version is reported to the server as clientInfo.version.
	Version string
	// InterruptGrace bounds how long Cancel waits for turn/completed.
	InterruptGrace time.Duration
}

func (*Codex) Name() string { return "codex" }

func (*Codex) Capabilities() Capabilities {
	return Capabilities{Streaming: true, ToolApproval: true, Question: true, Cancel: true, Usage: true, Coding: true}
}

func (*Codex) Resume(context.Context, StartRequest, string) (Session, error) {
	return nil, ErrUnsupported
}

func (c *Codex) Start(ctx context.Context, req StartRequest) (Session, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	if c.Executable == "" {
		return nil, errors.New("codex: executable path not configured")
	}
	env := c.Env
	if env == nil {
		env = os.Environ()
	}
	// The process outlives the Start call; Cancel ends it, not ctx.
	p, err := startProc(context.Background(), c.Executable, []string{"app-server"}, req.Workspace, env)
	if err != nil {
		return nil, err
	}
	grace := c.InterruptGrace
	if grace == 0 {
		grace = 10 * time.Second
	}
	s := &codexSession{
		stream:   newStream(req),
		proc:     p,
		grace:    grace,
		calls:    map[int64]pendingCall{},
		requests: map[string]pendingRPC{},
		turnDone: make(chan turnResult, 1),
		cancelCh: make(chan struct{}),
	}
	go s.readLoop()
	go s.run(ctx, req, c.Version)
	return s, nil
}

type rpcMessage struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("rpc error %d: %s", e.Code, e.Message) }

type pendingCall struct {
	method string
	ch     chan rpcReply
}

type rpcReply struct {
	result json.RawMessage
	err    error
}

// pendingRPC is a server request waiting for the user's decision.
type pendingRPC struct {
	id          json.RawMessage
	kind        string // approval | question
	questionIDs []string
}

type turnResult struct {
	status string
	errMsg string
}

type codexSession struct {
	*stream
	proc  *proc
	grace time.Duration

	nextID atomic.Int64
	mu     sync.Mutex
	calls  map[int64]pendingCall
	// requests is keyed by the requestId exposed in events.
	requests map[string]pendingRPC

	threadID, turnID string
	lastMessage      string

	turnDone   chan turnResult
	cancelOnce sync.Once
	cancelCh   chan struct{}
}

func (s *codexSession) Events() <-chan Event { return s.ch }

func (s *codexSession) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := s.nextID.Add(1)
	ch := make(chan rpcReply, 1)
	s.mu.Lock()
	s.calls[id] = pendingCall{method: method, ch: ch}
	s.mu.Unlock()
	raw, _ := json.Marshal(params)
	if err := s.proc.send(rpcMessage{ID: json.RawMessage(fmt.Sprint(id)), Method: method, Params: raw}); err != nil {
		return nil, err
	}
	select {
	case r := <-ch:
		return r.result, r.err
	case <-s.proc.exited:
		return nil, errProcExited
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.cancelCh:
		return nil, context.Canceled
	}
}

func (s *codexSession) readLoop() {
	for line := range s.proc.lines {
		var m rpcMessage
		if err := json.Unmarshal(line, &m); err != nil {
			s.emit(KindError, ErrorPayload{Message: "codex: unreadable message: " + truncate(string(line), 200)})
			continue
		}
		switch {
		case m.Method == "" && len(m.ID) > 0:
			var id int64
			json.Unmarshal(m.ID, &id)
			s.mu.Lock()
			pc, ok := s.calls[id]
			delete(s.calls, id)
			if ok && pc.method == "turn/start" && m.Error == nil {
				// Record the turn before reading further so its
				// notifications are never mistaken for another turn's.
				var tr struct {
					Turn struct {
						ID string `json:"id"`
					} `json:"turn"`
				}
				json.Unmarshal(m.Result, &tr)
				s.turnID = tr.Turn.ID
			}
			s.mu.Unlock()
			if ok {
				if m.Error != nil {
					pc.ch <- rpcReply{err: m.Error}
				} else {
					pc.ch <- rpcReply{result: m.Result}
				}
			}
		case m.Method != "" && len(m.ID) > 0:
			s.onServerRequest(m)
		case m.Method != "":
			s.onNotification(m)
		}
	}
}

func (s *codexSession) run(ctx context.Context, req StartRequest, version string) {
	fail := func(err error) {
		select {
		case <-s.cancelCh:
			s.complete(CompletedPayload{Status: StatusCancelled})
		default:
			msg := err.Error()
			if tail := strings.TrimSpace(s.proc.stderrTail()); tail != "" {
				msg += ": " + truncate(tail, 500)
			}
			s.complete(CompletedPayload{Status: StatusFailed, Error: msg})
		}
		s.proc.stop(2 * time.Second)
	}

	if version == "" {
		version = "dev"
	}
	if _, err := s.call(ctx, "initialize", map[string]any{
		"clientInfo":   map[string]any{"name": "agent-office", "title": "Agent Office", "version": version},
		"capabilities": map[string]any{"experimentalApi": false, "requestAttestation": false},
	}); err != nil {
		fail(fmt.Errorf("initialize: %w", err))
		return
	}
	if err := s.proc.send(rpcMessage{Method: "initialized"}); err != nil {
		fail(err)
		return
	}

	approval := "never"
	if req.Policy.AskApproval {
		approval = "on-request"
	}
	sandbox := req.Policy.Sandbox
	if sandbox == "" {
		sandbox = "read-only"
	}
	threadParams := map[string]any{
		"approvalPolicy": approval,
		"sandbox":        sandbox,
		"ephemeral":      true,
	}
	if req.Workspace != "" {
		threadParams["cwd"] = req.Workspace
	}
	if req.Instructions != "" {
		threadParams["developerInstructions"] = req.Instructions
	}
	if req.Model != "" {
		threadParams["model"] = req.Model
	}
	res, err := s.call(ctx, "thread/start", threadParams)
	if err != nil {
		fail(fmt.Errorf("thread/start: %w", err))
		return
	}
	var ts struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
		Model string `json:"model"`
	}
	json.Unmarshal(res, &ts)
	s.mu.Lock()
	s.threadID = ts.Thread.ID
	s.mu.Unlock()
	s.emit(KindStarted, StartedPayload{SessionID: ts.Thread.ID, Model: ts.Model})

	// readLoop records the turn id from this reply.
	if _, err := s.call(ctx, "turn/start", map[string]any{
		"threadId": ts.Thread.ID,
		"input":    []any{map[string]any{"type": "text", "text": req.Prompt, "text_elements": []any{}}},
	}); err != nil {
		fail(fmt.Errorf("turn/start: %w", err))
		return
	}

	var timeout <-chan time.Time
	if req.Limits.Timeout > 0 {
		timeout = time.After(req.Limits.Timeout)
	}
	select {
	case r := <-s.turnDone:
		s.mu.Lock()
		text := s.lastMessage
		s.mu.Unlock()
		s.complete(CompletedPayload{Status: r.status, Text: text, Error: r.errMsg})
		s.proc.stop(2 * time.Second)
	case <-s.proc.exited:
		fail(errProcExited)
	case <-timeout:
		s.interrupt()
		fail(errors.New("time limit reached"))
	case <-s.cancelCh:
		s.interrupt()
		select {
		case <-s.turnDone:
		case <-time.After(s.grace):
		case <-s.proc.exited:
		}
		s.complete(CompletedPayload{Status: StatusCancelled})
		s.proc.stop(2 * time.Second)
	}
}

// interrupt asks the server to stop the current turn. Best effort.
func (s *codexSession) interrupt() {
	s.mu.Lock()
	threadID, turnID := s.threadID, s.turnID
	s.mu.Unlock()
	if turnID == "" {
		return
	}
	id := s.nextID.Add(1)
	params, _ := json.Marshal(map[string]string{"threadId": threadID, "turnId": turnID})
	s.proc.send(rpcMessage{ID: json.RawMessage(fmt.Sprint(id)), Method: "turn/interrupt", Params: params})
}

func (s *codexSession) currentTurn(turnID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.turnID != "" && turnID == s.turnID
}

func (s *codexSession) onNotification(m rpcMessage) {
	switch m.Method {
	case "item/agentMessage/delta":
		var p struct {
			TurnID string `json:"turnId"`
			Delta  string `json:"delta"`
		}
		json.Unmarshal(m.Params, &p)
		if s.currentTurn(p.TurnID) {
			s.emit(KindMessageDelta, TextPayload{Text: p.Delta})
		}
	case "item/completed":
		var p struct {
			TurnID string `json:"turnId"`
			Item   struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				Command  string `json:"command"`
				Status   string `json:"status"`
				ExitCode *int   `json:"exitCode"`
			} `json:"item"`
		}
		json.Unmarshal(m.Params, &p)
		if !s.currentTurn(p.TurnID) {
			return
		}
		switch p.Item.Type {
		case "agentMessage":
			s.mu.Lock()
			s.lastMessage = p.Item.Text
			s.mu.Unlock()
			s.emit(KindMessage, TextPayload{Text: p.Item.Text})
		case "commandExecution":
			s.emit(KindTool, ToolPayload{Name: "command", Status: p.Item.Status, Detail: p.Item.Command})
		case "fileChange":
			s.emit(KindTool, ToolPayload{Name: "file_change", Status: p.Item.Status})
		}
	case "thread/tokenUsage/updated":
		var p struct {
			TokenUsage struct {
				Total struct {
					InputTokens  int64 `json:"inputTokens"`
					OutputTokens int64 `json:"outputTokens"`
				} `json:"total"`
			} `json:"tokenUsage"`
		}
		json.Unmarshal(m.Params, &p)
		s.emit(KindUsage, UsagePayload{InputTokens: p.TokenUsage.Total.InputTokens, OutputTokens: p.TokenUsage.Total.OutputTokens})
	case "error":
		var p struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
			WillRetry bool `json:"willRetry"`
		}
		json.Unmarshal(m.Params, &p)
		s.emit(KindError, ErrorPayload{Message: p.Error.Message, Retryable: p.WillRetry})
	case "turn/completed":
		var p struct {
			Turn struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"turn"`
		}
		json.Unmarshal(m.Params, &p)
		if !s.currentTurn(p.Turn.ID) {
			return
		}
		r := turnResult{status: StatusFailed}
		switch p.Turn.Status {
		case "completed":
			r.status = StatusSucceeded
		case "interrupted":
			r.status = StatusInterrupted
		}
		if p.Turn.Error != nil {
			r.errMsg = p.Turn.Error.Message
		}
		select {
		case s.turnDone <- r:
		default:
		}
	}
}

func (s *codexSession) onServerRequest(m rpcMessage) {
	reqID := "codex-" + strings.Trim(string(m.ID), `"`)
	switch m.Method {
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval":
		var p struct {
			Command string `json:"command"`
			Reason  string `json:"reason"`
		}
		json.Unmarshal(m.Params, &p)
		action := "command"
		if m.Method == "item/fileChange/requestApproval" {
			action = "file_change"
		}
		detail := p.Command
		if detail == "" {
			detail = p.Reason
		}
		s.mu.Lock()
		s.requests[reqID] = pendingRPC{id: m.ID, kind: "approval"}
		s.mu.Unlock()
		s.emit(KindApprovalRequest, RequestPayload{RequestID: reqID, Action: action, Detail: detail,
			Options: []string{DecisionAccept, DecisionDecline}})
	case "item/tool/requestUserInput":
		var p struct {
			Questions []struct {
				ID       string `json:"id"`
				Question string `json:"question"`
			} `json:"questions"`
		}
		json.Unmarshal(m.Params, &p)
		var ids, texts []string
		for _, q := range p.Questions {
			ids = append(ids, q.ID)
			texts = append(texts, q.Question)
		}
		s.mu.Lock()
		s.requests[reqID] = pendingRPC{id: m.ID, kind: "question", questionIDs: ids}
		s.mu.Unlock()
		s.emit(KindQuestion, RequestPayload{RequestID: reqID, Action: "question", Detail: strings.Join(texts, "\n")})
	default:
		// Anything we do not model is refused rather than silently allowed.
		s.proc.send(rpcMessage{ID: m.ID, Error: &rpcError{Code: -32601, Message: "not supported by Agent Office"}})
		s.emit(KindError, ErrorPayload{Message: "codex: refused unsupported request " + m.Method})
	}
}

func (s *codexSession) Respond(_ context.Context, r Response) error {
	if s.closed() {
		return ErrSessionClosed
	}
	s.mu.Lock()
	pr, ok := s.requests[r.RequestID]
	delete(s.requests, r.RequestID)
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownRequest, r.RequestID)
	}
	var result any
	decision := r.Decision
	switch pr.kind {
	case "approval":
		if decision != DecisionAccept {
			decision = DecisionDecline
		}
		result = map[string]string{"decision": decision}
	case "question":
		answers := map[string]any{}
		for _, id := range pr.questionIDs {
			answers[id] = map[string]any{"answers": []string{r.Answer}}
		}
		result = map[string]any{"answers": answers}
		decision = "answered"
	}
	raw, _ := json.Marshal(result)
	if err := s.proc.send(rpcMessage{ID: pr.id, Result: raw}); err != nil {
		return err
	}
	s.emit(KindRequestResolved, ResolvedPayload{RequestID: r.RequestID, Decision: decision})
	return nil
}

func (s *codexSession) Cancel(context.Context) error {
	s.cancelOnce.Do(func() { close(s.cancelCh) })
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
