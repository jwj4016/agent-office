package engine_test

import (
	"errors"
	"strings"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/testenv"
)

func seedLegal(t *testing.T, h *harness) testenv.Project {
	return testenv.Seed(t, h.db, "법률", "workflows/legal-branch.json",
		map[string]string{"a-planner": "기획", "a-legal": "법률"}, []string{"a-owner"})
}

// T09: the branch not taken is skipped and the join does not wait for it.
func TestConditionSkipsUntakenBranch(t *testing.T) {
	h := newHarness(t)
	p := seedLegal(t, h)
	h.useScenario("scope", "scope-no-legal")
	runID := h.start(p)
	d := h.waitRun(p.ID, runID, engine.RunSucceeded)
	for step, want := range map[string]string{
		"scope": engine.StSucceeded, "route": engine.StSucceeded, "legal": engine.StSkipped,
		"legal_review": engine.StSkipped, "merge": engine.StSucceeded, "report": engine.StSucceeded,
	} {
		if got := d.Step(step).Status; got != want {
			t.Errorf("%s = %s, want %s", step, got, want)
		}
	}
	// The optional input from the skipped branch is recorded as absent.
	if !strings.Contains(string(d.Step("report").Inputs), `"name":"법률 의견"`) || strings.Count(string(d.Step("report").Inputs), "artifactId") != 1 {
		t.Fatalf("report inputs = %s", d.Step("report").Inputs)
	}
}

func TestConditionTakesBranch(t *testing.T) {
	h := newHarness(t)
	p := seedLegal(t, h)
	h.useScenario("scope", "scope-legal")
	runID := h.start(p)
	h.review(p.ID, runID, "legal_review", engine.ReviewPass, "확인")
	d := h.waitRun(p.ID, runID, engine.RunSucceeded)
	if d.Step("legal").Status != engine.StSucceeded || strings.Count(string(d.Step("report").Inputs), "artifactId") != 2 {
		t.Fatalf("branch not taken:\n%s\n%s", describe(d), d.Step("report").Inputs)
	}
}

// A path error fails the condition instead of guessing a branch.
func TestConditionPathErrorFails(t *testing.T) {
	h := newHarness(t)
	p := seedLegal(t, h)
	h.useScenario("scope", "scope-broken")
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "route", engine.StFailed)
	if !strings.Contains(s.Error, "분기 판단 실패") {
		t.Fatalf("error = %q", s.Error)
	}
	h.waitRun(p.ID, runID, engine.RunFailed)
}

// T12: a declined tool action is not run by the provider and the
// decision is recorded.
func TestToolApprovalDecline(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.useScenario("plan", "tool-approval-write")
	runID := h.start(p)
	h.waitStep(p.ID, runID, "plan", engine.StWaitingApproval)
	var aprID, detail string
	h.db.Read().QueryRow(`SELECT id, target_manifest FROM approvals WHERE run_id = ? AND kind = 'tool'`, runID).Scan(&aprID, &detail)
	if !strings.Contains(detail, "rm -rf build") {
		t.Fatalf("tool approval target = %s", detail)
	}
	if d := h.detail(p.ID, runID); d.Status != engine.RunWaiting {
		t.Fatalf("run = %s while waiting for tool approval", d.Status)
	}
	out, err := h.eng.DecideToolApproval(h.ctx, p.ID, aprID, false)
	if err != nil || out.Decision != engine.ApprovalRejected {
		t.Fatalf("decide: %+v %v", out, err)
	}
	if again, _ := h.eng.DecideToolApproval(h.ctx, p.ID, aprID, true); !again.Already || again.Decision != engine.ApprovalRejected {
		t.Fatalf("second click changed the decision: %+v", again)
	}
	h.waitStep(p.ID, runID, "plan", engine.StSucceeded)
	var resolved string
	h.db.Read().QueryRow(`SELECT payload FROM execution_events WHERE run_id = ? AND kind = 'provider.request_resolved'`, runID).Scan(&resolved)
	if !strings.Contains(resolved, `"decline"`) {
		t.Fatalf("provider did not receive the decline: %s", resolved)
	}
}

func TestQuestionAndAnswer(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.useScenario("plan", "question")
	runID := h.start(p)
	h.waitStep(p.ID, runID, "plan", engine.StWaitingInput)
	var msgID, body string
	h.db.Read().QueryRow(`SELECT id, body FROM messages WHERE run_id = ? AND kind = 'question'`, runID).Scan(&msgID, &body)
	if body != "대상 플랫폼은 웹만인가요?" {
		t.Fatalf("question = %q", body)
	}
	if _, err := h.eng.AnswerQuestion(h.ctx, p.ID, msgID, "네, 웹만입니다"); err != nil {
		t.Fatal(err)
	}
	// The scenario's final text becomes the single markdown output.
	s := h.waitStep(p.ID, runID, "plan", engine.StSucceeded)
	if len(s.Artifacts) != 1 {
		t.Fatalf("artifacts = %+v", s.Artifacts)
	}
	var answers int
	h.db.Read().QueryRow(`SELECT COUNT(*) FROM messages WHERE reply_to = ?`, msgID).Scan(&answers)
	if answers != 1 {
		t.Fatalf("answers = %d", answers)
	}
}

var humanTaskWorkflow = []byte(`{
  "schemaVersion": 1, "title": "사람이 조사",
  "nodes": [
    {"id": "research", "title": "자료 조사", "kind": "task", "assignmentId": "a-owner", "dependsOn": [],
     "outputs": [{"key": "notes", "type": "markdown"}, {"key": "data", "type": "json", "required": false,
       "schema": {"type": "object", "required": ["count"], "properties": {"count": {"type": "integer"}}}}]},
    {"id": "summary", "title": "요약", "kind": "task", "assignmentId": "a-planner", "dependsOn": ["research"],
     "inputs": [{"name": "조사", "fromStep": "research", "outputKey": "notes"}], "outputs": [{"key": "summary", "type": "markdown"}]}
  ]}`)

// A person does real work (not just approval); a submission that misses
// the completion criteria is refused without failing the step.
func TestHumanTaskSubmission(t *testing.T) {
	h := newHarness(t)
	p := testenv.SeedDraft(t, h.db, "조사", humanTaskWorkflow, map[string]string{"a-planner": "기획"}, []string{"a-owner"})
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "research", engine.StWaitingHuman)
	if d := h.detail(p.ID, runID); d.Status != engine.RunWaiting {
		t.Fatalf("run = %s", d.Status)
	}

	var verr *engine.VerificationError
	_, err := h.eng.SubmitHumanResult(h.ctx, p.ID, s.AttemptID, s.Generation, map[string]string{"data": `{"count": 3}`})
	if !errors.As(err, &verr) {
		t.Fatalf("missing required output accepted: %v", err)
	}
	_, err = h.eng.SubmitHumanResult(h.ctx, p.ID, s.AttemptID, s.Generation, map[string]string{"notes": "# 조사", "data": `{"count": "three"}`})
	if !errors.As(err, &verr) || !strings.Contains(verr.Problem, "schema") {
		t.Fatalf("schema violation accepted: %v", err)
	}
	if _, err := h.eng.SubmitHumanResult(h.ctx, p.ID, s.AttemptID, s.Generation, map[string]string{"nope": "x"}); !errors.Is(err, engine.ErrInvalid) {
		t.Fatalf("unknown output accepted: %v", err)
	}
	if h.detail(p.ID, runID).Step("research").Status != engine.StWaitingHuman {
		t.Fatal("refused submission changed the step")
	}
	out, err := h.eng.SubmitHumanResult(h.ctx, p.ID, s.AttemptID, s.Generation, map[string]string{"notes": "# 조사 결과", "data": `{"count": 3}`})
	if err != nil || out.Already {
		t.Fatalf("submit: %+v %v", out, err)
	}
	again, err := h.eng.SubmitHumanResult(h.ctx, p.ID, s.AttemptID, s.Generation, map[string]string{"notes": "# 다른 내용"})
	if err != nil || !again.Already {
		t.Fatalf("second submit: %+v %v", again, err)
	}
	d := h.waitRun(p.ID, runID, engine.RunSucceeded)
	if n := len(d.Step("research").Artifacts); n != 2 {
		t.Fatalf("research artifacts = %d", n)
	}
	if !strings.Contains(h.started("summary")[0].Prompt, "# 조사 결과") {
		t.Fatal("AI did not receive the person's notes")
	}
}
