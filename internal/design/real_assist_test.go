package design_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"agent-office/internal/connect"
	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
)

// TestRealAssist runs one workflow assistant job with a real provider
// (uses the account's quota; off by default):
//
//	AGENT_OFFICE_REAL_ASSIST=edit:codex|diagram:claude|… [AGENT_OFFICE_REAL_ASSIST_OUT=<dir>] \
//	  go test ./internal/design/ -run TestRealAssist -v -timeout 30m
func TestRealAssist(t *testing.T) {
	spec := os.Getenv("AGENT_OFFICE_REAL_ASSIST")
	kind, provider, _ := cutColon(spec)
	if (kind != design.AssistEdit && kind != design.AssistDiagram) || (provider != "claude" && provider != "codex") {
		t.Skip("set AGENT_OFFICE_REAL_ASSIST=edit:codex, diagram:claude, … to run with a real provider")
	}
	e := newEnv(t)
	ctx := context.Background()
	verified := json.RawMessage(`{"call":true}`)
	var conn domain.ProviderConnection
	var prov providers.Provider
	if provider == "claude" {
		node, _ := exec.LookPath("node")
		script, _ := filepath.Abs(filepath.Join(testenv.RepoRoot(), "runners", "claude", "dist", "main.js"))
		conn, _ = e.db.SaveConnection(ctx, domain.ProviderConnection{Name: "Claude", Provider: "claude", VerifiedCapabilities: verified})
		prov = &providers.ClaudeBridge{Node: node, Script: script}
	} else {
		codex, ok := connect.FindExecutable("codex")
		if !ok {
			t.Skip("codex is not installed")
		}
		conn, _ = e.db.SaveConnection(ctx, domain.ProviderConnection{Name: "Codex", Provider: "codex", ExecutablePath: codex, VerifiedCapabilities: verified})
		prov = &providers.Codex{Executable: codex, Version: "real-assist"}
	}
	plugin, _ := connect.FindDiagramPlugin()
	e.svc.Providers = func(domain.ProviderConnection) (providers.Provider, error) { return prov, nil }
	e.svc.Connections = func(context.Context) ([]design.ConnectionInfo, error) {
		return []design.ConnectionInfo{{ID: conn.ID, Name: conn.Name, Provider: conn.Provider, Usable: true, Coding: true}}, nil
	}
	e.svc.DataDir = t.TempDir()
	e.svc.Diagram = engine.DiagramSkill{Plugin: plugin, Python: connect.FindPython()}

	wf, err := os.ReadFile(filepath.Join(testenv.RepoRoot(), "tests", "fixtures", "workflows", "service-dev.json"))
	if err != nil {
		t.Fatal(err)
	}
	p := testenv.SeedDraft(t, e.db, "VillaLens", wf, map[string]string{"a-planner": "기획", "a-architect": "아키텍트", "a-backend": "백엔드", "a-frontend": "프론트엔드", "a-qa": "QA"}, []string{"a-owner"})
	request := "QA 다음, 전달(deliver) 전에 보안 검토(리뷰) 업무를 하나 추가하고 QA 담당에게 맡겨 줘"
	if kind == design.AssistDiagram {
		request = "사람이 하는 승인 단계를 눈에 띄게"
	}
	rec := e.assist(design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: kind, Request: request, ConnectionID: conn.ID})
	t.Logf("status=%s error=%q result=%s", rec.Status, rec.Error, clip(string(rec.Result), 1500))
	if rec.Status != design.AssistReady {
		t.Fatalf("assist %s: %s", rec.Status, rec.Error)
	}
	switch kind {
	case design.AssistEdit:
		var res design.EditResult
		json.Unmarshal(rec.Result, &res)
		if len(res.Changes.Added) == 0 || !res.CanVersion {
			t.Fatalf("edit result: %+v", res.Changes)
		}
		if _, err := e.svc.ApplyAssist(ctx, rec.ID); err != nil {
			t.Fatal(err)
		}
	case design.AssistDiagram:
		html, err := e.svc.AssistDiagram(ctx, rec.ID)
		if err != nil {
			t.Fatal(err)
		}
		if out := os.Getenv("AGENT_OFFICE_REAL_ASSIST_OUT"); out != "" {
			os.MkdirAll(out, 0o700)
			os.WriteFile(filepath.Join(out, "pipeline-"+provider+".html"), []byte(html), 0o600)
		}
	}
}

func cutColon(s string) (string, string, bool) {
	for i := range s {
		if s[i] == ':' {
			return s[:i], s[i+1:], true
		}
	}
	return s, "", false
}

func clip(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
