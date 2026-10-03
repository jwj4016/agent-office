package domain

import (
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
)

func keys(m map[string]bool) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestGraphOrderAndDescendants(t *testing.T) {
	g := NewGraph(loadSpec(t, "service-dev.json"))
	if got := strings.Join(g.Order(), ","); got != "plan,approve,design,backend,frontend,integrate,review,qa,deliver" {
		t.Fatalf("order = %s", got)
	}
	if got := keys(g.Descendants("backend")); got != "deliver,integrate,qa,review" {
		t.Fatalf("descendants(backend) = %s", got)
	}
	if got := keys(g.Descendants("backend", "frontend")); got != "deliver,integrate,qa,review" {
		t.Fatalf("descendants(backend,frontend) = %s", got)
	}
}

func TestBranchMembers(t *testing.T) {
	g := NewGraph(loadSpec(t, "legal-branch.json"))
	if got := keys(g.BranchMembers("route", "legal")); got != "legal,legal_review" {
		t.Fatalf("branch = %s", got)
	}
	if got := keys(g.BranchMembers("route", "merge")); got != "" {
		t.Fatalf("empty branch = %s", got)
	}
	if got := strings.Join(g.BranchTargets("route"), ","); got != "legal,merge" {
		t.Fatalf("targets = %s", got)
	}
}

func TestEvaluateRouting(t *testing.T) {
	r := &Routing{
		Source: RouteSource{FieldPath: "legal.needed"},
		Branches: []Branch{
			{Operator: OpEq, Value: json.RawMessage(`true`), TargetStep: "legal"},
			{Operator: OpIn, Value: json.RawMessage(`["maybe", 2]`), TargetStep: "ask"},
		},
		DefaultTarget: "merge",
	}
	cases := []struct{ doc, want string }{
		{`{"legal":{"needed":true}}`, "legal"},
		{`{"legal":{"needed":false}}`, "merge"},
		{`{"legal":{"needed":"maybe"}}`, "ask"},
		{`{"legal":{"needed":2}}`, "ask"},
	}
	for _, c := range cases {
		got, err := EvaluateRouting(r, []byte(c.doc))
		if err != nil || got != c.want {
			t.Errorf("%s -> %q, %v; want %q", c.doc, got, err, c.want)
		}
	}
	if _, err := EvaluateRouting(r, []byte(`{"legal":{}}`)); !errors.Is(err, ErrRoutePath) {
		t.Errorf("missing path: %v", err)
	}
	if _, err := EvaluateRouting(r, []byte(`not json`)); err == nil {
		t.Error("bad json accepted")
	}

	exists := &Routing{Source: RouteSource{FieldPath: "items.1.id"},
		Branches: []Branch{{Operator: OpExists, TargetStep: "x"}}, DefaultTarget: "d"}
	for doc, want := range map[string]string{`{"items":[{},{"id":1}]}`: "x", `{"items":[{}]}`: "d", `{}`: "d"} {
		if got, err := EvaluateRouting(exists, []byte(doc)); err != nil || got != want {
			t.Errorf("exists %s -> %q, %v; want %q", doc, got, err, want)
		}
	}
}
