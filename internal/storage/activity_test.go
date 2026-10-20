package storage

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/providers"
)

func TestActivityPersistsAndPaginatesWithinSource(t *testing.T) {
	path := filepath.Join(t.TempDir(), "activity.db")
	db := openTemp(t, path)
	ctx := context.Background()
	p, _ := db.CreateProject(ctx, domain.Project{Name: "기록"})
	w, _ := db.CreateWorkflow(ctx, p.ID, json.RawMessage(`{"schemaVersion":1,"title":"흐름","nodes":[]}`))
	a, err := db.CreateAssist(ctx, AssistRecord{ProjectID: p.ID, WorkflowID: w.ID, Kind: "edit"})
	if err != nil {
		t.Fatal(err)
	}
	r := NewOutputRecorder(func(text string) error {
		_, err := db.Change(ctx, func(c *Change) error {
			return c.Emit(p.ID, "", "", "activity.output", map[string]string{"assistId": a.ID, "text": text})
		})
		return err
	})
	for _, text := range []string{"진행 ", "출력"} {
		raw, _ := json.Marshal(providers.TextPayload{Text: text})
		if err := r.Add(providers.Event{Kind: providers.KindMessageDelta, Payload: raw}); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Change(ctx, func(c *Change) error {
		for i := 0; i < 205; i++ {
			if err := c.Emit(p.ID, "", "", "assist.ready", map[string]string{"assistId": a.ID}); err != nil {
				return err
			}
		}
		return c.Emit(p.ID, "", "", "assist.ready", map[string]string{"assistId": "other"})
	}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	db = openTemp(t, path)
	defer db.Close()
	page, err := db.Activity(ctx, p.ID, "", a.ID, 0)
	if err != nil || len(page.Events) != 200 || !page.HasMore {
		t.Fatalf("page: %d %+v %v", len(page.Events), page.HasMore, err)
	}
	older, err := db.Activity(ctx, p.ID, "", a.ID, page.Events[0].Sequence)
	if err != nil || len(older.Events) != 6 || older.HasMore || !strings.Contains(string(older.Events[0].Payload), "진행 출력") {
		t.Fatalf("older: %+v %v", older, err)
	}
	wrong, err := db.Activity(ctx, "another-project", "", a.ID, 0)
	if err != nil || len(wrong.Events) != 0 {
		t.Fatalf("leaked: %+v %v", wrong, err)
	}
}
