package engine_test

import (
	"errors"
	"strings"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

func kinds(msgs []engine.MessageView) map[string]int {
	out := map[string]int{}
	for _, m := range msgs {
		out[m.Kind]++
	}
	return out
}

// A step's prompt carries the handoff note of the input versions it was
// given, with a reference to the source; the review step gets a review
// request, and the reviewer's verdict is handed on to QA.
func TestHandoffNotesReachDownstreamPrompts(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.review(p.ID, runID, "review", engine.ReviewPass, "에러 처리를 잘 했습니다")
	run := h.waitRun(p.ID, runID, engine.RunSucceeded)

	design := h.started("design")[0].Prompt
	if !strings.Contains(design, "전달 메모 · 기획") || !strings.Contains(design, "자동 완료") || !strings.Contains(design, "plan.spec v1") {
		t.Fatalf("design prompt lacks the plan handoff:\n%s", design)
	}
	qa := h.started("qa")[0].Prompt
	if !strings.Contains(qa, "사람(나)의 리뷰: 통과") || !strings.Contains(qa, "에러 처리를 잘 했습니다") {
		t.Fatalf("qa prompt lacks the review handoff:\n%s", qa)
	}

	msgs, err := h.eng.RunMessages(h.ctx, p.ID, runID)
	if err != nil {
		t.Fatal(err)
	}
	k := kinds(msgs)
	if k["handoff"] == 0 || k["review_request"] != 1 {
		t.Fatalf("message kinds = %v", k)
	}
	for _, m := range msgs {
		if m.Kind == "review_request" {
			if m.Recipient != p.Assignments["a-owner"] || len(m.Refs.Artifacts) != 1 || m.Refs.Artifacts[0].ID != run.Step("integrate").Artifacts[0].ID {
				t.Fatalf("review request = %+v", m)
			}
		}
		if m.Kind == "handoff" && len(m.Refs.Artifacts) == 0 {
			t.Fatalf("handoff without a source reference: %+v", m)
		}
	}
}

// A reworked step remembers what it already asked and was told, and gets
// the reason for the rework as an instruction.
func TestReworkedStepRemembersAnswers(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.setModel(p, "a-planner", "question")
	runID := h.start(p)
	h.waitStep(p.ID, runID, "plan", engine.StWaitingInput)
	items, err := h.eng.Inbox(h.ctx)
	if err != nil || len(items) != 1 || items[0].Kind != engine.InboxQuestion {
		t.Fatalf("inbox = %+v %v", items, err)
	}
	if _, err := h.eng.AnswerQuestion(h.ctx, p.ID, items[0].MessageID, "네, 웹만 지원합니다"); err != nil {
		t.Fatal(err)
	}
	s := h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)
	if _, err := h.eng.DecideApproval(h.ctx, p.ID, s.ApprovalID, s.Generation, engine.ApprovalRejected, "범위를 더 좁혀 주세요", []string{"plan"}); err != nil {
		t.Fatal(err)
	}
	h.waitFor(p.ID, runID, "second plan attempt", func(engine.RunDetail) bool { return len(h.started("plan")) == 2 })
	again := h.started("plan")[1]
	if !strings.Contains(again.Prompt, "대상 플랫폼은 웹만인가요?") || !strings.Contains(again.Prompt, "네, 웹만 지원합니다") {
		t.Fatalf("rework prompt lacks the earlier answer:\n%s", again.Prompt)
	}
	if !strings.Contains(again.Instructions, "범위를 더 좁혀 주세요") {
		t.Fatalf("rework reason missing from instructions:\n%s", again.Instructions)
	}
}

// A person's note to a step reaches the step's next attempt; notes are
// checked against the project and the step.
func TestPostNoteReachesNextAttempt(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	other := testenv.ServiceDev(t, h.db, "부동산")
	runID := h.start(p)
	h.waitStep(p.ID, runID, "approve", engine.StWaitingApproval)

	if _, err := h.eng.PostNote(h.ctx, other.ID, runID, "design", engine.MsgDecision, "x"); !errors.Is(err, storage.ErrNotInProject) {
		t.Fatalf("cross-project note: %v", err)
	}
	for _, bad := range []struct{ step, kind, body string }{
		{"design", "question", "x"}, {"design", engine.MsgDecision, " "}, {"nope", engine.MsgDecision, "x"},
	} {
		if _, err := h.eng.PostNote(h.ctx, p.ID, runID, bad.step, bad.kind, bad.body); !errors.Is(err, engine.ErrInvalid) {
			t.Fatalf("note %+v accepted: %v", bad, err)
		}
	}
	if _, err := h.eng.PostNote(h.ctx, p.ID, runID, "design", engine.MsgDecision, "API는 REST로 통일합니다"); err != nil {
		t.Fatal(err)
	}
	h.approve(p.ID, runID, "approve")
	h.waitFor(p.ID, runID, "design started", func(engine.RunDetail) bool { return len(h.started("design")) == 1 })
	if prompt := h.started("design")[0].Prompt; !strings.Contains(prompt, "사용자 결정") || !strings.Contains(prompt, "API는 REST로 통일합니다") {
		t.Fatalf("design prompt lacks the note:\n%s", prompt)
	}
	if prompt := h.started("backend"); len(prompt) > 0 && strings.Contains(prompt[0].Prompt, "REST로 통일") {
		t.Fatal("a note for design leaked into another step")
	}
}
