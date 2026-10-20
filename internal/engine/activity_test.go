package engine_test

import (
	"encoding/json"
	"strings"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
)

func TestActivityKeepsOutputAfterAttemptEnds(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "출력 보존")
	h.prov.Scripts["output"] = []providers.Step{
		{Emit: providers.KindMessageDelta, Payload: json.RawMessage(`{"text":"기획을 정리하고 있습니다"}`)},
		{Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded, Text: "# 기획 완료"}},
	}
	h.useScenario("plan", "output")
	runID := h.start(p)
	h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
	h.stopEngine()
	page, err := h.db.Activity(h.ctx, p.ID, runID, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var output, final bool
	for _, ev := range page.Events {
		if ev.StepID != "plan" {
			continue
		}
		output = output || ev.Kind == "activity.output" && strings.Contains(string(ev.Payload), "기획을 정리하고 있습니다")
		final = final || ev.Kind == "provider.completed" && strings.Contains(string(ev.Payload), "기획 완료")
	}
	if !output || !final {
		t.Fatalf("output=%v final=%v: %+v", output, final, page.Events)
	}
}
