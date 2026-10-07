package providers

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type captured struct {
	path, key, beta string
	body            map[string]any
}

// fakeAPI serves one canned response and records the request.
func fakeAPI(t *testing.T, status int, response string) (*httptest.Server, *captured) {
	t.Helper()
	c := &captured{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.path = r.URL.Path
		c.key = r.Header.Get("x-api-key") + r.Header.Get("Authorization")
		c.beta = r.Header.Get("anthropic-beta")
		data, _ := io.ReadAll(r.Body)
		json.Unmarshal(data, &c.body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		io.WriteString(w, response)
	}))
	t.Cleanup(srv.Close)
	return srv, c
}

func apiReq(t *testing.T) StartRequest {
	dir := t.TempDir()
	return StartRequest{ProjectID: "p", RunID: "r", StepAttemptID: "s", Prompt: "기획서를 써라", Instructions: "## 역할\n기획",
		OutputSpec: []OutputSpec{
			{Key: "spec", Type: "markdown", Required: true, Path: filepath.Join(dir, "out", "spec.md")},
			{Key: "risks", Type: "json", Required: false, Path: filepath.Join(dir, "out", "risks.json")},
		}}
}

func answer(fields map[string]string) string {
	b, _ := json.Marshal(fields)
	return string(b)
}

func TestClaudeAPIWritesStructuredOutputs(t *testing.T) {
	text := answer(map[string]string{"message": "작성했습니다", "spec": "# 기획서", "risks": ""})
	body, _ := json.Marshal(map[string]any{
		"id": "msg_1", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"content": []any{map[string]any{"type": "text", "text": text}}, "stop_reason": "end_turn",
		"usage": map[string]any{"input_tokens": 100, "output_tokens": 40},
	})
	srv, c := fakeAPI(t, 200, string(body))
	in, out := 4.0, 20.0
	m := &ModelAPI{Kind: KindClaudeAPI, APIKey: "sk-ant-test", BaseURL: srv.URL, Prices: Prices{InputPerMTok: &in, OutputPerMTok: &out}}
	req := apiReq(t)
	s, err := m.Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	evs := drain(t, s)
	if cp := completed(t, evs); cp.Status != StatusSucceeded || cp.Text != "작성했습니다" {
		t.Fatalf("completed = %+v", cp)
	}
	if data, _ := os.ReadFile(req.OutputSpec[0].Path); string(data) != "# 기획서" {
		t.Fatalf("spec = %q", data)
	}
	if _, err := os.Stat(req.OutputSpec[1].Path); !os.IsNotExist(err) {
		t.Fatal("empty optional output was written")
	}
	// Request shape: key header, fallback beta, schema, system, default model.
	if !strings.HasSuffix(c.path, "/v1/messages") || c.key != "sk-ant-test" || !strings.Contains(c.beta, "server-side-fallback") {
		t.Fatalf("request: path=%s key=%q beta=%q", c.path, c.key, c.beta)
	}
	if c.body["model"] != DefaultClaudeModel || c.body["fallbacks"] != "default" {
		t.Fatalf("body model/fallbacks = %v %v", c.body["model"], c.body["fallbacks"])
	}
	schema := c.body["output_config"].(map[string]any)["format"].(map[string]any)["schema"].(map[string]any)
	if req := schema["required"].([]any); len(req) != 3 {
		t.Fatalf("schema required = %v", req)
	}
	for _, ev := range evs {
		if ev.Kind == KindUsage {
			u := payload[UsagePayload](t, ev)
			if want := 100.0/1e6*4 + 40.0/1e6*20; u.CostUSD == nil || math.Abs(*u.CostUSD-want) > 1e-12 {
				t.Fatalf("cost = %v", u.CostUSD)
			}
		}
	}
}

func TestClaudeAPIRefusalFails(t *testing.T) {
	body := `{"id":"m","type":"message","role":"assistant","model":"claude-opus-5-5","content":[],"stop_reason":"refusal",
		"stop_details":{"type":"refusal","category":"cyber","explanation":""},"usage":{"input_tokens":5,"output_tokens":0}}`
	srv, _ := fakeAPI(t, 200, body)
	s, _ := (&ModelAPI{Kind: KindClaudeAPI, APIKey: "k", BaseURL: srv.URL}).Start(context.Background(), apiReq(t))
	evs := drain(t, s)
	if cp := completed(t, evs); cp.Status != StatusFailed || !strings.Contains(cp.Error, "거절") || !strings.Contains(cp.Error, "cyber") {
		t.Fatalf("completed = %+v", cp)
	}
	for _, ev := range evs {
		if ev.Kind == KindUsage && payload[UsagePayload](t, ev).CostUSD != nil {
			t.Fatal("cost without prices must be unknown")
		}
	}
}

func TestOpenAIAPIWritesStructuredOutputs(t *testing.T) {
	text := answer(map[string]string{"message": "done", "spec": "# spec", "risks": `{"items":[]}`})
	body, _ := json.Marshal(map[string]any{
		"id": "resp_1", "object": "response", "created_at": 0, "status": "completed", "model": "test-model",
		"output": []any{map[string]any{"type": "message", "id": "m1", "role": "assistant", "status": "completed",
			"content": []any{map[string]any{"type": "output_text", "text": text, "annotations": []any{}}}}},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15,
			"input_tokens_details": map[string]any{"cached_tokens": 0}, "output_tokens_details": map[string]any{"reasoning_tokens": 0}},
	})
	srv, c := fakeAPI(t, 200, string(body))
	req := apiReq(t)
	req.Model = "test-model"
	s, err := (&ModelAPI{Kind: KindOpenAIAPI, APIKey: "sk-test", BaseURL: srv.URL + "/v1/"}).Start(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if cp := completed(t, drain(t, s)); cp.Status != StatusSucceeded {
		t.Fatalf("completed = %+v", cp)
	}
	if data, _ := os.ReadFile(req.OutputSpec[1].Path); string(data) != `{"items":[]}` {
		t.Fatalf("risks = %q", data)
	}
	format := c.body["text"].(map[string]any)["format"].(map[string]any)
	if !strings.HasSuffix(c.path, "/v1/responses") || c.key != "Bearer sk-test" || c.body["store"] != false || format["strict"] != true {
		t.Fatalf("request: path=%s key=%q store=%v format=%v", c.path, c.key, c.body["store"], format)
	}
}

func TestModelAPIStartChecks(t *testing.T) {
	req := apiReq(t)
	if _, err := (&ModelAPI{Kind: KindOpenAIAPI, APIKey: "k"}).Start(context.Background(), req); err == nil {
		t.Fatal("OpenAI without a model accepted")
	}
	if _, err := (&ModelAPI{Kind: KindClaudeAPI}).Start(context.Background(), req); err == nil {
		t.Fatal("missing API key accepted")
	}
	req.OutputSpec = append(req.OutputSpec, OutputSpec{Key: "bin", Type: "file", Path: "/x"})
	if _, err := (&ModelAPI{Kind: KindClaudeAPI, APIKey: "k"}).Start(context.Background(), req); err == nil {
		t.Fatal("file output accepted")
	}
}

func TestModelAPICancel(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-block:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(block)
	s, _ := (&ModelAPI{Kind: KindClaudeAPI, APIKey: "k", BaseURL: srv.URL}).Start(context.Background(), apiReq(t))
	next(t, s) // started
	time.Sleep(50 * time.Millisecond)
	s.Cancel(context.Background())
	if cp := completed(t, drain(t, s)); cp.Status != StatusCancelled {
		t.Fatalf("completed = %+v", cp)
	}
}
