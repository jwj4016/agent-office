package design

import (
	"context"
	"os"
	"strings"
	"testing"

	"agent-office/internal/providers"
)

// promptOnly behaves like a real CLI agent: it sees only the prompt
// (not OutputSpec), so it can write the result only if the prompt says
// where. It writes to the first path it finds after "결과 파일 경로:".
type promptOnly struct {
	*providers.TestProvider
	content string
	reply   string
}

func (p promptOnly) Start(ctx context.Context, req providers.StartRequest) (providers.Session, error) {
	req.OutputSpec = nil
	if i := strings.Index(req.Prompt, "결과 파일 경로: "); i >= 0 && p.content != "" {
		path := strings.TrimSpace(strings.SplitN(req.Prompt[i+len("결과 파일 경로: "):], "\n", 2)[0])
		os.WriteFile(path, []byte(p.content), 0o600)
	}
	p.TestProvider.Scripts = map[string][]providers.Step{"x": {{Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded, Text: p.reply}}}}
	req.Model = "x"
	return p.TestProvider.Start(ctx, req)
}

// Real-AI finding: the designer prompt must say where to write.
func TestRunTellsAgentWhereToWrite(t *testing.T) {
	raw, err := Run(context.Background(), promptOnly{TestProvider: &providers.TestProvider{}, content: `{"ok":true}`}, "", "목표")
	if err != nil || string(raw) != `{"ok":true}` {
		t.Fatalf("Run = %s, %v", raw, err)
	}
}

// An agent that answers in text with a fenced JSON block still works.
func TestRunAcceptsFencedJSONReply(t *testing.T) {
	reply := "설계안입니다.\n```json\n{\"ok\": true}\n```\n"
	raw, err := Run(context.Background(), promptOnly{TestProvider: &providers.TestProvider{}, reply: reply}, "", "목표")
	if err != nil || strings.TrimSpace(string(raw)) != `{"ok": true}` {
		t.Fatalf("Run = %q, %v", raw, err)
	}
}
