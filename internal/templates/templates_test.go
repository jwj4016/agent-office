package templates

import (
	"strings"
	"testing"

	"agent-office/internal/domain"
)

// Every template is a valid flow whose only problem is that nobody is
// assigned yet (the user picks who does each step).
func TestTemplatesAreValidFlows(t *testing.T) {
	want := []string{"blank", "service-dev", "game-launch", "legal-review", "real-estate"}
	list := List()
	if len(list) != len(want) {
		t.Fatalf("templates = %d, want %d", len(list), len(want))
	}
	for i, tpl := range list {
		if tpl.ID != want[i] || tpl.Title == "" || tpl.Description == "" {
			t.Fatalf("template %d = %+v", i, tpl)
		}
		w, err := domain.ParseWorkflow(tpl.Workflow)
		if err != nil {
			t.Fatalf("%s: %v", tpl.ID, err)
		}
		issues := domain.ValidateStructure(w)
		if tpl.ID == "blank" && len(issues) == 1 && issues[0].Code == "empty" {
			continue // the user adds the first step
		}
		if len(issues) > 0 {
			t.Fatalf("%s: %v", tpl.ID, issues)
		}
		for _, n := range w.Nodes {
			if n.AssignmentID != "" {
				t.Fatalf("%s/%s comes with an assignee", tpl.ID, n.ID)
			}
		}
		for _, is := range domain.ValidateAssignments(w, nil) {
			if is.Code != "missing_assignment" {
				t.Fatalf("%s: %v", tpl.ID, is)
			}
		}
	}
}

// Legal and finance-like templates leave the final call to a person and
// tell the AI not to invent research it could not do (spec §12).
func TestDomainTemplatesKeepPeopleInCharge(t *testing.T) {
	for _, id := range []string{"game-launch", "legal-review", "real-estate"} {
		raw, _ := Get(id)
		w, err := domain.ParseWorkflow(raw)
		if err != nil {
			t.Fatal(err)
		}
		var people, honest bool
		for _, n := range w.Nodes {
			people = people || n.Kind == domain.KindApproval || n.Kind == domain.KindReview
			honest = honest || strings.Contains(n.Instructions, "지어내지 않") || strings.Contains(n.Instructions, "조사한 것처럼 쓰지 않") ||
				strings.Contains(n.Instructions, "조사한 근거로 적는다")
		}
		if !people || !honest {
			t.Fatalf("%s: person step %v, no-invention instruction %v", id, people, honest)
		}
	}
}
