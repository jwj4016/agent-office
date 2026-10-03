package domain

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func loadSpec(t *testing.T, name string) *WorkflowSpec {
	t.Helper()
	data, err := os.ReadFile("../../tests/fixtures/workflows/" + name)
	if err != nil {
		t.Fatal(err)
	}
	w, err := ParseWorkflow(data)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func codes(issues []Issue) string {
	var out []string
	for _, i := range issues {
		out = append(out, i.NodeID+":"+i.Code)
	}
	return strings.Join(out, ",")
}

func TestFixturesAreValid(t *testing.T) {
	for _, f := range []string{"service-dev.json", "legal-branch.json"} {
		if issues := ValidateStructure(loadSpec(t, f)); len(issues) > 0 {
			t.Errorf("%s: %v", f, issues)
		}
	}
}

func TestParseRejectsUnknownFields(t *testing.T) {
	_, err := ParseWorkflow([]byte(`{"schemaVersion":1,"title":"x","nodes":[],"nodez":[]}`))
	if err == nil {
		t.Fatal("unknown field accepted")
	}
}

// mutate applies fn to a fresh copy of a fixture and validates it.
func mutate(t *testing.T, fixture string, fn func(w *WorkflowSpec)) []Issue {
	t.Helper()
	w := loadSpec(t, fixture)
	fn(w)
	// Round-trip so the mutation is validated exactly as it would be saved.
	data, _ := json.Marshal(w)
	w2, err := ParseWorkflow(data)
	if err != nil {
		t.Fatal(err)
	}
	return ValidateStructure(w2)
}

func f() *bool { b := false; return &b }

func TestStructureErrors(t *testing.T) {
	cases := []struct {
		name, fixture, want string
		fn                  func(w *WorkflowSpec)
	}{
		{"duplicate id", "service-dev.json", "qa:duplicate_id", func(w *WorkflowSpec) { w.Node("deliver").ID = "qa" }},
		{"bad id", "service-dev.json", "../x:bad_id", func(w *WorkflowSpec) { w.Node("deliver").ID = "../x" }},
		{"unknown dependency", "service-dev.json", "qa:unknown_dependency", func(w *WorkflowSpec) { w.Node("qa").DependsOn = []string{"nope"} }},
		{"cycle", "service-dev.json", "approve:cycle", func(w *WorkflowSpec) { w.Node("plan").DependsOn = []string{"qa"} }},
		{"self dependency", "service-dev.json", "plan:self_dependency", func(w *WorkflowSpec) { w.Node("plan").DependsOn = []string{"plan"} }},
		{"input from non-ancestor", "service-dev.json", "backend:input_not_ancestor", func(w *WorkflowSpec) {
			w.Node("backend").Inputs = append(w.Node("backend").Inputs, Input{Name: "fe", FromStep: "frontend", OutputKey: "change"})
		}},
		{"unknown output key", "service-dev.json", "qa:unknown_output", func(w *WorkflowSpec) { w.Node("qa").Inputs[0].OutputKey = "nope" }},
		{"rework target not ancestor", "service-dev.json", "review:rework_not_ancestor", func(w *WorkflowSpec) { w.Node("review").ReworkTargets = []string{"qa"} }},
		{"rework target is approval", "service-dev.json", "review:rework_bad_target", func(w *WorkflowSpec) { w.Node("review").ReworkTargets = []string{"approve"} }},
		{"rework on task", "service-dev.json", "qa:rework_on_wrong_kind", func(w *WorkflowSpec) { w.Node("qa").ReworkTargets = []string{"plan"} }},
		{"task without outputs", "service-dev.json", "plan:missing_outputs", func(w *WorkflowSpec) { w.Node("plan").Outputs = nil }},
		{"approval with outputs", "service-dev.json", "approve:approval_outputs", func(w *WorkflowSpec) {
			w.Node("approve").Outputs = []Output{{Key: "x", Type: OutMarkdown}}
		}},
		{"bad output type", "service-dev.json", "plan:bad_output_type", func(w *WorkflowSpec) { w.Node("plan").Outputs[0].Type = "pdf" }},
		{"bad kind", "service-dev.json", "plan:bad_kind", func(w *WorkflowSpec) { w.Node("plan").Kind = "meeting" }},
		{"bad timeout", "service-dev.json", "plan:bad_timeout", func(w *WorkflowSpec) { w.Node("plan").Limits = &NodeLimits{Timeout: "soon"} }},
		{"schema version", "service-dev.json", ":schema_version", func(w *WorkflowSpec) { w.SchemaVersion = 2 }},

		{"join is not a join", "legal-branch.json", "route:bad_join", func(w *WorkflowSpec) { w.Node("route").Routing.JoinStep = "report" }},
		{"missing default", "legal-branch.json", "route:missing_target", func(w *WorkflowSpec) { w.Node("route").Routing.DefaultTarget = "" }},
		{"bad operator", "legal-branch.json", "route:bad_operator", func(w *WorkflowSpec) { w.Node("route").Routing.Branches[0].Operator = "gt" }},
		{"in needs array", "legal-branch.json", "route:bad_in_value", func(w *WorkflowSpec) {
			w.Node("route").Routing.Branches[0].Operator = OpIn
		}},
		{"bad field path", "legal-branch.json", "route:bad_field_path", func(w *WorkflowSpec) { w.Node("route").Routing.Source.FieldPath = "a..b" }},
		{"route source not json", "legal-branch.json", "route:route_source_not_json", func(w *WorkflowSpec) {
			w.Node("scope").Outputs[0].Type = OutMarkdown
		}},
		{"branch start with extra dependency", "legal-branch.json", "legal:ambiguous_branch_dependency", func(w *WorkflowSpec) {
			w.Node("legal").DependsOn = []string{"route", "scope"}
		}},
		{"untargeted child of condition", "legal-branch.json", "report:untargeted_child", func(w *WorkflowSpec) {
			w.Node("report").DependsOn = []string{"merge", "route"}
		}},
		{"branch not joined", "legal-branch.json", "legal:branch_not_joined", func(w *WorkflowSpec) {
			w.Node("merge").DependsOn = []string{"route"}
		}},
		{"node after join depends into branch", "legal-branch.json", "report:ambiguous_branch_dependency", func(w *WorkflowSpec) {
			w.Node("report").DependsOn = []string{"merge", "legal_review"}
		}},
		{"required input from optional branch", "legal-branch.json", "report:ambiguous_required_input", func(w *WorkflowSpec) {
			w.Node("report").Inputs[1].Required = nil
		}},
		{"nested condition", "legal-branch.json", "legal_review:nested_condition", func(w *WorkflowSpec) {
			n := w.Node("legal_review")
			n.Kind = KindCondition
			n.AssignmentID, n.Inputs, n.Outputs, n.ReworkTargets = "", nil, nil, nil
			n.Routing = &Routing{Source: RouteSource{FromStep: "scope", OutputKey: "scope", FieldPath: "x"},
				Branches: []Branch{{Operator: OpExists, TargetStep: "merge"}}, DefaultTarget: "merge", JoinStep: "merge"}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := codes(mutate(t, c.fixture, c.fn))
			if !strings.Contains(","+got+",", ","+c.want+",") {
				t.Fatalf("want %s, got [%s]", c.want, got)
			}
		})
	}
}

func TestOptionalInputFromBranchIsAllowed(t *testing.T) {
	// legal-branch.json already marks the legal memo optional for report.
	if issues := mutate(t, "legal-branch.json", func(w *WorkflowSpec) { w.Node("report").Inputs[1].Required = f() }); len(issues) > 0 {
		t.Fatal(issues)
	}
}

func TestValidateAssignments(t *testing.T) {
	w := loadSpec(t, "service-dev.json")
	all := map[string]AssignmentInfo{
		"a-planner": {"ai", true}, "a-architect": {"ai", true}, "a-backend": {"ai", true},
		"a-frontend": {"ai", true}, "a-qa": {"ai", true}, "a-owner": {"human", true},
	}
	if issues := ValidateAssignments(w, all); len(issues) > 0 {
		t.Fatal(issues)
	}

	// T04: an unconnected AI assignment saves but blocks running.
	partial := map[string]AssignmentInfo{}
	for k, v := range all {
		partial[k] = v
	}
	partial["a-qa"] = AssignmentInfo{"ai", false}
	issues := ValidateAssignments(w, partial)
	if codes(issues) != "qa:ai_not_connected" || HasSeverity(issues, SevError) || !HasSeverity(issues, SevRun) {
		t.Fatalf("got %v", issues)
	}

	// An approval assigned to AI, or an assignment from another project.
	partial["a-owner"] = AssignmentInfo{"ai", true}
	delete(partial, "a-backend")
	got := codes(ValidateAssignments(w, partial))
	for _, want := range []string{"approve:approval_needs_human", "backend:unknown_assignment"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in %s", want, got)
		}
	}
}
