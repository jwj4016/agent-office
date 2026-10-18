package engine_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-office/internal/connect"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

// TestRealDiagram runs one diagram step with a real provider and the
// bundled diagram-design skill (uses the account's quota; off by default):
//
//	AGENT_OFFICE_REAL_DIAGRAM=claude|codex [AGENT_OFFICE_REAL_DIAGRAM_OUT=<dir>] \
//	  go test ./internal/engine/ -run TestRealDiagram -v -timeout 30m
//
// Tool approvals are accepted only for commands that touch the skill or
// the attempt's own folders; everything else is declined. A question is
// answered with "use the default style". The result is copied to
// AGENT_OFFICE_REAL_DIAGRAM_OUT when set.
func TestRealDiagram(t *testing.T) {
	kind := os.Getenv("AGENT_OFFICE_REAL_DIAGRAM")
	if kind != "claude" && kind != "codex" {
		t.Skip("set AGENT_OFFICE_REAL_DIAGRAM=claude or codex to run with a real provider")
	}
	ctx := context.Background()
	dataDir := t.TempDir()
	db, err := storage.Open(ctx, filepath.Join(dataDir, "agent-office.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	plugin, ok := connect.FindDiagramPlugin()
	if !ok {
		t.Fatal("bundled diagram-design plugin not found")
	}
	diagram := engine.DiagramSkill{Plugin: plugin, Python: connect.FindPython()}
	verified := json.RawMessage(`{"call":true}`)
	var conn domain.ProviderConnection
	var prov providers.Provider
	switch kind {
	case "claude":
		node, _ := exec.LookPath("node")
		script, _ := filepath.Abs(filepath.Join(testenv.RepoRoot(), "runners", "claude", "dist", "main.js"))
		conn, _ = db.SaveConnection(ctx, domain.ProviderConnection{Name: "Claude", Provider: "claude", VerifiedCapabilities: verified})
		prov = &providers.ClaudeBridge{Node: node, Script: script}
	case "codex":
		codex, ok := connect.FindExecutable("codex")
		if !ok {
			t.Skip("codex is not installed")
		}
		conn, _ = db.SaveConnection(ctx, domain.ProviderConnection{Name: "Codex", Provider: "codex", ExecutablePath: codex, VerifiedCapabilities: verified})
		prov = &providers.Codex{Executable: codex, Version: "real-diagram"}
	}

	wf := []byte(`{"schemaVersion": 1, "title": "아키텍처 그림", "nodes": [
	  {"id": "arch", "title": "시스템 아키텍처 다이어그램", "kind": "task", "assignmentId": "a-architect", "dependsOn": [],
	   "instructions": "VillaLens(서울 빌라 상세 분석 서비스)의 시스템 아키텍처를 그린다. 사용자 → 웹 프론트엔드 → 백엔드 API → 수집기(국토부 실거래가 API, 건축물대장, 등기부) → 원본 저장소 → 분석 엔진 → 리포트. 상자는 8개 이하로 간결하게.",
	   "outputs": [{"key": "architecture", "type": "diagram"}]}]}`)
	p := testenv.SeedDraft(t, db, "VillaLens", wf, map[string]string{"a-architect": "아키텍트"}, nil)
	list, _ := db.Assignments(ctx, p.ID)
	for _, a := range list {
		a.ConnectionID, a.Model = conn.ID, ""
		if _, err := db.SaveAssignment(ctx, a); err != nil {
			t.Fatal(err)
		}
	}

	eng := engine.New(engine.Config{
		DB: db, DataDir: dataDir, Logf: t.Logf, Diagram: diagram,
		Providers: func(domain.ProviderConnection) (providers.Provider, error) { return prov, nil },
	})
	ectx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { eng.Run(ectx); close(done) }()
	defer func() { stop(); <-done }()

	w, _ := db.Workflow(ctx, p.ID, p.WorkflowID)
	v, res, err := db.ConfirmVersion(ctx, p.ID, w.ID, w.Revision)
	if err != nil {
		t.Fatalf("confirm: %v %v", err, res.Issues)
	}
	runID, err := eng.StartRun(ctx, p.ID, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("provider=%s plugin=%s python=%q", kind, plugin, diagram.Python)

	allowed := func(detail string) bool {
		return strings.Contains(detail, plugin) || strings.Contains(detail, dataDir)
	}
	decided := map[string]bool{}
	deadline := time.Now().Add(25 * time.Minute)
	for time.Now().Before(deadline) {
		d, err := eng.RunDetail(ctx, p.ID, runID)
		if err != nil {
			t.Fatal(err)
		}
		s := d.Step("arch")
		switch s.Status {
		case engine.StSucceeded:
			c, err := eng.ReadArtifact(ctx, p.ID, s.Artifacts[0].ID)
			if err != nil || !c.HashOK {
				t.Fatalf("artifact: %v", err)
			}
			if out := os.Getenv("AGENT_OFFICE_REAL_DIAGRAM_OUT"); out != "" {
				os.MkdirAll(out, 0o700)
				os.WriteFile(filepath.Join(out, "architecture-"+kind+".html"), []byte(c.Content), 0o600)
			}
			logTools(t, db, runID)
			t.Logf("SUCCEEDED: %d bytes", len(c.Content))
			return
		case engine.StFailed, engine.StCancelled, engine.StInterrupted:
			logTools(t, db, runID)
			t.Fatalf("step %s: %s", s.Status, s.Error)
		}
		items, _ := eng.Inbox(ctx)
		for _, it := range items {
			key := it.Kind + "-" + firstOf(it.ApprovalID, it.MessageID)
			if decided[key] {
				continue
			}
			decided[key] = true
			switch it.Kind {
			case engine.InboxToolApproval:
				ok := allowed(it.Detail)
				_, err = eng.DecideToolApproval(ctx, p.ID, it.ApprovalID, ok)
				t.Logf("TOOL accept=%v %s", ok, clipText(it.Detail, 300))
			case engine.InboxQuestion:
				_, err = eng.AnswerQuestion(ctx, p.ID, it.MessageID, "스킬의 기본 스타일로 진행하세요.")
				t.Logf("QUESTION %s", clipText(it.Detail, 300))
			}
			if err != nil {
				t.Logf("decide %s: %v", key, err)
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatal("the diagram step did not finish in time")
}

// logTools prints the tools the provider used, to see whether it loaded
// the skill and read its files.
func logTools(t *testing.T, db *storage.DB, runID string) {
	rows, err := db.Read().Query(`SELECT kind, payload FROM execution_events WHERE run_id = ? AND kind IN ('provider.tool', 'provider.started') ORDER BY sequence`, runID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var k, payload string
		rows.Scan(&k, &payload)
		t.Logf("%s %s", k, clipText(payload, 240))
	}
}
