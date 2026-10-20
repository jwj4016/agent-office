package engine_test

import (
	"testing"
	"time"

	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
)

var threeWayWorkflow = []byte(`{
  "schemaVersion": 1, "title": "동시 조사",
  "nodes": [
    {"id": "a", "title": "조사 A", "kind": "task", "assignmentId": "a-planner", "dependsOn": [], "outputs": [{"key": "a", "type": "markdown"}]},
    {"id": "b", "title": "조사 B", "kind": "task", "assignmentId": "a-architect", "dependsOn": [], "outputs": [{"key": "b", "type": "markdown"}]},
    {"id": "c", "title": "조사 C", "kind": "task", "assignmentId": "a-qa", "dependsOn": [], "outputs": [{"key": "c", "type": "markdown"}]}
  ]}`)

func running(d engine.RunDetail) int {
	n := 0
	for _, s := range d.Steps {
		if s.Status == engine.StRunning {
			n++
		}
	}
	return n
}

// The limit on AI steps running at once holds the third step back, and
// changing it takes effect without restarting the app.
func TestMaxActiveChangesWhileRunning(t *testing.T) {
	h := newHarness(t)
	h.prov.Scripts["hold"] = []providers.Step{{Sleep: "1m"}}
	for _, step := range []string{"a", "b", "c"} {
		h.useScenario(step, "hold")
	}
	p := testenv.SeedDraft(t, h.db, "조사", threeWayWorkflow,
		map[string]string{"a-planner": "기획", "a-architect": "분석", "a-qa": "보조"}, nil)
	runID := h.start(p)

	h.waitFor(p.ID, runID, "two running", func(d engine.RunDetail) bool { return running(d) == 2 })
	time.Sleep(300 * time.Millisecond)
	h.eng.Wake()
	time.Sleep(100 * time.Millisecond)
	if d := h.detail(p.ID, runID); running(d) != 2 {
		t.Fatalf("default limit not held:\n%s", describe(d))
	}

	h.eng.SetMaxActive(3)
	h.waitFor(p.ID, runID, "three running", func(d engine.RunDetail) bool { return running(d) == 3 })

	// Lowering it leaves running steps alone.
	h.eng.SetMaxActive(1)
	time.Sleep(100 * time.Millisecond)
	if d := h.detail(p.ID, runID); running(d) != 3 {
		t.Fatalf("lowering the limit stopped running steps:\n%s", describe(d))
	}
}
