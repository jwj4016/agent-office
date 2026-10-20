package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-office/internal/design"
	"agent-office/internal/providers"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func TestChooseFolderOpensAtCurrentFolder(t *testing.T) {
	a := startApp(t)
	start := t.TempDir()
	var asked runtime.OpenDialogOptions
	a.folderDialog = func(_ context.Context, o runtime.OpenDialogOptions) (string, error) {
		asked = o
		return "/picked", nil
	}
	if got, err := a.ChooseFolder("작업 폴더 선택", start); err != nil || got != "/picked" {
		t.Fatalf("ChooseFolder = %q, %v", got, err)
	}
	if asked.DefaultDirectory != start || !asked.CanCreateDirectories || asked.Title != "작업 폴더 선택" {
		t.Fatalf("dialog options = %+v", asked)
	}
	// A folder that no longer exists is not offered as the start.
	a.ChooseFolder("", filepath.Join(start, "gone"))
	if asked.DefaultDirectory != "" || asked.Title != "폴더 선택" {
		t.Fatalf("missing start folder: %+v", asked)
	}
}

func TestAIActivityRejectsOtherProjectAndMixedSources(t *testing.T) {
	a := startApp(t)
	p, version := buildService(t, a, "기록 서비스")
	started := need(a.StartRun(p.ID, version))
	run := started.RunID
	other, _ := buildService(t, a, "다른 서비스")
	if _, err := a.AIActivity(other.ID, run, "", 0); err == nil {
		t.Fatal("cross-project activity allowed")
	}
	if _, err := a.AIActivity(p.ID, run, "mixed", 0); err == nil {
		t.Fatal("mixed sources allowed")
	}
	page, err := a.AIActivity(p.ID, run, "", 0)
	if err != nil || len(page.Events) == 0 {
		t.Fatalf("activity: %+v %v", page, err)
	}
}

// The person asks for a pipeline diagram and saves it where they choose.
func TestWorkflowDiagramAssistAndSave(t *testing.T) {
	a := startApp(t)
	p, _ := buildService(t, a, "그림 서비스")
	conn := need(a.EnsureTestConnection())
	w := need(a.ListWorkflows(p.ID))[0]
	html := `<html><body><svg viewBox="0 0 10 10" role="img" aria-labelledby="diagram-t diagram-d"><title id="diagram-t">흐름</title><desc id="diagram-d">흐름 그림</desc><rect width="5" height="5"/></svg></body></html>`
	a.testProvider.Scripts["pipe"] = []providers.Step{
		{Write: &providers.WriteStep{Key: "pipeline", Content: html}},
		{Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded}},
	}
	rec, err := a.StartWorkflowAssist(design.AssistInput{ProjectID: p.ID, WorkflowID: w.ID, Kind: design.AssistDiagram, ConnectionID: conn.ID, Model: "pipe"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(e2eWait)
	for rec.Status == design.AssistWorking && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		rec = need(a.GetWorkflowAssist(rec.ID))
	}
	if rec.Status != design.AssistReady {
		t.Fatalf("status %s: %s", rec.Status, rec.Error)
	}
	if list := need(a.ListWorkflowAssists(p.ID, w.ID)); len(list) != 1 || list[0].ID != rec.ID {
		t.Fatalf("list = %+v", list)
	}
	if got := need(a.WorkflowDiagram(rec.ID)); got != html {
		t.Fatalf("diagram = %q", got)
	}
	dst := filepath.Join(t.TempDir(), "흐름 그림")
	a.saveDialog = func(context.Context, runtime.SaveDialogOptions) (string, error) { return dst, nil }
	path := need(a.SaveWorkflowDiagram(rec.ID))
	if !strings.HasSuffix(path, ".html") {
		t.Fatalf("saved as %q", path)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != html {
		t.Fatalf("saved file: %v", err)
	}
	if _, err := a.ApplyWorkflowAssist(rec.ID); err == nil {
		t.Fatal("a diagram was applied as an edit")
	}
}
