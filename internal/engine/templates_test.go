package engine_test

import (
	"encoding/json"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/templates"
	"agent-office/internal/testenv"
)

// Every built-in template runs to the end once people are assigned: AI
// for tasks, the person for reviews and approvals (M5 gate).
func TestTemplatesRunEndToEnd(t *testing.T) {
	for _, tpl := range templates.List() {
		if tpl.ID == "blank" {
			continue
		}
		t.Run(tpl.ID, func(t *testing.T) {
			var spec map[string]any
			if err := json.Unmarshal(tpl.Workflow, &spec); err != nil {
				t.Fatal(err)
			}
			for _, raw := range spec["nodes"].([]any) {
				n := raw.(map[string]any)
				n["assignmentId"] = "a-ai"
				if n["kind"] == "approval" || n["kind"] == "review" {
					n["assignmentId"] = "a-owner"
				}
			}
			draft, _ := json.Marshal(spec)
			h := newHarness(t)
			p := testenv.SeedDraft(t, h.db, tpl.Title, draft, map[string]string{"a-ai": "담당"}, []string{"a-owner"})
			runID := h.start(p)
			people := 0
			for {
				d := h.waitFor(p.ID, runID, "a person's turn or the end", func(d engine.RunDetail) bool {
					if d.Status == engine.RunSucceeded || d.Status == engine.RunFailed {
						return true
					}
					for _, s := range d.Steps {
						if s.Status == engine.StWaitingApproval || s.Status == engine.StWaitingHuman {
							return true
						}
					}
					return false
				})
				if d.Status == engine.RunFailed {
					t.Fatalf("run failed\n%s", describe(d))
				}
				if d.Status == engine.RunSucceeded {
					break
				}
				for _, s := range d.Steps {
					switch s.Status {
					case engine.StWaitingApproval:
						h.approve(p.ID, runID, s.ID)
						people++
					case engine.StWaitingHuman:
						h.review(p.ID, runID, s.ID, engine.ReviewPass, "확인했습니다")
						people++
					}
				}
			}
			run := h.detail(p.ID, runID)
			for _, s := range run.Steps {
				if s.Status != engine.StSucceeded {
					t.Fatalf("step %s = %s", s.ID, s.Status)
				}
			}
			if people == 0 {
				t.Fatal("no person step in the template")
			}
		})
	}
}
