package providers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// EventSchemaVersion is bumped when Event's shape changes incompatibly.
const EventSchemaVersion = 1

var (
	// ErrUnsupported is returned for operations a provider cannot do.
	// Callers must disable the matching UI instead of faking it.
	ErrUnsupported = errors.New("not supported by this provider")
	// ErrUnknownRequest means Respond named a request that is not pending.
	ErrUnknownRequest = errors.New("no pending request with this id")
	// ErrSessionClosed is returned when talking to a finished session.
	ErrSessionClosed = errors.New("session closed")
)

// Capabilities tells the engine and UI which controls are real.
type Capabilities struct {
	Streaming    bool `json:"streaming"`
	ToolApproval bool `json:"toolApproval"`
	Question     bool `json:"question"`
	Cancel       bool `json:"cancel"`
	Resume       bool `json:"resume"`
	Usage        bool `json:"usage"`
	Coding       bool `json:"coding"`
}

// InputRef pins one input artifact version for a step attempt.
type InputRef struct {
	Name       string `json:"name"`
	ArtifactID string `json:"artifactId"`
	Path       string `json:"path"`
	Hash       string `json:"hash"`
}

// OutputSpec describes one output the step must produce.
type OutputSpec struct {
	Key      string          `json:"key"`
	Type     string          `json:"type"`
	Required bool            `json:"required"`
	Schema   json.RawMessage `json:"schema,omitempty"`
	// Path is where the provider must write this output.
	Path string `json:"path"`
}

type Limits struct {
	Timeout   time.Duration `json:"timeout"`
	MaxTokens int           `json:"maxTokens"`
}

// Policy is the structured permission set for one attempt. Prompt text is
// never relied on to restrict tools.
type Policy struct {
	Sandbox      string `json:"sandbox"`     // read-only | workspace-write
	AskApproval  bool   `json:"askApproval"` // route tool use through Respond
	AllowNetwork bool   `json:"allowNetwork"`
}

// StartRequest carries everything a provider needs for one step attempt.
type StartRequest struct {
	ProjectID     string       `json:"projectId"`
	RunID         string       `json:"runId"`
	StepAttemptID string       `json:"stepAttemptId"`
	StepID        string       `json:"stepId"`
	Generation    int          `json:"generation"`
	Instructions  string       `json:"instructions"`
	Prompt        string       `json:"prompt"`
	InputManifest []InputRef   `json:"inputManifest"`
	OutputSpec    []OutputSpec `json:"outputSpec"`
	Workspace     string       `json:"workspace"`
	Limits        Limits       `json:"limits"`
	Policy        Policy       `json:"policy"`
	ConnectionRef string       `json:"connectionRef"`
	Model         string       `json:"model"`
}

func (r StartRequest) validate() error {
	if r.ProjectID == "" || r.RunID == "" || r.StepAttemptID == "" {
		return errors.New("projectId, runId and stepAttemptId are required")
	}
	return nil
}

// Event kinds. Payload shapes are documented next to each payload type.
const (
	KindStarted         = "started"          // StartedPayload
	KindMessageDelta    = "message_delta"    // TextPayload
	KindMessage         = "message"          // TextPayload
	KindTool            = "tool"             // ToolPayload
	KindApprovalRequest = "approval_request" // RequestPayload
	KindQuestion        = "question"         // RequestPayload
	KindRequestResolved = "request_resolved" // ResolvedPayload
	KindUsage           = "usage"            // UsagePayload
	KindError           = "error"            // ErrorPayload
	KindCompleted       = "completed"        // CompletedPayload; always the last event
)

// Terminal statuses reported by KindCompleted. "succeeded" here only means
// the provider finished; the engine still verifies outputs.
const (
	StatusSucceeded   = "succeeded"
	StatusFailed      = "failed"
	StatusCancelled   = "cancelled"
	StatusInterrupted = "interrupted"
)

// Event is the normalized, public stream every provider produces.
type Event struct {
	SchemaVersion int             `json:"schemaVersion"`
	EventID       string          `json:"eventId"`
	Sequence      int64           `json:"sequence"`
	ProjectID     string          `json:"projectId"`
	RunID         string          `json:"runId"`
	StepAttemptID string          `json:"stepAttemptId"`
	Generation    int             `json:"generation"`
	Kind          string          `json:"kind"`
	Timestamp     string          `json:"timestamp"`
	Payload       json.RawMessage `json:"payload"`
}

type StartedPayload struct {
	SessionID string `json:"sessionId"`
	Model     string `json:"model,omitempty"`
}

type TextPayload struct {
	Text string `json:"text"`
}

type ToolPayload struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type RequestPayload struct {
	RequestID string   `json:"requestId"`
	Action    string   `json:"action"`
	Detail    string   `json:"detail"`
	Options   []string `json:"options,omitempty"`
}

type ResolvedPayload struct {
	RequestID string `json:"requestId"`
	Decision  string `json:"decision"`
}

type UsagePayload struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	// CostUSD is nil when the provider does not report cost. Never 0 for unknown.
	CostUSD *float64 `json:"costUsd"`
}

type ErrorPayload struct {
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type CompletedPayload struct {
	Status string `json:"status"`
	Text   string `json:"text,omitempty"`
	Error  string `json:"error,omitempty"`
}

// Decisions for approval requests.
const (
	DecisionAccept  = "accept"
	DecisionDecline = "decline"
)

// Response answers one approval or question request.
type Response struct {
	RequestID string `json:"requestId"`
	Decision  string `json:"decision,omitempty"`
	Answer    string `json:"answer,omitempty"`
}

// Session is one running step attempt.
type Session interface {
	// Events is closed after the KindCompleted event.
	Events() <-chan Event
	Respond(ctx context.Context, r Response) error
	Cancel(ctx context.Context) error
}

// Provider starts sessions for one connection type.
type Provider interface {
	Name() string
	Capabilities() Capabilities
	Start(ctx context.Context, req StartRequest) (Session, error)
	// Resume returns ErrUnsupported unless Capabilities().Resume.
	Resume(ctx context.Context, req StartRequest, sessionID string) (Session, error)
}

// stream stamps identity, ordering and time on every event so providers
// only supply kind and payload. It guarantees exactly one KindCompleted.
type stream struct {
	req  StartRequest
	ch   chan Event
	mu   sync.Mutex
	seq  int64
	done bool
}

func newStream(req StartRequest) *stream {
	return &stream{req: req, ch: make(chan Event, 256)}
}

func newID() string {
	b := make([]byte, 12)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// emit sends an event. It returns false once the stream is completed.
func (s *stream) emit(kind string, payload any) bool {
	raw, err := json.Marshal(payload)
	if err != nil {
		raw, _ = json.Marshal(ErrorPayload{Message: fmt.Sprintf("marshal %s payload: %v", kind, err)})
		kind = KindError
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return false
	}
	s.seq++
	s.ch <- Event{
		SchemaVersion: EventSchemaVersion,
		EventID:       newID(),
		Sequence:      s.seq,
		ProjectID:     s.req.ProjectID,
		RunID:         s.req.RunID,
		StepAttemptID: s.req.StepAttemptID,
		Generation:    s.req.Generation,
		Kind:          kind,
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		Payload:       raw,
	}
	if kind == KindCompleted {
		s.done = true
		close(s.ch)
	}
	return true
}

func (s *stream) complete(p CompletedPayload) { s.emit(KindCompleted, p) }

func (s *stream) closed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done
}
