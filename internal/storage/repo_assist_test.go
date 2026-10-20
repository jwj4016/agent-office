package storage

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"agent-office/internal/domain"
)

func TestAssistLifecycle(t *testing.T) {
	db := openTemp(t, "")
	defer db.Close()
	ctx := context.Background()
	p, err := db.CreateProject(ctx, domain.Project{Name: "서비스"})
	if err != nil {
		t.Fatal(err)
	}
	w, err := db.CreateWorkflow(ctx, p.ID, json.RawMessage(`{"schemaVersion":1,"title":"흐름","nodes":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	a, err := db.CreateAssist(ctx, AssistRecord{ProjectID: p.ID, WorkflowID: w.ID, Kind: "edit", Request: "리뷰 추가", BaseRevision: w.Revision})
	if err != nil || a.Status != "working" || a.Request != "리뷰 추가" {
		t.Fatalf("create: %+v %v", a, err)
	}
	if _, err := db.CreateAssist(ctx, AssistRecord{ProjectID: "prj-other", WorkflowID: w.ID, Kind: "edit"}); err == nil {
		t.Fatal("assist accepted for a workflow of another project")
	}
	if ok, _ := db.UpdateAssist(ctx, a.ID, "ready", json.RawMessage(`{"notes":"x"}`), "", "working"); !ok {
		t.Fatal("working -> ready refused")
	}
	if ok, _ := db.UpdateAssist(ctx, a.ID, "ready", nil, "", "working"); ok {
		t.Fatal("late update overwrote a finished job")
	}
	got, _ := db.Assist(ctx, a.ID)
	if got.Status != "ready" || string(got.Result) != `{"notes":"x"}` {
		t.Fatalf("after update: %+v", got)
	}
	list, _ := db.Assists(ctx, w.ID, 10)
	all, _ := db.Assists(ctx, "", 10)
	if len(list) != 1 || len(all) != 1 {
		t.Fatalf("lists: %d %d", len(list), len(all))
	}
	if _, err := db.Assist(ctx, "ast-none"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	res, err := db.ValidateDraft(ctx, p.ID, json.RawMessage(`{"schemaVersion":1,"title":"x","nodes":[{"id":"a","title":"A","kind":"task","dependsOn":["zz"]}]}`))
	if err != nil || res.CanVersion {
		t.Fatalf("validate: %+v %v", res, err)
	}
}
