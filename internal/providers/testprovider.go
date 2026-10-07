package providers

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Step is one line of a test provider script (JSONL). Exactly one field
// is set per step.
//
//	{"emit": "message_delta", "payload": {"text": "hi"}}
//	{"request": "approval_request", "payload": {"requestId": "r1", "action": "shell"}}
//	{"sleep": "50ms"}
//	{"write": {"key": "spec", "content": "# 기획서"}}
//	{"complete": {"status": "succeeded", "text": "done"}}
//
// A request step emits the event and blocks until Respond names its
// requestId, then emits request_resolved. A write step writes content to
// the path the engine gave for that output key.
type Step struct {
	Emit     string            `json:"emit,omitempty"`
	Request  string            `json:"request,omitempty"`
	Payload  json.RawMessage   `json:"payload,omitempty"`
	Sleep    string            `json:"sleep,omitempty"`
	Write    *WriteStep        `json:"write,omitempty"`
	Complete *CompletedPayload `json:"complete,omitempty"`
}

type WriteStep struct {
	Key     string `json:"key"`
	Content string `json:"content"`
}

// AutoScenario is built in: it writes a well-formed file for every
// requested output and succeeds. Whole-workflow tests use it.
const AutoScenario = "auto"

// AutoSlowScenario is AutoScenario after a 20 second pause, to try
// pause, cancel and restart behaviour in the app.
const AutoSlowScenario = "auto-slow"

// autoContent returns plausible content for an output type.
func autoContent(req StartRequest, o OutputSpec) string {
	switch o.Type {
	case "json":
		return `{"ok":true}`
	case "report":
		return `{"summary":"자동 생성된 보고","ok":true}`
	case "code_change":
		b, _ := json.Marshal(map[string]any{
			"baseCommit": "", "changes": []map[string]string{{"path": req.StepID + ".txt", "status": "added"}},
			"tests": map[string]any{"command": "", "passed": true},
		})
		return string(b)
	default:
		return "# " + req.StepID + "\n\n자동 생성된 " + o.Key + " 결과\n"
	}
}

// ParseScript reads a JSONL script, skipping blank lines and # comments.
func ParseScript(data []byte) ([]Step, error) {
	var steps []Step
	sc := bufio.NewScanner(bytes.NewReader(data))
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		var s Step
		if err := json.Unmarshal(line, &s); err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if s.Request != "" {
			var p RequestPayload
			if err := json.Unmarshal(s.Payload, &p); err != nil || p.RequestID == "" {
				return nil, fmt.Errorf("line %d: request step needs payload.requestId", n)
			}
		}
		if s.Sleep != "" {
			if _, err := time.ParseDuration(s.Sleep); err != nil {
				return nil, fmt.Errorf("line %d: %w", n, err)
			}
		}
		steps = append(steps, s)
	}
	return steps, sc.Err()
}

// TestProvider replays scripted events. It is a real Provider so the
// engine and UI can be built and tested without any AI connection; its
// results never count as a verified real connection.
type TestProvider struct {
	// Scripts maps a scenario name (StartRequest.Model) to its steps.
	Scripts map[string][]Step
}

// LoadTestProvider loads every *.jsonl file in dir as a scenario named
// after the file (without extension).
func LoadTestProvider(dir string) (*TestProvider, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return nil, err
	}
	p := &TestProvider{Scripts: map[string][]Step{}}
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, err
		}
		steps, err := ParseScript(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
		p.Scripts[filepath.Base(f[:len(f)-len(".jsonl")])] = steps
	}
	return p, nil
}

func (*TestProvider) Name() string { return "test" }

func (*TestProvider) Capabilities() Capabilities {
	return Capabilities{Streaming: true, ToolApproval: true, Question: true, Cancel: true, Usage: true}
}

func (p *TestProvider) Start(ctx context.Context, req StartRequest) (Session, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	steps, ok := p.Scripts[req.Model]
	if req.Model == AutoScenario || req.Model == AutoSlowScenario {
		steps, ok = nil, true
		if req.Model == AutoSlowScenario {
			steps = append(steps, Step{Emit: KindMessage, Payload: json.RawMessage(`{"text":"20초 동안 작업하는 척합니다"}`)}, Step{Sleep: "20s"})
		}
		for _, o := range req.OutputSpec {
			steps = append(steps, Step{Write: &WriteStep{Key: o.Key, Content: autoContent(req, o)}})
		}
		steps = append(steps, Step{Complete: &CompletedPayload{Status: StatusSucceeded, Text: "자동 완료"}})
	}
	if !ok {
		return nil, fmt.Errorf("test provider: unknown scenario %q", req.Model)
	}
	s := &testSession{
		req:       req,
		stream:    newStream(req),
		cancelled: make(chan struct{}),
		pending:   map[string]chan Response{},
	}
	go s.run(steps)
	return s, nil
}

func (*TestProvider) Resume(context.Context, StartRequest, string) (Session, error) {
	return nil, ErrUnsupported
}

type testSession struct {
	*stream
	req        StartRequest
	cancelOnce sync.Once
	cancelled  chan struct{}
	mu         sync.Mutex
	pending    map[string]chan Response
}

func (s *testSession) Events() <-chan Event { return s.ch }

func (s *testSession) run(steps []Step) {
	s.emit(KindStarted, StartedPayload{SessionID: "test-" + newID()})
	for _, st := range steps {
		select {
		case <-s.cancelled:
			s.complete(CompletedPayload{Status: StatusCancelled})
			return
		default:
		}
		switch {
		case st.Complete != nil:
			s.complete(*st.Complete)
			return
		case st.Write != nil:
			if err := s.write(*st.Write); err != nil {
				s.complete(CompletedPayload{Status: StatusFailed, Error: err.Error()})
				return
			}
		case st.Sleep != "":
			d, _ := time.ParseDuration(st.Sleep)
			select {
			case <-time.After(d):
			case <-s.cancelled:
			}
		case st.Request != "":
			var p RequestPayload
			json.Unmarshal(st.Payload, &p)
			ch := make(chan Response, 1)
			s.mu.Lock()
			s.pending[p.RequestID] = ch
			s.mu.Unlock()
			s.emit(st.Request, st.Payload)
			select {
			case r := <-ch:
				decision := r.Decision
				if decision == "" {
					decision = "answered"
				}
				s.emit(KindRequestResolved, ResolvedPayload{RequestID: r.RequestID, Decision: decision})
			case <-s.cancelled:
			}
		case st.Emit != "":
			s.emit(st.Emit, st.Payload)
		}
	}
	select {
	case <-s.cancelled:
		s.complete(CompletedPayload{Status: StatusCancelled})
	default:
		s.complete(CompletedPayload{Status: StatusFailed, Error: "script ended without a complete step"})
	}
}

func (s *testSession) write(w WriteStep) error {
	for _, o := range s.req.OutputSpec {
		if o.Key == w.Key {
			if o.Path == "" {
				return fmt.Errorf("no path for output %q", w.Key)
			}
			if err := os.MkdirAll(filepath.Dir(o.Path), 0o700); err != nil {
				return err
			}
			return os.WriteFile(o.Path, []byte(w.Content), 0o600)
		}
	}
	return fmt.Errorf("output %q was not requested", w.Key)
}

func (s *testSession) Respond(_ context.Context, r Response) error {
	if s.closed() {
		return ErrSessionClosed
	}
	s.mu.Lock()
	ch, ok := s.pending[r.RequestID]
	delete(s.pending, r.RequestID)
	s.mu.Unlock()
	if !ok {
		return fmt.Errorf("%w: %q", ErrUnknownRequest, r.RequestID)
	}
	ch <- r
	return nil
}

func (s *testSession) Cancel(context.Context) error {
	s.cancelOnce.Do(func() { close(s.cancelled) })
	return nil
}
