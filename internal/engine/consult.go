package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"agent-office/internal/domain"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// questionTo matches a question addressed to another assignee:
// "@<assignmentId> question".
var questionTo = regexp.MustCompile(`^\s*@([A-Za-z0-9][A-Za-z0-9_-]*)\s+([\s\S]+)$`)

// peers lists the AI assignees of the run's version a step may ask,
// besides its own assignee.
func peers(st *runState, self string) []domain.AssignmentSnapshot {
	var out []domain.AssignmentSnapshot
	for id, a := range st.version.Assignments {
		if id != self && a.ActorKind == domain.ActorAI {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].DisplayName < out[j].DisplayName })
	return out
}

// questionTarget returns the AI assignee a question is addressed to and
// the question without the address. Anything else is for the person.
func questionTarget(st *runState, self, detail string) (string, string) {
	m := questionTo.FindStringSubmatch(detail)
	if m == nil {
		return "", detail
	}
	a, ok := st.version.Assignments[m[1]]
	if !ok || a.ActorKind != domain.ActorAI || m[1] == self {
		return "", detail
	}
	return m[1], strings.TrimSpace(m[2])
}

// consultBlocked says why a question from asker to target must go to the
// person instead (T18): too many questions from this attempt, too many
// between the same two assignees (a loop), no usable connection, or the
// budget. It is checked before the new question is stored.
func (e *Engine) consultBlocked(ctx context.Context, q querier, st *runState, attemptID, asker, target string) string {
	var mine, pair int
	q.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE step_attempt_id = ? AND kind = 'question' AND recipient <> 'local-owner'`, attemptID).Scan(&mine)
	if mine >= e.cfg.MaxConsults {
		return fmt.Sprintf("이 업무가 다른 AI에게 물을 수 있는 횟수(%d회)를 넘었습니다", e.cfg.MaxConsults)
	}
	q.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE run_id = ? AND kind = 'question'
		AND ((sender = ? AND recipient = ?) OR (sender = ? AND recipient = ?))`, st.run.ID, asker, target, target, asker).Scan(&pair)
	if pair >= e.cfg.MaxPairQuestions {
		return fmt.Sprintf("두 담당자 사이의 질문이 %d회를 넘어 반복 협의를 멈췄습니다", e.cfg.MaxPairQuestions)
	}
	if _, ok := e.connectionReady(ctx, st.version.Assignments[target].ConnectionID); !ok {
		return "답할 AI의 연결이 준비되지 않았습니다"
	}
	if u, err := projectUsage(ctx, q, st.run.ProjectID); err == nil && u.HoldReason != "" {
		return u.HoldReason
	}
	return ""
}

// startConsult has target answer the question in a side session. Answers
// queue one at a time; if the target cannot answer in time the question
// goes to the person.
func (e *Engine) startConsult(a *activeAttempt, n *domain.Node, msgID, target, question string) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		select {
		case e.consultSlot <- struct{}{}:
		case <-a.ctx.Done():
			return
		}
		defer func() { <-e.consultSlot }()
		answer, err := e.askAI(a, n, msgID, target, question)
		if a.ctx.Err() != nil {
			return // the asker stopped; nobody waits for the answer
		}
		ctx := context.Background()
		if err != nil {
			if err := e.escalateQuestion(ctx, a, msgID, target, err.Error()); err != nil {
				e.cfg.Logf("engine: escalate %s: %v", msgID, err)
			}
			return
		}
		if _, err := e.deliverAnswer(ctx, msgID, target, answer); err != nil && !errors.Is(err, ErrStale) {
			e.cfg.Logf("engine: deliver answer %s: %v", msgID, err)
		}
	}()
}

// askAI runs target once, read-only, to answer question.
func (e *Engine) askAI(a *activeAttempt, n *domain.Node, msgID, target, question string) (string, error) {
	ctx := context.Background()
	st, err := loadState(ctx, e.db.Read(), a.runID)
	if err != nil {
		return "", err
	}
	snap := st.version.Assignments[target]
	conn, ok := e.connectionReady(ctx, snap.ConnectionID)
	if !ok {
		return "", errors.New("답할 AI의 연결이 준비되지 않았습니다")
	}
	if u, err := projectUsage(ctx, e.db.Read(), st.run.ProjectID); err == nil && u.HoldReason != "" {
		return "", errors.New(u.HoldReason)
	}
	prov, err := e.cfg.Providers(conn)
	if err != nil {
		return "", err
	}
	work, err := e.workspaceDir(st)
	if err != nil {
		return "", err
	}
	asker := st.version.Assignments[n.AssignmentID].DisplayName
	layers := append(append([]domain.InstructionLayer(nil), snap.Instructions...), domain.InstructionLayer{
		Source: "task", Name: "다른 담당자의 질문에 답하기",
		Text: "같은 서비스의 다른 담당자가 작업 중에 질문했습니다. 당신이 맡은 역할과 이 실행에서 당신이 만든 결과를 근거로 짧고 구체적으로 답하세요. 파일을 만들거나 고치지 말고, 모르면 모른다고 답하세요.",
	})
	req := providers.StartRequest{
		ProjectID: a.projectID, RunID: a.runID, StepAttemptID: a.id, StepID: n.ID, Generation: a.generation,
		Instructions: domain.ComposeInstructions(layers),
		Prompt:       consultPrompt(ctx, e.db.Read(), e.cfg.DataDir, st, target, asker, n, question),
		Workspace:    work,
		Model:        snap.Model,
		Policy:       providers.Policy{Sandbox: "read-only", AskApproval: true},
		Limits:       providers.Limits{Timeout: e.cfg.ConsultTimeout},
	}
	tctx, cancel := context.WithTimeout(a.ctx, e.cfg.ConsultTimeout)
	defer cancel()
	sess, err := prov.Start(tctx, req)
	if err != nil {
		return "", err
	}
	p, err := e.runSide(tctx, a, sess, "consult:"+msgID)
	if err != nil {
		return "", err
	}
	if p.Status != providers.StatusSucceeded {
		return "", fmt.Errorf("답하지 못했습니다 (%s %s)", p.Status, p.Error)
	}
	text := strings.TrimSpace(p.Text)
	if text == "" {
		return "", errors.New("빈 답변이 왔습니다")
	}
	return text, nil
}

// consultPrompt shows the question and the target's own results in this
// run, which are what it can answer from.
func consultPrompt(ctx context.Context, q querier, dataDir string, st *runState, target, asker string, n *domain.Node, question string) string {
	var b strings.Builder
	b.WriteString("# 다른 담당자의 질문에 답하기\n\n")
	if g := st.version.Policy.Goal; g != "" {
		fmt.Fprintf(&b, "서비스 목표: %s\n\n", g)
	}
	fmt.Fprintf(&b, "질문한 담당자: %s (업무: %s)\n\n## 질문\n\n%s\n\n", asker, n.Title, question)
	var own []string
	for _, id := range st.graph.Order() {
		sn := st.graph.Node(id)
		a, ok := st.attempts[id]
		if sn.AssignmentID != target || !ok || a.Status != StSucceeded {
			continue
		}
		rows, err := q.QueryContext(ctx, `SELECT output_key, version, path FROM artifacts WHERE step_attempt_id = ? AND validity = 'valid' ORDER BY output_key`, a.ID)
		if err != nil {
			continue
		}
		for rows.Next() {
			var key, path string
			var version int
			rows.Scan(&key, &version, &path)
			full := filepath.Join(dataDir, filepath.FromSlash(path))
			line := fmt.Sprintf("- %s · %s v%d: %s", sn.Title, key, version, full)
			if data, err := os.ReadFile(full); err == nil && len(data) <= inlineLimit && utf8.Valid(data) {
				line += "\n\n```\n" + strings.TrimRight(string(data), "\n") + "\n```\n"
			}
			own = append(own, line)
		}
		rows.Close()
	}
	if len(own) > 0 {
		b.WriteString("## 이 실행에서 당신이 만든 결과\n\n" + strings.Join(own, "\n") + "\n\n")
	}
	b.WriteString("## 답변 방법\n\n- 마지막 응답에 답만 쓰세요. 이 답이 그대로 질문한 담당자에게 전달됩니다.\n")
	return b.String()
}

// sideAnswer is what a side session hears when it asks something itself:
// side sessions never wait on anyone, so questions cannot loop (T18).
const sideAnswer = "지금은 질문할 수 없습니다. 주어진 자료와 아는 범위에서 진행하고, 확실하지 않은 점은 결과에 적어 주세요."

// runSide drives a session nobody interacts with (a meeting turn, an
// answer to another AI): tool requests are declined, questions get a
// fixed reply, usage is recorded under the attempt with phase as key.
func (e *Engine) runSide(ctx context.Context, a *activeAttempt, s providers.Session, phase string) (providers.CompletedPayload, error) {
	for {
		select {
		case ev, ok := <-s.Events():
			if !ok {
				return providers.CompletedPayload{Status: providers.StatusFailed, Error: "세션이 결과 없이 끝났습니다"}, nil
			}
			switch ev.Kind {
			case providers.KindCompleted:
				var p providers.CompletedPayload
				json.Unmarshal(ev.Payload, &p)
				return p, nil
			case providers.KindApprovalRequest:
				var p providers.RequestPayload
				json.Unmarshal(ev.Payload, &p)
				s.Respond(ctx, providers.Response{RequestID: p.RequestID, Decision: providers.DecisionDecline})
				e.sideEvent(a, "side.tool_declined", map[string]string{"phase": phase, "action": p.Action, "detail": clip(p.Detail, 300)})
			case providers.KindQuestion:
				var p providers.RequestPayload
				json.Unmarshal(ev.Payload, &p)
				s.Respond(ctx, providers.Response{RequestID: p.RequestID, Answer: sideAnswer})
			case providers.KindUsage:
				var p map[string]any
				json.Unmarshal(ev.Payload, &p)
				if p != nil {
					p["phase"] = phase
					e.sideEvent(a, "provider.usage", p)
				}
			case providers.KindMessageDelta:
				if e.cfg.Ephemeral != nil {
					e.cfg.Ephemeral(a.projectID, ev)
				}
			}
		case <-ctx.Done():
			s.Cancel(context.Background())
			drainUntil(s, sideDrain)
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return providers.CompletedPayload{}, errors.New("응답 시간 제한을 넘었습니다")
			}
			return providers.CompletedPayload{}, ctx.Err()
		}
	}
}

// sideDrain bounds the wait for a cancelled side session to end.
const sideDrain = 10 * time.Second

// drainUntil waits for a cancelled session to finish, so its process is
// gone before the caller moves on.
func drainUntil(s providers.Session, d time.Duration) {
	timeout := time.After(d)
	for {
		select {
		case _, ok := <-s.Events():
			if !ok {
				return
			}
		case <-timeout:
			return
		}
	}
}

func (e *Engine) sideEvent(a *activeAttempt, kind string, payload any) {
	_, err := e.db.Change(context.Background(), func(c *storage.Change) error {
		return c.Emit(a.projectID, a.runID, a.id, kind, payload)
	})
	if err != nil {
		e.cfg.Logf("engine: attempt %s %s: %v", a.id, kind, err)
	}
}

// escalateQuestion passes a question an AI could not answer to the
// person: the asking attempt now waits for the person's answer.
func (e *Engine) escalateQuestion(ctx context.Context, a *activeAttempt, msgID, target, reason string) error {
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		var answered int
		c.Tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM messages WHERE reply_to = ? AND kind IN ('answer', 'escalation')`, msgID).Scan(&answered)
		if answered > 0 {
			return nil
		}
		st, err := loadState(ctx, c.Tx, a.runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, a.id)
		if err != nil {
			return err
		}
		if !st.isCurrent(row) || !isLive(row.Status) {
			return nil
		}
		if err := escalate(ctx, c, st, row, msgID, actorName(st, target), reason); err != nil {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	return err
}

// escalate records that a question goes to the person and makes the
// attempt wait for input.
func escalate(ctx context.Context, c *storage.Change, st *runState, row attemptRow, msgID, targetName, reason string) error {
	if _, err := addMessage(ctx, c, st.run.ProjectID, newMessage{
		RunID: st.run.ID, AttemptID: row.ID, Sender: SenderEngine, Recipient: domain.LocalOwner, Kind: MsgEscalation, ReplyTo: msgID,
		Body: fmt.Sprintf("%s에게 보낸 질문을 사용자에게 넘깁니다: %s", targetName, reason), Refs: MessageRefs{Reason: reason},
	}); err != nil {
		return err
	}
	_, err := setAttemptStatus(ctx, c, st, row, StWaitingInput, "", StRunning, StWaitingApproval)
	return err
}

// writePeers tells an AI step whom it may ask and how.
func writePeers(b *strings.Builder, list []domain.AssignmentSnapshot, maxConsults int) {
	if len(list) == 0 {
		return
	}
	b.WriteString("## 다른 담당자에게 묻기\n\n")
	fmt.Fprintf(b, "다른 담당자의 판단이 꼭 필요하면 질문 기능에 `@담당자ID 질문` 형식으로 물으세요. 그 담당자 AI가 답합니다(이 업무에서 최대 %d회). 앱 사용자에게 묻는 질문에는 @를 붙이지 않습니다.\n\n", maxConsults)
	for _, a := range list {
		fmt.Fprintf(b, "- @%s: %s (%s)\n", a.ID, a.DisplayName, a.RoleName)
	}
	b.WriteString("\n")
}
