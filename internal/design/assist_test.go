package design_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"agent-office/internal/connect"
	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

var assistWorkflow = []byte(`{"schemaVersion": 1, "title": "기획 흐름", "nodes": [
  {"id": "plan", "title": "기획", "kind": "task", "assignmentId": "a-planner", "dependsOn": [], "outputs": [{"key": "spec", "type": "markdown"}]},
  {"id": "approve", "title": "기획 승인", "kind": "approval", "assignmentId": "a-owner", "dependsOn": ["plan"], "reworkTargets": ["plan"]}
]}`)

// capture records the start requests a provider receives.
type capture struct {
	*providers.TestProvider
	mu   sync.Mutex
	reqs []providers.StartRequest
}

func (c *capture) Start(ctx context.Context, req providers.StartRequest) (providers.Session, error) {
	c.mu.Lock()
	c.reqs = append(c.reqs, req)
	c.mu.Unlock()
	return c.TestProvider.Start(ctx, req)
}

func (c *capture) last() providers.StartRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.reqs[len(c.reqs)-1]
}

func assistEnv(t *testing.T) (*env, testenv.Project, *capture) {
	e := newEnv(t)
	p := testenv.SeedDraft(t, e.db, "기획 서비스", assistWorkflow, map[string]string{"a-planner": "기획"}, []string{"a-owner"})
	c := &capture{TestProvider: e.prov}
	e.svc.Providers = func(domain.ProviderConnection) (providers.Provider, error) { return c, nil }
	e.svc.DataDir = t.TempDir()
	e.svc.Diagram = engine.DiagramSkill{Plugin: filepath.Join(testenv.RepoRoot(), "third_party", "diagram-design"), Python: connect.FindPython()}
	return e, p, c
}

// script makes the scenario name write content to the output key.
func (e *env) script(name, key, content string) {
	e.prov.Scripts[name] = []providers.Step{
		{Write: &providers.WriteStep{Key: key, Content: content}},
		{Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded}},
	}
}

func (e *env) assist(in design.AssistInput) storage.AssistRecord {
	e.t.Helper()
	if in.ConnectionID == "" {
		in.ConnectionID = e.conn.ID
	}
	rec, err := e.svc.StartAssist(e.ctx, in)
	if err != nil {
		e.t.Fatal(err)
	}
	e.svc.Wait()
	rec, _ = e.db.Assist(e.ctx, rec.ID)
	return rec
}

// editWith returns an edit proposal: the seeded flow plus a review step.
func editWith(p testenv.Project) string {
	return `{"notes": "기획 뒤에 리뷰를 넣었습니다", "workflow": {"schemaVersion": 1, "title": "기획 흐름", "nodes": [
	  {"id": "plan", "title": "기획", "kind": "task", "assignmentId": "` + p.Assignments["a-planner"] + `", "dependsOn": [], "outputs": [{"key": "spec", "type": "markdown"}]},
	  {"id": "review", "title": "기획 리뷰", "kind": "review", "assignmentId": "` + p.Assignments["a-planner"] + `", "dependsOn": ["plan"],
	   "inputs": [{"name": "기획", "fromStep": "plan", "outputKey": "spec"}], "outputs": [{"key": "review", "type": "report"}], "reworkTargets": ["plan"]},
	  {"id": "approve", "title": "기획 승인", "kind": "approval", "assignmentId": "` + p.Assignments["a-owner"] + `", "dependsOn": ["review"], "reworkTargets": ["plan"]}
	]}}`
}

func TestAssistEditProposesAndApplies(t *testing.T) {
	e, p, c := assistEnv(t)
	e.script("edit-ok", "edit", editWith(p))
	rec := e.assist(design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistEdit, Request: "기획 뒤에 리뷰를 넣어 줘", Model: "edit-ok"})
	if rec.Status != design.AssistReady {
		t.Fatalf("status %s: %s", rec.Status, rec.Error)
	}
	req := c.last()
	for _, want := range []string{"기획 뒤에 리뷰를 넣어 줘", p.Assignments["a-planner"], `"approve"`, "reworkTargets"} {
		if !strings.Contains(req.Prompt, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, req.Prompt)
		}
	}
	if len(req.Plugins) != 0 {
		t.Fatal("an edit got the diagram skill")
	}
	var res design.EditResult
	json.Unmarshal(rec.Result, &res)
	if !res.CanVersion || len(res.Changes.Added) != 1 || res.Changes.Added[0] != "기획 리뷰" || len(res.Changes.Changed) != 1 || res.Notes == "" {
		t.Fatalf("result = %+v", res)
	}
	w, err := e.svc.ApplyAssist(e.ctx, rec.ID)
	if err != nil {
		t.Fatal(err)
	}
	if w.Revision != rec.BaseRevision+1 || !strings.Contains(string(w.Draft), `"review"`) {
		t.Fatalf("applied draft r%d: %s", w.Revision, w.Draft)
	}
	if got, _ := e.db.Assist(e.ctx, rec.ID); got.Status != design.AssistApplied {
		t.Fatalf("after apply: %s", got.Status)
	}
	if _, err := e.svc.ApplyAssist(e.ctx, rec.ID); !errors.Is(err, design.ErrNotReady) {
		t.Fatalf("applied twice: %v", err)
	}
}

// An edit made from an older revision is not applied over newer edits.
func TestAssistEditRefusesChangedWorkflow(t *testing.T) {
	e, p, _ := assistEnv(t)
	e.script("edit-ok", "edit", editWith(p))
	rec := e.assist(design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistEdit, Request: "리뷰 추가", Model: "edit-ok"})
	w, _ := e.db.Workflow(e.ctx, p.ID, p.WorkflowID)
	if _, err := e.db.SaveWorkflowDraft(e.ctx, p.ID, p.WorkflowID, w.Revision, w.Draft); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.ApplyAssist(e.ctx, rec.ID); !errors.Is(err, design.ErrAssistConflict) {
		t.Fatalf("apply over a newer draft: %v", err)
	}
}

func TestAssistEditFailuresAndChecks(t *testing.T) {
	e, p, _ := assistEnv(t)
	if _, err := e.svc.StartAssist(e.ctx, design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistEdit, ConnectionID: e.conn.ID}); !errors.Is(err, design.ErrInvalid) {
		t.Fatalf("empty request: %v", err)
	}
	e.script("edit-none", "edit", `{"notes": "흐름 없음"}`)
	if rec := e.assist(design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistEdit, Request: "x", Model: "edit-none"}); rec.Status != design.AssistFailed || !strings.Contains(rec.Error, "workflow") {
		t.Fatalf("no workflow: %s %q", rec.Status, rec.Error)
	}
	// A proposal with problems is shown with its issues, not hidden.
	e.script("edit-bad", "edit", strings.Replace(editWith(p), `"dependsOn": ["review"]`, `"dependsOn": ["nowhere"]`, 1))
	rec := e.assist(design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistEdit, Request: "x", Model: "edit-bad"})
	var res design.EditResult
	json.Unmarshal(rec.Result, &res)
	if rec.Status != design.AssistReady || res.CanVersion || len(res.Issues) == 0 {
		t.Fatalf("bad proposal: %s %+v", rec.Status, res)
	}
	if err := e.svc.DiscardAssist(e.ctx, rec.ID); err != nil {
		t.Fatal(err)
	}
}

const pipelineHTML = `<!doctype html><html lang="ko"><head><meta charset="utf-8"><title>p</title></head><body>
<svg viewBox="0 0 200 80" role="img" aria-labelledby="diagram-title diagram-desc" xmlns="http://www.w3.org/2000/svg">
<title id="diagram-title">기획 흐름</title><desc id="diagram-desc">기획 다음 승인</desc>
<rect x="10" y="10" width="80" height="40" fill="none" stroke="#2d3142"/><rect x="110" y="10" width="80" height="40" fill="none" stroke="#2d3142"/>
</svg></body></html>`

func TestAssistDiagramUsesSkillAndKeepsFile(t *testing.T) {
	e, p, c := assistEnv(t)
	e.script("pipe-ok", "pipeline", pipelineHTML)
	rec := e.assist(design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistDiagram, Request: "승인을 강조", Model: "pipe-ok"})
	if rec.Status != design.AssistReady {
		t.Fatalf("status %s: %s", rec.Status, rec.Error)
	}
	req := c.last()
	if len(req.Plugins) != 1 || req.OutputSpec[0].Type != "diagram" || !strings.HasSuffix(req.OutputSpec[0].Path, ".html") {
		t.Fatalf("request plugins=%v output=%+v", req.Plugins, req.OutputSpec)
	}
	for _, want := range []string{"SKILL.md", "기획 승인", "선행=plan", "승인을 강조"} {
		if !strings.Contains(req.Prompt, want) {
			t.Fatalf("prompt lacks %q", want)
		}
	}
	html, err := e.svc.AssistDiagram(e.ctx, rec.ID)
	if err != nil || html != pipelineHTML {
		t.Fatalf("diagram: %v", err)
	}
	var res design.DiagramResult
	json.Unmarshal(rec.Result, &res)
	if _, err := os.Stat(filepath.Join(e.svc.DataDir, filepath.FromSlash(res.Path))); err != nil {
		t.Fatalf("diagram file: %v", err)
	}
}

func TestAssistDiagramChecks(t *testing.T) {
	e, p, _ := assistEnv(t)
	e.script("pipe-text", "pipeline", "# 그림 대신 글")
	if rec := e.assist(design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistDiagram, Model: "pipe-text"}); rec.Status != design.AssistFailed || !strings.Contains(rec.Error, "<svg>") {
		t.Fatalf("no svg: %s %q", rec.Status, rec.Error)
	}
	if e.svc.Diagram.Python != "" {
		e.script("pipe-remote", "pipeline", `<html><body><svg viewBox="0 0 10 10"><rect width="5" height="5"/></svg><script src="https://cdn.example/x.js"></script></body></html>`)
		if rec := e.assist(design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistDiagram, Model: "pipe-remote"}); rec.Status != design.AssistFailed || !strings.Contains(rec.Error, "self_check.py") {
			t.Fatalf("self-check: %s %q", rec.Status, rec.Error)
		}
	}
	api, _ := e.db.SaveConnection(e.ctx, domain.ProviderConnection{Name: "API", Provider: "claude_api", VerifiedCapabilities: json.RawMessage(`{"call":true}`)})
	if _, err := e.svc.StartAssist(e.ctx, design.AssistInput{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistDiagram, ConnectionID: api.ID}); !errors.Is(err, design.ErrInvalid) {
		t.Fatalf("model API diagram: %v", err)
	}
}

func TestAssistRecoverMarksWorkingFailed(t *testing.T) {
	e, p, _ := assistEnv(t)
	rec, err := e.db.CreateAssist(e.ctx, storage.AssistRecord{ProjectID: p.ID, WorkflowID: p.WorkflowID, Kind: design.AssistEdit, Request: "x"})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Recover(e.ctx); err != nil {
		t.Fatal(err)
	}
	if got, _ := e.db.Assist(e.ctx, rec.ID); got.Status != design.AssistFailed {
		t.Fatalf("after recover: %s", got.Status)
	}
}
