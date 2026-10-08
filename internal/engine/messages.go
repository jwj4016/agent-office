package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// Message kinds (spec §8). The table's CHECK list is the authority.
const (
	MsgQuestion      = "question"
	MsgAnswer        = "answer"
	MsgReviewRequest = "review_request"
	MsgProposal      = "proposal"
	MsgDecision      = "decision"
	MsgHandoff       = "handoff"
	MsgEscalation    = "escalation"
)

// SenderEngine marks messages the app writes itself (handoffs, review
// requests, escalations).
const SenderEngine = "engine"

// stepRecipient addresses a message to a step of the run rather than to
// an actor, so it reaches whichever attempt of the step runs next.
func stepRecipient(stepID string) string { return "step:" + stepID }

// ArtifactRef points a message at the exact result version it is about.
type ArtifactRef struct {
	ID        string `json:"id"`
	StepID    string `json:"stepId,omitempty"`
	OutputKey string `json:"outputKey"`
	Version   int    `json:"version"`
	Hash      string `json:"hash"`
}

// MessageRefs is stored in messages.artifact_refs. Fields are optional;
// each kind uses the ones that apply.
type MessageRefs struct {
	Artifacts []ArtifactRef `json:"artifacts,omitempty"`
	// Generation is the target step generation a rework decision is for.
	Generation int `json:"generation,omitempty"`
	// From is the step that sent a rework decision or handoff.
	From string `json:"from,omitempty"`
	// Note marks a person's note to a step.
	Note bool `json:"note,omitempty"`
	// Round and Agree describe one meeting opinion.
	Round int   `json:"round,omitempty"`
	Agree *bool `json:"agree,omitempty"`
	// Reason says why an escalation was raised.
	Reason string `json:"reason,omitempty"`
}

type newMessage struct {
	RunID, AttemptID, Sender, Recipient, Kind, Body, ReplyTo string
	Refs                                                     MessageRefs
}

// addMessage stores a message and its event in the caller's change.
func addMessage(ctx context.Context, c *storage.Change, projectID string, m newMessage) (string, error) {
	id := storage.NewID("msg")
	refs, _ := json.Marshal(m.Refs)
	if _, err := c.Tx.ExecContext(ctx, `INSERT INTO messages (id, project_id, run_id, step_attempt_id, sender, recipient, kind, body, artifact_refs, reply_to, created_at)
		VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, ?, ?, NULLIF(?, ''), ?)`,
		id, projectID, m.RunID, m.AttemptID, m.Sender, m.Recipient, m.Kind, m.Body, string(refs), m.ReplyTo, storage.Now()); err != nil {
		return "", err
	}
	return id, c.Emit(projectID, m.RunID, m.AttemptID, "message."+m.Kind, map[string]any{
		"messageId": id, "kind": m.Kind, "sender": m.Sender, "recipient": m.Recipient, "body": clip(m.Body, 500),
	})
}

// postHandoffs tells every step that takes this step's results as input
// what was handed over: the exact versions and the producer's own note
// (for an AI, its final message). Downstream prompts quote the note next
// to the pinned input, so a summary always carries its source reference.
func postHandoffs(ctx context.Context, c *storage.Change, st *runState, a attemptRow, arts []ArtifactRef, note string) error {
	if len(arts) == 0 {
		return nil
	}
	note = strings.TrimSpace(note)
	if note == "" {
		var keys []string
		for _, r := range arts {
			keys = append(keys, fmt.Sprintf("%s v%d", r.OutputKey, r.Version))
		}
		note = "결과를 전달합니다: " + strings.Join(keys, ", ")
	}
	sender := a.AssignmentID
	if sender == "" {
		sender = SenderEngine
	}
	for _, id := range st.graph.Order() {
		n := st.graph.Node(id)
		var mine []ArtifactRef
		for _, in := range n.Inputs {
			if in.FromStep != a.StepID {
				continue
			}
			for _, r := range arts {
				if r.OutputKey == in.OutputKey {
					mine = append(mine, r)
				}
			}
		}
		if len(mine) == 0 {
			continue
		}
		if _, err := addMessage(ctx, c, st.run.ProjectID, newMessage{
			RunID: st.run.ID, AttemptID: a.ID, Sender: sender, Recipient: stepRecipient(id), Kind: MsgHandoff,
			Body: clip(note, handoffLimit), Refs: MessageRefs{Artifacts: mine, From: a.StepID},
		}); err != nil {
			return err
		}
	}
	return nil
}

// postReviewRequest asks a review step's assignee to look at the pinned
// input versions.
func postReviewRequest(ctx context.Context, c *storage.Change, st *runState, n *domain.Node, a attemptRow, manifest []ManifestEntry) error {
	var refs []ArtifactRef
	var lines []string
	for _, m := range manifest {
		if m.ArtifactID == "" {
			continue
		}
		var version int
		c.Tx.QueryRowContext(ctx, `SELECT version FROM artifacts WHERE id = ?`, m.ArtifactID).Scan(&version)
		refs = append(refs, ArtifactRef{ID: m.ArtifactID, StepID: m.FromStep, OutputKey: m.OutputKey, Version: version, Hash: m.Hash})
		lines = append(lines, fmt.Sprintf("- %s (%s.%s v%d)", m.Name, m.FromStep, m.OutputKey, version))
	}
	body := "검토를 요청합니다: " + n.Title
	if len(lines) > 0 {
		body += "\n" + strings.Join(lines, "\n")
	}
	_, err := addMessage(ctx, c, st.run.ProjectID, newMessage{
		RunID: st.run.ID, AttemptID: a.ID, Sender: SenderEngine, Recipient: n.AssignmentID, Kind: MsgReviewRequest,
		Body: body, Refs: MessageRefs{Artifacts: refs},
	})
	return err
}

// Context limits keep prompts bounded: a step sees what concerns it,
// never the whole conversation (spec §8).
const (
	handoffLimit      = 2000
	contextEntryLimit = 1500
	contextTotalLimit = 8000
	contextMaxQA      = 10
)

// contextNote is one entry of a prompt's "related conversation" section.
type contextNote struct {
	Label string // e.g. "전달 메모 · 기획"
	Ref   string // where the original is (message id, artifact version)
	Text  string
}

// stepContext gathers what an attempt of step n should know besides its
// inputs: handoff notes for the pinned input versions, questions this
// step already asked in this run and their answers, and notes a person
// left for the step. Rework feedback is an instruction layer instead.
func stepContext(ctx context.Context, q querier, st *runState, n *domain.Node, manifest []ManifestEntry) ([]contextNote, error) {
	var out []contextNote
	pinned := map[string]ManifestEntry{}
	for _, m := range manifest {
		if m.ArtifactID != "" {
			pinned[m.ArtifactID] = m
		}
	}
	rows, err := q.QueryContext(ctx, `SELECT id, kind, sender, body, artifact_refs FROM messages
		WHERE run_id = ? AND recipient = ? AND kind IN ('handoff', 'proposal', 'decision') ORDER BY created_at`, st.run.ID, stepRecipient(n.ID))
	if err != nil {
		return nil, err
	}
	var notes []contextNote
	for rows.Next() {
		var id, kind, sender, body, refsJSON string
		if err := rows.Scan(&id, &kind, &sender, &body, &refsJSON); err != nil {
			rows.Close()
			return nil, err
		}
		var refs MessageRefs
		json.Unmarshal([]byte(refsJSON), &refs)
		switch {
		case kind == MsgHandoff:
			for _, r := range refs.Artifacts {
				if m, ok := pinned[r.ID]; ok {
					from := refs.From
					if fn := st.graph.Node(from); fn != nil {
						from = fn.Title
					}
					out = append(out, contextNote{Label: "전달 메모 · " + from, Ref: fmt.Sprintf("입력 %q(%s.%s v%d), 메시지 %s", m.Name, m.FromStep, m.OutputKey, r.Version, id), Text: body})
					break
				}
			}
		case refs.Note && sender == domain.LocalOwner:
			label := "사용자 제안"
			if kind == MsgDecision {
				label = "사용자 결정"
			}
			notes = append(notes, contextNote{Label: label, Ref: "메시지 " + id, Text: body})
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	qa, err := previousAnswers(ctx, q, st.run.ID, n.ID)
	if err != nil {
		return nil, err
	}
	out = append(out, qa...)
	return append(out, notes...), nil
}

// previousAnswers lists questions earlier attempts of the step asked in
// this run, with their answers, so a retry or rework does not ask again.
func previousAnswers(ctx context.Context, q querier, runID, stepID string) ([]contextNote, error) {
	rows, err := q.QueryContext(ctx, `SELECT m.id, m.body, a.body, a.sender FROM messages m
		JOIN messages a ON a.reply_to = m.id AND a.kind = 'answer'
		JOIN step_attempts s ON s.id = m.step_attempt_id
		WHERE m.run_id = ? AND m.kind = 'question' AND s.step_id = ? ORDER BY m.created_at DESC LIMIT ?`, runID, stepID, contextMaxQA)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []contextNote
	for rows.Next() {
		var id, question, answer, by string
		if err := rows.Scan(&id, &question, &answer, &by); err != nil {
			return nil, err
		}
		who := "사용자"
		if by != domain.LocalOwner {
			who = by
		}
		out = append([]contextNote{{Label: "이전에 받은 답변 · " + who, Ref: "질문 " + id, Text: "질문: " + question + "\n답변: " + answer}}, out...)
	}
	return out, rows.Err()
}

// writeContext renders notes within the size limits, saying how many
// were left out and where to find them.
func writeContext(b *strings.Builder, notes []contextNote) {
	if len(notes) == 0 {
		return
	}
	b.WriteString("## 관련 대화·결정\n\n")
	used := 0
	for i, nt := range notes {
		text := clip(strings.TrimSpace(nt.Text), contextEntryLimit)
		if used+utf8.RuneCountInString(text) > contextTotalLimit {
			fmt.Fprintf(b, "- … 이하 %d건은 길이 제한으로 생략했습니다. 앱의 실행 기록에서 원문을 볼 수 있습니다.\n", len(notes)-i)
			break
		}
		used += utf8.RuneCountInString(text)
		fmt.Fprintf(b, "- [%s] (%s)\n", nt.Label, nt.Ref)
		for _, line := range strings.Split(text, "\n") {
			fmt.Fprintf(b, "  > %s\n", line)
		}
	}
	b.WriteString("\n")
}

// clip shortens s to at most n runes, marking the cut.
func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + " …(생략)"
}

// MessageView is one message of a run's conversation record.
type MessageView struct {
	ID            string      `json:"id"`
	Kind          string      `json:"kind"`
	Sender        string      `json:"sender"`
	SenderName    string      `json:"senderName"`
	Recipient     string      `json:"recipient"`
	RecipientName string      `json:"recipientName"`
	StepID        string      `json:"stepId"`
	Body          string      `json:"body"`
	ReplyTo       string      `json:"replyTo,omitempty"`
	Refs          MessageRefs `json:"refs"`
	CreatedAt     string      `json:"createdAt"`
}

// actorName turns a sender or recipient into a display name.
func actorName(st *runState, who string) string {
	switch {
	case who == domain.LocalOwner:
		return "나"
	case who == SenderEngine:
		return "앱"
	case strings.HasPrefix(who, "step:"):
		if n := st.graph.Node(strings.TrimPrefix(who, "step:")); n != nil {
			return "업무 " + n.Title
		}
	}
	if a, ok := st.version.Assignments[who]; ok {
		return a.DisplayName
	}
	return who
}

// RunMessages lists a run's conversation: questions, answers, handoffs,
// review requests, proposals, decisions and escalations, oldest first.
func (e *Engine) RunMessages(ctx context.Context, projectID, runID string) ([]MessageView, error) {
	if err := e.runInProject(ctx, projectID, runID); err != nil {
		return nil, err
	}
	q := e.db.Read()
	st, err := loadState(ctx, q, runID)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT m.id, m.kind, m.sender, m.recipient, COALESCE(s.step_id, ''), m.body, COALESCE(m.reply_to, ''), m.artifact_refs, m.created_at
		FROM messages m LEFT JOIN step_attempts s ON s.id = m.step_attempt_id WHERE m.run_id = ? ORDER BY m.created_at, m.rowid`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MessageView{}
	for rows.Next() {
		var m MessageView
		var refs string
		if err := rows.Scan(&m.ID, &m.Kind, &m.Sender, &m.Recipient, &m.StepID, &m.Body, &m.ReplyTo, &refs, &m.CreatedAt); err != nil {
			return nil, err
		}
		json.Unmarshal([]byte(refs), &m.Refs)
		m.SenderName, m.RecipientName = actorName(st, m.Sender), actorName(st, m.Recipient)
		out = append(out, m)
	}
	return out, rows.Err()
}

// PostNote leaves a person's proposal or decision for a step. It is part
// of the context of the step's next attempt; an attempt already running
// keeps the prompt it started with.
func (e *Engine) PostNote(ctx context.Context, projectID, runID, stepID, kind, body string) (string, error) {
	if err := e.runInProject(ctx, projectID, runID); err != nil {
		return "", err
	}
	if kind != MsgProposal && kind != MsgDecision {
		return "", fmt.Errorf("%w: 메모 종류는 proposal 또는 decision입니다", ErrInvalid)
	}
	if strings.TrimSpace(body) == "" {
		return "", fmt.Errorf("%w: 내용이 비어 있습니다", ErrInvalid)
	}
	var id string
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, runID)
		if err != nil {
			return err
		}
		if st.run.Status == RunCancelled || st.run.Status == RunSucceeded {
			return fmt.Errorf("%w: 끝난 실행입니다", ErrInvalid)
		}
		if n := st.graph.Node(stepID); n == nil || n.Kind == domain.KindCondition || n.Kind == domain.KindJoin {
			return fmt.Errorf("%w: 메모를 받을 수 있는 업무가 아닙니다", ErrInvalid)
		}
		id, err = addMessage(ctx, c, projectID, newMessage{
			RunID: runID, Sender: domain.LocalOwner, Recipient: stepRecipient(stepID), Kind: kind, Body: body, Refs: MessageRefs{Note: true},
		})
		return err
	})
	return id, err
}
