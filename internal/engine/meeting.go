package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-office/internal/domain"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// A meeting task (spec §8) collects opinions from its participants for
// up to Rounds() rounds, stopping early once everyone agrees. Then the
// task's own assignee decides: an AI writes the outputs with every
// opinion in its context, a person gets the opinions in the task view.
// Agreement between models never replaces the completion check.

type opinion struct {
	Who   string // assignment id
	Round int
	Text  string
	Agree bool
}

// maxOpinionBytes bounds one opinion file.
const maxOpinionBytes = 64 << 10

// startMeeting creates the running attempt and runs the meeting in the
// background. Caller holds e.mu.
func (e *Engine) startMeeting(ctx context.Context, seen *runState, n *domain.Node) error {
	for _, pid := range n.Meeting.Participants {
		if _, err := e.providerFor(ctx, seen.version.Assignments[pid]); err != nil {
			return e.failNew(ctx, seen, n, fmt.Errorf("회의 참여자 %s: %w", actorName(seen, pid), err))
		}
	}
	if decider := seen.version.Assignments[n.AssignmentID]; decider.ActorKind == domain.ActorAI {
		if _, err := e.providerFor(ctx, decider); err != nil {
			return e.failNew(ctx, seen, n, err)
		}
	}
	gen := seen.run.Gens[n.ID].G
	id, err := e.createAttempt(ctx, seen, n, StRunning, func(c *storage.Change, st *runState, a attemptRow) error {
		if err := os.MkdirAll(filepath.Join(e.attemptDir(st.run.ProjectID, st.run.ID, a.ID), "out"), 0o700); err != nil {
			return err
		}
		return c.Emit(st.run.ProjectID, st.run.ID, a.ID, "meeting.started", map[string]any{
			"stepId": n.ID, "participants": n.Meeting.Participants, "rounds": n.Meeting.Rounds(), "decider": n.AssignmentID,
		})
	})
	if err != nil || id == "" {
		return err
	}
	a := e.newActive(id, seen, n.ID, gen)
	a.code = isCodeStep(n)
	e.active[id] = a
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.runMeeting(a, n)
	}()
	return nil
}

func (e *Engine) runMeeting(a *activeAttempt, n *domain.Node) {
	ctx := context.Background()
	handedOff := false
	defer func() {
		if r := recover(); r != nil {
			e.failAttempt(ctx, a, fmt.Sprintf("회의 진행 중 오류: %v", r))
		}
		if !handedOff {
			e.retire(a)
		}
	}()
	st, err := loadState(ctx, e.db.Read(), a.runID)
	if err != nil {
		e.cfg.Logf("engine: meeting %s: %v", a.id, err)
		return
	}
	row, _, err := loadAttempt(ctx, e.db.Read(), a.id)
	if err != nil {
		e.cfg.Logf("engine: meeting %s: %v", a.id, err)
		return
	}
	var manifest []ManifestEntry
	json.Unmarshal(row.InputManifest, &manifest)
	ws, err := e.prepareWorkspace(ctx, st, n, row)
	if err != nil {
		e.failAttempt(ctx, a, "작업 공간 준비 실패: "+err.Error())
		return
	}

	var said []opinion
	rounds := n.Meeting.Rounds()
	consensus, held := false, 0
	for r := 1; r <= rounds && !consensus; r++ {
		consensus = true
		held = r
		for _, pid := range n.Meeting.Participants {
			if a.isStopped() {
				e.endStopped(ctx, a)
				return
			}
			op, err := e.meetingTurn(a, st, n, manifest, ws.Dir, r, pid, said)
			if a.isStopped() {
				e.endStopped(ctx, a)
				return
			}
			if err != nil {
				e.failAttempt(ctx, a, fmt.Sprintf("회의 참여자 %s의 의견을 받지 못했습니다: %v", actorName(st, pid), err))
				return
			}
			if !e.recordOpinion(ctx, a, n, op) {
				e.endStopped(ctx, a)
				return
			}
			said = append(said, op)
			consensus = consensus && op.Agree
		}
	}
	e.sideEvent(a, "meeting.finished", map[string]any{"stepId": n.ID, "consensus": consensus, "rounds": held})

	decider := st.version.Assignments[n.AssignmentID]
	if decider.ActorKind == domain.ActorHuman {
		e.meetingToHuman(ctx, a)
		return
	}
	prov, err := e.providerFor(ctx, decider)
	if err != nil {
		e.failAttempt(ctx, a, err.Error())
		return
	}
	st, err = loadState(ctx, e.db.Read(), a.runID)
	if err != nil {
		e.cfg.Logf("engine: meeting %s: %v", a.id, err)
		return
	}
	row, _, _ = loadAttempt(ctx, e.db.Read(), a.id)
	if !st.isCurrent(row) {
		e.endStopped(ctx, a)
		return
	}
	var askable []domain.AssignmentSnapshot
	if prov.Capabilities().Question {
		askable = peers(st, n.AssignmentID)
	}
	req, err := e.aiRequest(ctx, e.db.Read(), st, n, decider, row, ws, askable)
	if err != nil {
		e.failAttempt(ctx, a, err.Error())
		return
	}
	session, err := prov.Start(e.ctx, req)
	if err != nil {
		e.failStart(ctx, a.runID, n.ID, err)
		return
	}
	a.setSession(session) // cancels it at once if the attempt was stopped
	handedOff = true
	e.pump(a, n, session)
}

// meetingTurn asks one participant for its opinion in a read-only side
// session.
func (e *Engine) meetingTurn(a *activeAttempt, st *runState, n *domain.Node, manifest []ManifestEntry, work string, round int, pid string, said []opinion) (opinion, error) {
	snap := st.version.Assignments[pid]
	prov, err := e.providerFor(context.Background(), snap)
	if err != nil {
		return opinion{}, err
	}
	dir := filepath.Join(e.attemptDir(a.projectID, a.runID, a.id), "meeting", fmt.Sprintf("r%d-%s", round, pid))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return opinion{}, err
	}
	out := providers.OutputSpec{Key: "opinion", Type: "report", Required: true, Path: filepath.Join(dir, "opinion.json")}
	layers := append(append([]domain.InstructionLayer(nil), snap.Instructions...), domain.InstructionLayer{
		Source: "task", Name: "회의 참여: " + n.Title, Text: n.Instructions,
	})
	req := providers.StartRequest{
		ProjectID: a.projectID, RunID: a.runID, StepAttemptID: a.id, StepID: n.ID, Generation: a.generation,
		Instructions: domain.ComposeInstructions(layers),
		Prompt:       meetingPrompt(e.cfg.DataDir, st, n, manifest, round, said, out.Path),
		Workspace:    work,
		WritableDirs: []string{dir},
		OutputSpec:   []providers.OutputSpec{out},
		Model:        snap.Model,
		Policy:       providers.Policy{Sandbox: "read-only", AskApproval: true},
		Limits:       providers.Limits{Timeout: e.cfg.MeetingTurnTimeout},
	}
	tctx, cancel := context.WithTimeout(a.ctx, e.cfg.MeetingTurnTimeout)
	defer cancel()
	sess, err := prov.Start(tctx, req)
	if err != nil {
		return opinion{}, err
	}
	if !a.setSession(sess) {
		drainUntil(sess, sideDrain)
		return opinion{}, errors.New("취소되었습니다")
	}
	p, err := e.runSide(tctx, a, sess, fmt.Sprintf("meeting:r%d:%s", round, pid))
	if err != nil {
		return opinion{}, err
	}
	if p.Status != providers.StatusSucceeded {
		return opinion{}, fmt.Errorf("%s %s", p.Status, p.Error)
	}
	op, err := e.readOpinion(out.Path, p.Text)
	op.Who, op.Round = pid, round
	return op, err
}

// readOpinion reads {"opinion": "...", "agree": bool}; a provider that
// only answered in text counts as not (yet) agreeing.
func (e *Engine) readOpinion(path, text string) (opinion, error) {
	var op opinion
	if resolved, err := e.resolveInside(path); err == nil {
		if info, err := os.Lstat(resolved); err == nil && info.Mode().IsRegular() && info.Size() <= maxOpinionBytes {
			data, _ := os.ReadFile(resolved)
			var doc struct {
				Opinion string `json:"opinion"`
				Summary string `json:"summary"`
				Agree   bool   `json:"agree"`
			}
			if json.Unmarshal(data, &doc) == nil {
				op.Text = strings.TrimSpace(doc.Opinion)
				if op.Text == "" {
					op.Text = strings.TrimSpace(doc.Summary)
				}
				op.Agree = doc.Agree
			}
		}
	}
	if op.Text == "" {
		op.Text, op.Agree = strings.TrimSpace(text), false
	}
	if op.Text == "" {
		return op, errors.New("의견이 비어 있습니다")
	}
	op.Text = clip(op.Text, contextEntryLimit*2)
	return op, nil
}

func meetingPrompt(dataDir string, st *runState, n *domain.Node, manifest []ManifestEntry, round int, said []opinion, outPath string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 회의: %s\n\n", n.Title)
	if g := st.version.Policy.Goal; g != "" {
		fmt.Fprintf(&b, "서비스 목표: %s\n\n", g)
	}
	agenda := strings.TrimSpace(n.Instructions)
	if agenda == "" {
		agenda = n.Title
	}
	fmt.Fprintf(&b, "## 의제\n\n%s\n\n", agenda)
	writeInputs(&b, dataDir, manifest)
	if len(said) > 0 {
		b.WriteString("## 지금까지 나온 의견\n\n")
		for _, o := range said {
			stance := "이견 있음"
			if o.Agree {
				stance = "동의"
			}
			fmt.Fprintf(&b, "- [%s · %d라운드 · %s]\n", actorName(st, o.Who), o.Round, stance)
			for _, line := range strings.Split(o.Text, "\n") {
				fmt.Fprintf(&b, "  > %s\n", line)
			}
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "## 이번 차례\n\n%d/%d 라운드입니다. 최종 결정과 결과물은 %s이(가) 만듭니다. 당신은 의견만 냅니다.\n\n",
		round, n.Meeting.Rounds(), actorName(st, n.AssignmentID))
	fmt.Fprintf(&b, "다음 JSON을 파일로 저장한 뒤 마치세요 → %s\n\n", outPath)
	b.WriteString("```json\n{\"opinion\": \"당신의 의견\", \"agree\": false}\n```\n\n")
	b.WriteString("- agree는 지금까지 나온 의견에 동의하고 더 보탤 내용이 없을 때만 true로 합니다.\n")
	b.WriteString("- 다른 파일은 만들거나 고치지 마세요.\n")
	return b.String()
}

// recordOpinion stores an opinion as a proposal to the step. It returns
// false if the attempt is no longer current and running.
func (e *Engine) recordOpinion(ctx context.Context, a *activeAttempt, n *domain.Node, op opinion) bool {
	ok := false
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, a.runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, a.id)
		if err != nil {
			return err
		}
		if !st.isCurrent(row) || row.Status != StRunning {
			return nil
		}
		agree := op.Agree
		_, err = addMessage(ctx, c, st.run.ProjectID, newMessage{
			RunID: st.run.ID, AttemptID: a.id, Sender: op.Who, Recipient: stepRecipient(n.ID), Kind: MsgProposal,
			Body: op.Text, Refs: MessageRefs{Round: op.Round, Agree: &agree},
		})
		ok = err == nil
		return err
	})
	if err != nil {
		e.cfg.Logf("engine: meeting %s opinion: %v", a.id, err)
	}
	return ok
}

// meetingToHuman hands the decision to the person once the rounds end.
func (e *Engine) meetingToHuman(ctx context.Context, a *activeAttempt) {
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, a.runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, a.id)
		if err != nil {
			return err
		}
		if !st.isCurrent(row) {
			return nil
		}
		if _, err := setAttemptStatus(ctx, c, st, row, StWaitingHuman, "", StRunning); err != nil {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	if err != nil {
		e.cfg.Logf("engine: meeting %s: %v", a.id, err)
	}
}

// endStopped closes an attempt whose work was stopped: superseded by a
// rework, or cancelled with its run. When the app itself is shutting
// down the attempt is left for start-up recovery to mark interrupted.
func (e *Engine) endStopped(ctx context.Context, a *activeAttempt) {
	if e.ctx.Err() != nil {
		return
	}
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, a.runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, a.id)
		if err != nil {
			return err
		}
		next := StSuperseded
		if st.run.Status == RunCancelled {
			next = StCancelled
		} else if st.isCurrent(row) {
			next = StCancelled
		}
		if _, err := setAttemptStatus(ctx, c, st, row, next, "", StRunning, StVerifying, StWaitingApproval, StWaitingInput); err != nil {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	if err != nil {
		e.cfg.Logf("engine: attempt %s stop: %v", a.id, err)
	}
}

// failAttempt fails a current, live attempt with a reason.
func (e *Engine) failAttempt(ctx context.Context, a *activeAttempt, reason string) {
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		st, err := loadState(ctx, c.Tx, a.runID)
		if err != nil {
			return err
		}
		row, _, err := loadAttempt(ctx, c.Tx, a.id)
		if err != nil {
			return err
		}
		if !st.isCurrent(row) {
			return nil
		}
		if _, err := setAttemptStatus(ctx, c, st, row, StFailed, reason, StRunning, StWaitingApproval, StWaitingInput); err != nil {
			return err
		}
		return saveRunStatus(ctx, c, st)
	})
	if err != nil {
		e.cfg.Logf("engine: attempt %s fail: %v", a.id, err)
	}
}
