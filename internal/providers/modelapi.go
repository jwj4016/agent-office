package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/anthropics/anthropic-sdk-go"
	aoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/openai/openai-go/v3"
	ooption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

// Model API kinds.
const (
	KindClaudeAPI = "claude_api"
	KindOpenAIAPI = "openai_api"
)

// DefaultClaudeModel is used when a Claude API assignment names no model.
const DefaultClaudeModel = "claude-opus-5-5"

// Prices are USD per million tokens. Nil means unknown: cost is then
// reported as unknown, never as 0 (spec §10.3).
type Prices struct {
	InputPerMTok  *float64 `json:"inputPerMTok,omitempty"`
	OutputPerMTok *float64 `json:"outputPerMTok,omitempty"`
}

func (p Prices) cost(in, out int64) *float64 {
	if p.InputPerMTok == nil || p.OutputPerMTok == nil {
		return nil
	}
	c := float64(in)/1e6**p.InputPerMTok + float64(out)/1e6**p.OutputPerMTok
	return &c
}

// ModelAPI calls a hosted model directly over HTTPS (OpenAI Responses or
// Claude Messages). It has no tools: it cannot edit code or run tests,
// and it never claims to have searched the web. Each output is requested
// as one field of a structured JSON answer and written to its path.
type ModelAPI struct {
	Kind    string
	APIKey  string
	BaseURL string // tests and proxies; empty for the official endpoint
	Prices  Prices
}

func (m *ModelAPI) Name() string { return m.Kind }

func (*ModelAPI) Capabilities() Capabilities {
	return Capabilities{Cancel: true, Usage: true}
}

func (*ModelAPI) Resume(context.Context, StartRequest, string) (Session, error) {
	return nil, ErrUnsupported
}

// outputSchema asks for one string field per output plus a short
// message for the person. Every field is required (strict mode); an
// optional output may be returned as "".
func outputSchema(specs []OutputSpec) map[string]any {
	props := map[string]any{"message": map[string]any{"type": "string", "description": "작업 요약 (사람에게 보여 줌)"}}
	required := []string{"message"}
	for _, o := range specs {
		desc := fmt.Sprintf("결과 %q의 전체 내용 (%s)", o.Key, o.Type)
		if o.Type == "json" || o.Type == "report" || o.Type == "code_change" {
			desc += ". 유효한 JSON 문서를 문자열로 넣는다"
		}
		if !o.Required {
			desc += ". 만들지 않으면 빈 문자열"
		}
		props[o.Key] = map[string]any{"type": "string", "description": desc}
		required = append(required, o.Key)
	}
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

func (m *ModelAPI) Start(ctx context.Context, req StartRequest) (Session, error) {
	if err := req.validate(); err != nil {
		return nil, err
	}
	if m.APIKey == "" {
		return nil, errors.New("API 키가 설정되지 않았습니다")
	}
	for _, o := range req.OutputSpec {
		if o.Path == "" {
			return nil, fmt.Errorf("output %q has no path", o.Key)
		}
		if o.Type == "file" {
			return nil, fmt.Errorf("모델 API는 파일 결과(%q)를 만들 수 없습니다", o.Key)
		}
	}
	model := req.Model
	switch m.Kind {
	case KindClaudeAPI:
		if model == "" {
			model = DefaultClaudeModel
		}
	case KindOpenAIAPI:
		if model == "" {
			return nil, errors.New("OpenAI 모델 이름을 담당자 설정에서 지정하세요")
		}
	default:
		return nil, fmt.Errorf("unknown model API kind %q", m.Kind)
	}
	cctx, cancel := context.WithCancel(context.Background())
	s := &apiSession{stream: newStream(req), cancel: cancel}
	go s.run(cctx, m, req, model)
	return s, nil
}

type apiSession struct {
	*stream
	cancel     context.CancelFunc
	cancelOnce sync.Once
	cancelled  bool
	mu         sync.Mutex
}

func (s *apiSession) Events() <-chan Event { return s.ch }

func (s *apiSession) Respond(context.Context, Response) error {
	return fmt.Errorf("%w: model APIs do not ask questions", ErrUnknownRequest)
}

func (s *apiSession) Cancel(context.Context) error {
	s.cancelOnce.Do(func() {
		s.mu.Lock()
		s.cancelled = true
		s.mu.Unlock()
		s.cancel()
	})
	return nil
}

func (s *apiSession) wasCancelled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelled
}

type apiResult struct {
	text      string
	in, out   int64
	model     string
	stopError string // non-empty when the model stopped without a usable answer
}

func (s *apiSession) run(ctx context.Context, m *ModelAPI, req StartRequest, model string) {
	defer s.cancel()
	s.emit(KindStarted, StartedPayload{SessionID: "api-" + newID(), Model: model})
	schema := outputSchema(req.OutputSpec)
	var res apiResult
	var err error
	if m.Kind == KindClaudeAPI {
		res, err = callClaude(ctx, m, req, model, schema)
	} else {
		res, err = callOpenAI(ctx, m, req, model, schema)
	}
	if s.wasCancelled() {
		s.complete(CompletedPayload{Status: StatusCancelled})
		return
	}
	if err != nil {
		s.emit(KindError, ErrorPayload{Message: err.Error()})
		s.complete(CompletedPayload{Status: StatusFailed, Error: err.Error()})
		return
	}
	s.emit(KindUsage, UsagePayload{InputTokens: res.in, OutputTokens: res.out, CostUSD: m.Prices.cost(res.in, res.out)})
	if res.stopError != "" {
		s.complete(CompletedPayload{Status: StatusFailed, Error: res.stopError})
		return
	}
	var fields map[string]string
	if err := json.Unmarshal([]byte(res.text), &fields); err != nil {
		s.complete(CompletedPayload{Status: StatusFailed, Error: "모델 응답이 요청한 JSON 형식이 아닙니다"})
		return
	}
	for _, o := range req.OutputSpec {
		content := fields[o.Key]
		if strings.TrimSpace(content) == "" {
			continue // missing required outputs are caught by verification
		}
		if err := os.MkdirAll(filepath.Dir(o.Path), 0o700); err != nil {
			s.complete(CompletedPayload{Status: StatusFailed, Error: err.Error()})
			return
		}
		if err := os.WriteFile(o.Path, []byte(content), 0o600); err != nil {
			s.complete(CompletedPayload{Status: StatusFailed, Error: err.Error()})
			return
		}
	}
	if msg := fields["message"]; msg != "" {
		s.emit(KindMessage, TextPayload{Text: msg})
	}
	s.complete(CompletedPayload{Status: StatusSucceeded, Text: fields["message"]})
}

func callClaude(ctx context.Context, m *ModelAPI, req StartRequest, model string, schema map[string]any) (apiResult, error) {
	opts := []aoption.RequestOption{aoption.WithAPIKey(m.APIKey)}
	if m.BaseURL != "" {
		opts = append(opts, aoption.WithBaseURL(m.BaseURL))
	}
	client := anthropic.NewClient(opts...)
	params := anthropic.BetaMessageNewParams{
		Model:     anthropic.Model(model),
		MaxTokens: 16000,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock(req.Prompt))},
		OutputConfig: anthropic.BetaOutputConfigParam{
			Format: anthropic.BetaJSONOutputFormatParam{Schema: schema},
		},
		// A safety refusal is re-served by a fallback model in the same call.
		Fallbacks: anthropic.BetaFallbacksParamOfDefault(),
		Betas:     []anthropic.AnthropicBeta{"server-side-fallback-2026-07-01"},
	}
	if req.Instructions != "" {
		params.System = []anthropic.BetaTextBlockParam{{Text: req.Instructions}}
	}
	msg, err := client.Beta.Messages.New(ctx, params)
	if err != nil {
		return apiResult{}, fmt.Errorf("Claude API 호출 실패: %w", err)
	}
	res := apiResult{in: msg.Usage.InputTokens, out: msg.Usage.OutputTokens, model: string(msg.Model)}
	switch string(msg.StopReason) {
	case "refusal":
		res.stopError = "모델이 요청을 거절했습니다"
		if c := string(msg.StopDetails.Category); c != "" {
			res.stopError += " (" + c + ")"
		}
		return res, nil
	case "max_tokens":
		res.stopError = "응답이 최대 길이에서 잘렸습니다"
		return res, nil
	}
	for _, b := range msg.Content {
		if t, ok := b.AsAny().(anthropic.BetaTextBlock); ok {
			res.text += t.Text
		}
	}
	return res, nil
}

func callOpenAI(ctx context.Context, m *ModelAPI, req StartRequest, model string, schema map[string]any) (apiResult, error) {
	opts := []ooption.RequestOption{ooption.WithAPIKey(m.APIKey)}
	if m.BaseURL != "" {
		opts = append(opts, ooption.WithBaseURL(m.BaseURL))
	}
	client := openai.NewClient(opts...)
	format := responses.ResponseFormatTextConfigParamOfJSONSchema("outputs", schema)
	format.OfJSONSchema.Strict = openai.Bool(true)
	params := responses.ResponseNewParams{
		Model: model,
		Input: responses.ResponseNewParamsInputUnion{OfString: openai.String(req.Prompt)},
		Text:  responses.ResponseTextConfigParam{Format: format},
		Store: openai.Bool(false),
	}
	if req.Instructions != "" {
		params.Instructions = openai.String(req.Instructions)
	}
	resp, err := client.Responses.New(ctx, params)
	if err != nil {
		return apiResult{}, fmt.Errorf("OpenAI API 호출 실패: %w", err)
	}
	res := apiResult{in: resp.Usage.InputTokens, out: resp.Usage.OutputTokens, model: string(resp.Model)}
	if resp.Status != "completed" {
		res.stopError = "응답이 완료되지 않았습니다: " + string(resp.Status)
		if r := string(resp.IncompleteDetails.Reason); r != "" {
			res.stopError += " (" + r + ")"
		}
		return res, nil
	}
	res.text = resp.OutputText()
	return res, nil
}
