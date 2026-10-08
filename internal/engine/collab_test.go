package engine_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
)

func step(t *testing.T, s string) providers.Step {
	t.Helper()
	var st providers.Step
	if err := json.Unmarshal([]byte(s), &st); err != nil {
		t.Fatal(err)
	}
	return st
}

// ask returns a request step that asks a question with the given text.
func ask(t *testing.T, id, text string) providers.Step {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"request": "question", "payload": map[string]string{"requestId": id, "action": "question", "detail": text}})
	return step(t, string(b))
}

const codeChange = `{"baseCommit":"","changes":[{"path":"api.go","status":"added"}],"tests":{"command":"","passed":true}}`

func writeChange(t *testing.T) providers.Step {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"write": map[string]string{"key": "change", "content": codeChange}})
	return step(t, string(b))
}

func done(t *testing.T) providers.Step {
	return step(t, `{"complete": {"status": "succeeded", "text": "완료"}}`)
}

func messagesOf(t *testing.T, h *harness, projectID, runID string) []engine.MessageView {
	t.Helper()
	msgs, err := h.eng.RunMessages(h.ctx, projectID, runID)
	if err != nil {
		t.Fatal(err)
	}
	return msgs
}

func byKind(msgs []engine.MessageView, kind string) []engine.MessageView {
	var out []engine.MessageView
	for _, m := range msgs {
		if m.Kind == kind {
			out = append(out, m)
		}
	}
	return out
}

// An AI addresses a question to another AI assignee: that assignee
// answers in a read-only side session and the asker continues; the
// person is not involved.
func TestAIQuestionAnsweredByAnotherAI(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	arch := p.Assignments["a-architect"]
	h.prov.Scripts["ask-architect"] = []providers.Step{ask(t, "q1", "@"+arch+" 인증은 세션 방식인가요?"), writeChange(t), done(t)}
	h.setModel(p, "a-backend", "ask-architect")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "backend", engine.StSucceeded)

	if prompt := h.started("backend")[0].Prompt; !strings.Contains(prompt, "다른 담당자에게 묻기") || !strings.Contains(prompt, "@"+arch) {
		t.Fatalf("backend prompt does not explain how to ask:\n%s", prompt)
	}
	msgs := messagesOf(t, h, p.ID, runID)
	qs, as := byKind(msgs, "question"), byKind(msgs, "answer")
	if len(qs) != 1 || qs[0].Recipient != arch || len(as) != 1 || as[0].ReplyTo != qs[0].ID || as[0].Sender != arch {
		t.Fatalf("question/answer = %+v / %+v", qs, as)
	}
	if len(byKind(msgs, "escalation")) != 0 {
		t.Fatal("escalated although the AI answered")
	}
	// The answering session was read-only and saw the question.
	var consult *providers.StartRequest
	for _, r := range h.started("backend") {
		if strings.Contains(r.Prompt, "다른 담당자의 질문에 답하기") {
			r := r
			consult = &r
		}
	}
	if consult == nil || consult.Policy.Sandbox != "read-only" || len(consult.WritableDirs) != 0 || !strings.Contains(consult.Prompt, "인증은 세션 방식인가요?") {
		t.Fatalf("consult request = %+v", consult)
	}
}

// T18 (no response): an AI that does not answer in time hands the
// question to the person, who can answer it so the asker continues.
func TestAIQuestionEscalatesWhenUnanswered(t *testing.T) {
	h := newHarness(t, func(c *engine.Config) { c.ConsultTimeout = 300 * time.Millisecond })
	p := testenv.ServiceDev(t, h.db, "게임")
	arch := p.Assignments["a-architect"]
	h.prov.Scripts["ask-architect"] = []providers.Step{ask(t, "q1", "@"+arch+" 인증은 세션 방식인가요?"), writeChange(t), done(t)}
	h.setModel(p, "a-backend", "ask-architect")
	h.consultScenario = "slow" // the architect's answer sleeps past the limit
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "backend", engine.StWaitingInput)
	items, err := h.eng.Inbox(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	var q *engine.InboxItem
	for i := range items {
		if items[i].Kind == engine.InboxQuestion {
			q = &items[i]
		}
	}
	if q == nil || !strings.Contains(q.Escalation, "사용자에게 넘깁니다") || !strings.Contains(q.Escalation, "시간") {
		t.Fatalf("no escalated question in inbox: %+v", items)
	}
	if _, err := h.eng.AnswerQuestion(h.ctx, p.ID, q.MessageID, "네, 세션 방식입니다"); err != nil {
		t.Fatal(err)
	}
	h.waitStep(p.ID, runID, "backend", engine.StSucceeded)
	msgs := messagesOf(t, h, p.ID, runID)
	if as := byKind(msgs, "answer"); len(as) != 1 || as[0].Sender != "local-owner" {
		t.Fatalf("answers = %+v", as)
	}
}

// T18 (loop): questions between the same two assignees are capped; the
// one over the limit goes to the person instead of another AI turn.
func TestAIQuestionLoopIsCapped(t *testing.T) {
	h := newHarness(t, func(c *engine.Config) { c.MaxPairQuestions = 1 })
	p := testenv.ServiceDev(t, h.db, "게임")
	arch := p.Assignments["a-architect"]
	h.prov.Scripts["ask-twice"] = []providers.Step{
		ask(t, "q1", "@"+arch+" 첫 질문"), ask(t, "q2", "@"+arch+" 같은 질문을 또"), writeChange(t), done(t),
	}
	h.setModel(p, "a-backend", "ask-twice")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "backend", engine.StWaitingInput)
	items, _ := h.eng.Inbox(h.ctx)
	var q *engine.InboxItem
	for i := range items {
		if items[i].Kind == engine.InboxQuestion {
			q = &items[i]
		}
	}
	if q == nil || !strings.Contains(q.Detail, "같은 질문을 또") || !strings.Contains(q.Escalation, "반복 협의") {
		t.Fatalf("second question not passed to the person: %+v", items)
	}
	// The first question was answered by the AI, not the person.
	if as := byKind(messagesOf(t, h, p.ID, runID), "answer"); len(as) != 1 || as[0].Sender != arch {
		t.Fatalf("answers = %+v", as)
	}
	// A question meant for an AI cannot be answered by the person unless
	// it was passed on.
	first := byKind(messagesOf(t, h, p.ID, runID), "question")[0]
	if _, err := h.eng.AnswerQuestion(h.ctx, p.ID, first.ID, "x"); err == nil {
		t.Fatal("person answered a question meant for another AI")
	}
	if _, err := h.eng.AnswerQuestion(h.ctx, p.ID, q.MessageID, "그만 물어보고 진행하세요"); err != nil {
		t.Fatal(err)
	}
	h.waitStep(p.ID, runID, "backend", engine.StSucceeded)
}

// meetingFlow makes design a meeting of backend and frontend, decided by
// decider (a fixture assignment id).
func meetingFlow(t *testing.T, h *harness, decider string, rounds int) testenv.Project {
	t.Helper()
	draft := testenv.EditFixture(t, "workflows/service-dev.json", func(n map[string]map[string]any) {
		n["design"]["meeting"] = map[string]any{"participants": []any{"a-backend", "a-frontend"}, "maxRounds": rounds}
		n["design"]["assignmentId"] = decider
		n["design"]["instructions"] = "API 형식을 정한다"
	})
	return testenv.SeedDraft(t, h.db, "게임", draft, testenv.ServiceDevRoles, []string{"a-owner"})
}

func proposals(msgs []engine.MessageView) (rounds []int) {
	for _, m := range byKind(msgs, "proposal") {
		rounds = append(rounds, m.Refs.Round)
	}
	return
}

// Without agreement the meeting runs every round, then the AI decider
// writes the result with all opinions in its context.
func TestMeetingRunsRoundsThenAIDecides(t *testing.T) {
	h := newHarness(t)
	p := meetingFlow(t, h, "a-architect", 2)
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	d := h.waitStep(p.ID, runID, "design", engine.StSucceeded)
	if len(d.Artifacts) != 1 || d.Artifacts[0].OutputKey != "contract" {
		t.Fatalf("design artifacts = %+v", d.Artifacts)
	}
	msgs := messagesOf(t, h, p.ID, runID)
	if got := proposals(msgs); len(got) != 4 || got[0] != 1 || got[3] != 2 {
		t.Fatalf("proposal rounds = %v", got)
	}
	var decider, turn *providers.StartRequest
	for _, r := range h.started("design") {
		r := r
		if len(r.OutputSpec) == 1 && r.OutputSpec[0].Key == "contract" {
			decider = &r
		}
		if strings.Contains(r.Prompt, "# 회의:") && strings.Contains(r.Prompt, "2/2 라운드") {
			turn = &r
		}
	}
	if decider == nil || strings.Count(decider.Prompt, "회의 의견 ·") != 4 {
		t.Fatalf("decider prompt lacks the opinions: %+v", decider)
	}
	if turn == nil || turn.Policy.Sandbox != "read-only" || !strings.Contains(turn.Prompt, "지금까지 나온 의견") || !strings.Contains(turn.Prompt, "API 형식을 정한다") {
		t.Fatalf("round 2 turn = %+v", turn)
	}
	h.review(p.ID, runID, "review", engine.ReviewPass, "ok")
	h.waitRun(p.ID, runID, engine.RunSucceeded)
}

// Agreement ends the meeting early; agreement alone does not complete
// the step — the decider's outputs are still verified.
func TestMeetingStopsWhenAllAgree(t *testing.T) {
	h := newHarness(t)
	h.prov.Scripts["agree"] = []providers.Step{
		step(t, `{"write": {"key": "opinion", "content": "{\"opinion\": \"REST로 갑시다\", \"agree\": true}"}}`), done(t),
	}
	p := meetingFlow(t, h, "a-architect", 3)
	h.setModel(p, "a-backend", "agree")
	h.setModel(p, "a-frontend", "agree")
	h.setModel(p, "a-architect", "claims-done-no-complete")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "design", engine.StFailed)
	if got := proposals(messagesOf(t, h, p.ID, runID)); len(got) != 2 {
		t.Fatalf("proposal rounds = %v, want one round", got)
	}
}

// A person can decide a meeting: after the AI rounds the step waits for
// the person, whose task shows every opinion.
func TestMeetingDecidedByPerson(t *testing.T) {
	h := newHarness(t)
	p := meetingFlow(t, h, "a-owner", 1)
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	s := h.waitStep(p.ID, runID, "design", engine.StWaitingHuman)
	items, _ := h.eng.Inbox(h.ctx)
	var task *engine.InboxItem
	for i := range items {
		if items[i].StepID == "design" {
			task = &items[i]
		}
	}
	if task == nil || task.Kind != engine.InboxTask {
		t.Fatalf("no task for the person: %+v", items)
	}
	opinions := 0
	for _, c := range task.Context {
		if strings.HasPrefix(c.Label, "회의 의견") {
			opinions++
		}
	}
	if opinions != 2 {
		t.Fatalf("task context = %+v", task.Context)
	}
	if _, err := h.eng.SubmitHumanResult(h.ctx, p.ID, s.AttemptID, s.Generation, map[string]string{"contract": "# REST API"}); err != nil {
		t.Fatal(err)
	}
	h.waitStep(p.ID, runID, "design", engine.StSucceeded)
}

// Cancelling a run stops a meeting between turns; no more turns start.
func TestMeetingCancelled(t *testing.T) {
	h := newHarness(t)
	p := meetingFlow(t, h, "a-architect", 3)
	h.setModel(p, "a-backend", "slow")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "design", engine.StRunning)
	h.waitFor(p.ID, runID, "first turn", func(engine.RunDetail) bool { return len(h.started("design")) == 1 })
	if err := h.eng.CancelRun(h.ctx, p.ID, runID); err != nil {
		t.Fatal(err)
	}
	h.waitStep(p.ID, runID, "design", engine.StCancelled)
	time.Sleep(200 * time.Millisecond)
	if n := len(h.started("design")); n != 1 {
		t.Fatalf("%d turns started after cancel", n)
	}
}
