package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"unicode/utf8"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// Inbox item kinds.
const (
	InboxTask         = "task"
	InboxReview       = "review"
	InboxApproval     = "approval"
	InboxToolApproval = "tool_approval"
	InboxQuestion     = "question"
	InboxBudget       = "budget"
)

type StepRef struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// InboxItem is one thing waiting for the person, with everything needed
// to act on it from any project.
type InboxItem struct {
	Kind          string          `json:"kind"`
	ProjectID     string          `json:"projectId"`
	ProjectName   string          `json:"projectName"`
	RunID         string          `json:"runId"`
	RunTitle      string          `json:"runTitle"`
	VersionNumber int             `json:"versionNumber"`
	StepID        string          `json:"stepId"`
	StepTitle     string          `json:"stepTitle"`
	Instructions  string          `json:"instructions"`
	AttemptID     string          `json:"attemptId"`
	Generation    int             `json:"generation"`
	Attempt       int             `json:"attempt"`
	Round         int             `json:"round"`
	ApprovalID    string          `json:"approvalId,omitempty"`
	MessageID     string          `json:"messageId,omitempty"`
	Detail        string          `json:"detail,omitempty"`
	Inputs        json.RawMessage `json:"inputs"`
	Outputs       []domain.Output `json:"outputs"`
	ReworkTargets []StepRef       `json:"reworkTargets"`
	// Context is what the step's AI would see besides its inputs:
	// handoff notes, earlier answers, notes and meeting opinions.
	Context []ContextNote `json:"context"`
	// Escalation explains why a question meant for an AI came to the
	// person instead.
	Escalation string `json:"escalation,omitempty"`
	// Workspace is where a person's task or review works (Git runs).
	Workspace *WorkspaceInfo `json:"workspace,omitempty"`
	Since      string `json:"since"`
}

// Inbox lists every open human item across active projects.
func (e *Engine) Inbox(ctx context.Context) ([]InboxItem, error) {
	q := e.db.Read()
	rows, err := q.QueryContext(ctx, `SELECT r.id, p.name FROM runs r JOIN projects p ON p.id = r.project_id
		WHERE r.status IN ('running', 'waiting', 'paused', 'failed') AND p.status = 'active' ORDER BY r.started_at`)
	if err != nil {
		return nil, err
	}
	type runRef struct{ id, project string }
	var runs []runRef
	for rows.Next() {
		var r runRef
		rows.Scan(&r.id, &r.project)
		runs = append(runs, r)
	}
	rows.Close()

	items := []InboxItem{}
	for _, r := range runs {
		st, err := loadState(ctx, q, r.id)
		if err != nil {
			return nil, err
		}
		if item, ok, err := budgetItem(ctx, q, st, r.project); err != nil {
			return nil, err
		} else if ok {
			items = append(items, item)
		}
		for _, id := range st.graph.Order() {
			a, ok := st.attempts[id]
			if !ok || !isWaiting(a.Status) {
				continue
			}
			n := st.graph.Node(id)
			base := InboxItem{
				ProjectID: st.run.ProjectID, ProjectName: r.project, RunID: st.run.ID, RunTitle: st.version.Spec.Title,
				VersionNumber: st.version.Number, StepID: id, StepTitle: n.Title, Instructions: n.Instructions,
				AttemptID: a.ID, Generation: a.Generation, Attempt: a.Attempt, Round: a.Round,
				Inputs: a.InputManifest, Outputs: n.Outputs, ReworkTargets: []StepRef{},
			}
			if base.Outputs == nil {
				base.Outputs = []domain.Output{}
			}
			for _, t := range n.ReworkTargets {
				base.ReworkTargets = append(base.ReworkTargets, StepRef{ID: t, Title: st.graph.Node(t).Title})
			}
			base.Context = []ContextNote{}
			if a.Status == StWaitingHuman {
				base.Workspace = workspaceInfo(ctx, q, a.ID)
			}
			if a.Status == StWaitingHuman || n.Kind == domain.KindApproval {
				var manifest []ManifestEntry
				json.Unmarshal(a.InputManifest, &manifest)
				notes, err := stepContext(ctx, q, st, n, manifest, a.ID)
				if err != nil {
					return nil, err
				}
				if notes != nil {
					base.Context = notes
				}
			}
			q.QueryRowContext(ctx, `SELECT COALESCE(started_at, created_at) FROM step_attempts WHERE id = ?`, a.ID).Scan(&base.Since)
			switch {
			case a.Status == StWaitingHuman && n.Kind == domain.KindReview:
				base.Kind = InboxReview
				items = append(items, base)
			case a.Status == StWaitingHuman:
				base.Kind = InboxTask
				items = append(items, base)
			case a.Status == StWaitingApproval && n.Kind == domain.KindApproval:
				base.Kind = InboxApproval
				q.QueryRowContext(ctx, `SELECT id FROM approvals WHERE step_attempt_id = ? AND kind = 'step' AND decision IS NULL`, a.ID).Scan(&base.ApprovalID)
				items = append(items, base)
			case a.Status == StWaitingApproval:
				rows, err := q.QueryContext(ctx, `SELECT id, target_manifest, created_at FROM approvals WHERE step_attempt_id = ? AND kind = 'tool' AND decision IS NULL`, a.ID)
				if err != nil {
					return nil, err
				}
				for rows.Next() {
					it := base
					it.Kind = InboxToolApproval
					var target string
					rows.Scan(&it.ApprovalID, &target, &it.Since)
					var t struct{ Action, Detail string }
					json.Unmarshal([]byte(target), &t)
					it.Detail = t.Action + ": " + t.Detail
					items = append(items, it)
				}
				rows.Close()
			case a.Status == StWaitingInput:
				// Questions for the person: addressed to them, or passed on
				// after another AI could not answer.
				rows, err := q.QueryContext(ctx, `SELECT m.id, m.body, m.created_at,
					COALESCE((SELECT x.body FROM messages x WHERE x.reply_to = m.id AND x.kind = 'escalation' LIMIT 1), '')
					FROM messages m WHERE m.step_attempt_id = ? AND m.kind = 'question'
					AND NOT EXISTS (SELECT 1 FROM messages r WHERE r.reply_to = m.id AND r.kind = 'answer')
					AND (m.recipient = 'local-owner' OR EXISTS (SELECT 1 FROM messages x WHERE x.reply_to = m.id AND x.kind = 'escalation'))`, a.ID)
				if err != nil {
					return nil, err
				}
				for rows.Next() {
					it := base
					it.Kind = InboxQuestion
					rows.Scan(&it.MessageID, &it.Detail, &it.Since, &it.Escalation)
					items = append(items, it)
				}
				rows.Close()
			}
		}
	}
	return items, nil
}

// ProjectSummary is one card on the services dashboard.
type ProjectSummary struct {
	Project     domain.Project `json:"project"`
	ActiveRuns  int            `json:"activeRuns"`
	WaitingRuns int            `json:"waitingRuns"`
	FailedRuns  int            `json:"failedRuns"`
	PausedRuns  int            `json:"pausedRuns"`
	Inbox       int            `json:"inbox"`
	LastRun     *RunBrief      `json:"lastRun"`
}

type RunBrief struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	StartedAt string `json:"startedAt"`
}

// Dashboard summarizes every project for the overview screen.
func (e *Engine) Dashboard(ctx context.Context, includeArchived bool) ([]ProjectSummary, error) {
	projects, err := e.db.Projects(ctx, includeArchived)
	if err != nil {
		return nil, err
	}
	inbox, err := e.Inbox(ctx)
	if err != nil {
		return nil, err
	}
	q := e.db.Read()
	out := []ProjectSummary{}
	for _, p := range projects {
		s := ProjectSummary{Project: p}
		rows, err := q.QueryContext(ctx, `SELECT status, COUNT(*) FROM runs WHERE project_id = ? GROUP BY status`, p.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var status string
			var n int
			rows.Scan(&status, &n)
			switch status {
			case RunRunning:
				s.ActiveRuns += n
			case RunWaiting:
				s.WaitingRuns += n
			case RunFailed, RunInterrupted:
				s.FailedRuns += n
			case RunPaused:
				s.PausedRuns += n
			}
		}
		rows.Close()
		var b RunBrief
		var versionID string
		if err := q.QueryRowContext(ctx, `SELECT id, workflow_version_id, status, started_at FROM runs WHERE project_id = ? ORDER BY started_at DESC LIMIT 1`, p.ID).
			Scan(&b.ID, &versionID, &b.Status, &b.StartedAt); err == nil {
			if v, err := loadVersion(ctx, q, versionID); err == nil {
				b.Title = v.Spec.Title
			}
			s.LastRun = &b
		}
		for _, it := range inbox {
			if it.ProjectID == p.ID {
				s.Inbox++
			}
		}
		out = append(out, s)
	}
	return out, nil
}

// ArtifactContent is an artifact's metadata and (text) content.
type ArtifactContent struct {
	ArtifactView
	StepID    string `json:"stepId"`
	Content   string `json:"content"`
	Binary    bool   `json:"binary"`
	Truncated bool   `json:"truncated"`
	HashOK    bool   `json:"hashOk"`
}

const maxArtifactRead = 1 << 20

// ReadArtifact returns an artifact's content after checking it belongs
// to the project and its file still matches the recorded hash.
func (e *Engine) ReadArtifact(ctx context.Context, projectID, artifactID string) (ArtifactContent, error) {
	if err := e.db.CheckScope(ctx, projectID, storage.Ref{Kind: storage.RefArtifact, ID: artifactID}); err != nil {
		return ArtifactContent{}, err
	}
	var c ArtifactContent
	err := e.db.Read().QueryRowContext(ctx, `SELECT a.id, a.output_key, a.version, a.type, a.path, a.hash, a.validity, s.step_id
		FROM artifacts a JOIN step_attempts s ON s.id = a.step_attempt_id WHERE a.id = ?`, artifactID).
		Scan(&c.ID, &c.OutputKey, &c.Version, &c.Type, &c.Path, &c.Hash, &c.Validity, &c.StepID)
	if err != nil {
		return c, err
	}
	full, err := e.resolveInside(filepath.Join(e.cfg.DataDir, filepath.FromSlash(c.Path)))
	if err != nil {
		return c, fmt.Errorf("결과 파일을 열 수 없습니다: %w", err)
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return c, err
	}
	sum := sha256.Sum256(data)
	c.HashOK = hex.EncodeToString(sum[:]) == c.Hash
	if len(data) > maxArtifactRead {
		data, c.Truncated = data[:maxArtifactRead], true
	}
	if utf8.Valid(data) || c.Truncated {
		c.Content = string(data)
	} else {
		c.Binary = true
	}
	return c, nil
}

// budgetItem reports a run whose ready AI work is held by the budget.
func budgetItem(ctx context.Context, q querier, st *runState, projectName string) (InboxItem, bool, error) {
	if st.run.Status != RunWaiting || st.run.Paused {
		return InboxItem{}, false, nil
	}
	held := ""
	for _, id := range st.graph.Order() {
		n := st.graph.Node(id)
		if st.status(id) != StPending || n.Kind == domain.KindCondition || n.Kind == domain.KindJoin || n.Kind == domain.KindApproval {
			continue
		}
		if a := st.version.Assignments[n.AssignmentID]; a.ActorKind != domain.ActorAI && n.Meeting == nil {
			continue
		}
		if done, _ := st.depsDone(n); done {
			held = n.Title
			break
		}
	}
	if held == "" {
		return InboxItem{}, false, nil
	}
	u, err := projectUsage(ctx, q, st.run.ProjectID)
	if err != nil || u.HoldReason == "" {
		return InboxItem{}, false, err
	}
	return InboxItem{
		Kind: InboxBudget, ProjectID: st.run.ProjectID, ProjectName: projectName, RunID: st.run.ID,
		RunTitle: st.version.Spec.Title, VersionNumber: st.version.Number, StepTitle: held, Detail: u.HoldReason,
		Inputs: json.RawMessage("[]"), Outputs: []domain.Output{}, ReworkTargets: []StepRef{}, Context: []ContextNote{},
	}, true, nil
}
