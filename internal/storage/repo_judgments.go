package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"agent-office/internal/domain"
)

const JobJudgment = "judgment"

// Judgment outcomes (plan §4.3).
const (
	OutcomeNone     = "none"
	OutcomePropose  = "propose"
	OutcomeQuestion = "question"
)

// MaxProposals bounds the work one judgment may propose.
const MaxProposals = 5

// ProposedWork is one work item a judgment proposes.
type ProposedWork struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	DoneCriteria string   `json:"doneCriteria"`
	Categories   []string `json:"categories"`
	Priority     string   `json:"priority"`
}

// JudgmentResult is what a department's AI decided about a delivery.
type JudgmentResult struct {
	Outcome   string         `json:"outcome"`
	Reason    string         `json:"reason"`
	Evidence  []string       `json:"evidence"`
	Question  string         `json:"question"`
	Proposals []ProposedWork `json:"proposals"`
}

// Judgment is a stored judgment with what it was about.
type Judgment struct {
	ID             string    `json:"id"`
	ProjectID      string    `json:"projectId"`
	ProjectName    string    `json:"projectName"`
	DeliveryID     string    `json:"deliveryId"`
	Round          int       `json:"round"`
	AIJobID        string    `json:"aiJobId"`
	DepartmentID   string    `json:"departmentId"`
	DepartmentName string    `json:"departmentName"`
	Outcome        string    `json:"outcome"`
	Reason         string    `json:"reason"`
	Evidence       []string  `json:"evidence"`
	Question       string    `json:"question"`
	Answer         string    `json:"answer"`
	QuestionStatus string    `json:"questionStatus"`
	EventSequence  int64     `json:"eventSequence"`
	EventKind      string    `json:"eventKind"`
	Event          WorkEvent `json:"event"`
	CreatedAt      string    `json:"createdAt"`
}

// Proposal is a proposed work item with the judgment it came from.
type Proposal struct {
	ID             string   `json:"id"`
	ProjectID      string   `json:"projectId"`
	ProjectName    string   `json:"projectName"`
	JudgmentID     string   `json:"judgmentId"`
	Position       int      `json:"position"`
	DepartmentID   string   `json:"departmentId"`
	DepartmentName string   `json:"departmentName"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	DoneCriteria   string   `json:"doneCriteria"`
	Categories     []string `json:"categories"`
	Priority       string   `json:"priority"`
	Status         string   `json:"status"`
	WorkItemID     string   `json:"workItemId"`
	DuplicateOf    string   `json:"duplicateOf"`
	DecisionNote   string   `json:"decisionNote"`
	Reason         string   `json:"reason"` // the judgment's reason
	EventSequence  int64    `json:"eventSequence"`
	EventKind      string   `json:"eventKind"`
	EventTitle     string   `json:"eventTitle"`
	CreatedAt      string   `json:"createdAt"`
}

// normalizeTitle compares titles ignoring case, spaces and punctuation.
func normalizeTitle(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CompleteJudgment stores a judgment job's result in one transaction: the
// job becomes ready, the judgment and its proposals are recorded, and the
// delivery is done, or waits for an answer when round 1 asked a question.
// A question in round 2 is left for a person to decide.
func (db *DB) CompleteJudgment(ctx context.Context, job AIJob, r JudgmentResult) error {
	raw, _ := json.Marshal(r)
	_, err := db.Change(ctx, func(c *Change) error {
		tx := c.Tx
		cur, err := aiJobTx(ctx, tx, job.ID)
		if err != nil {
			return err
		}
		if cur.Status != JobWorking {
			return nil // finished meanwhile; the stored result stands
		}
		var deptID string
		if err := tx.QueryRowContext(ctx, `SELECT department_id FROM event_deliveries WHERE id = ?`, job.DeliveryID).Scan(&deptID); err != nil {
			return err
		}
		now := Now()
		if _, err := tx.ExecContext(ctx, `UPDATE work_ai_jobs SET status = 'ready', result = ?, updated_at = ? WHERE id = ?`, string(raw), now, job.ID); err != nil {
			return err
		}
		qStatus := ""
		if r.Outcome == OutcomeQuestion {
			qStatus = "open"
			if job.Round == 2 {
				qStatus = "manual"
			}
		}
		evidence, _ := json.Marshal(orEmpty(r.Evidence))
		jid := NewID("jdg")
		if _, err := tx.ExecContext(ctx, `INSERT INTO judgments (id, project_id, delivery_id, round, ai_job_id, department_id, outcome, reason, evidence,
			question, question_status, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			jid, job.ProjectID, job.DeliveryID, job.Round, job.ID, deptID, r.Outcome, r.Reason, string(evidence), r.Question, qStatus, now, now); err != nil {
			return err
		}
		if r.Outcome == OutcomePropose {
			if err := insertProposalsTx(ctx, tx, job.ProjectID, jid, deptID, r.Proposals, now); err != nil {
				return err
			}
		}
		status, note := domain.DeliveryDone, r.Reason
		switch {
		case r.Outcome == OutcomePropose:
			note = fmt.Sprintf("업무 %d건 제안: %s", len(r.Proposals), r.Reason)
		case r.Outcome == OutcomeQuestion && job.Round == 1:
			status, note = domain.DeliveryWaitingAnswer, "질문: "+r.Question
		case r.Outcome == OutcomeQuestion:
			note = "다시 질문해 사람이 결정합니다: " + r.Question
		default:
			note = "대응 불필요: " + r.Reason
		}
		if _, err := tx.ExecContext(ctx, `UPDATE event_deliveries SET status = ?, note = ?, error = '', hold_reason = '', updated_at = ? WHERE id = ?`,
			status, note, now, job.DeliveryID); err != nil {
			return err
		}
		if err := emitJobUpdate(c, job.ProjectID, job.ID, JobReady); err != nil {
			return err
		}
		return c.Emit(job.ProjectID, "", "", "judgment.ready", map[string]string{"judgmentId": jid, "outcome": r.Outcome})
	})
	return err
}

func insertProposalsTx(ctx context.Context, tx *sql.Tx, projectID, judgmentID, deptID string, props []ProposedWork, now string) error {
	known := map[string]string{} // normalized title -> what it duplicates
	rows, err := tx.QueryContext(ctx, `SELECT id, title FROM work_items WHERE department_id = ? AND status NOT IN ('done', 'cancelled')
		UNION ALL SELECT id, title FROM work_proposals WHERE department_id = ? AND status = 'proposed'`, deptID, deptID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			rows.Close()
			return err
		}
		known[normalizeTitle(title)] = id
	}
	rows.Close()
	for i, p := range props {
		if i >= MaxProposals {
			break
		}
		cats, _ := json.Marshal(orEmpty(p.Categories))
		if p.Priority == "" {
			p.Priority = "normal"
		}
		key := normalizeTitle(p.Title)
		dup := known[key]
		id := NewID("prop")
		if _, err := tx.ExecContext(ctx, `INSERT INTO work_proposals (id, project_id, judgment_id, position, department_id, title, description,
			done_criteria, categories, priority, duplicate_of, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id, projectID, judgmentID, i+1, deptID, p.Title, p.Description, p.DoneCriteria, string(cats), p.Priority, dup, now); err != nil {
			return err
		}
		if dup == "" {
			known[key] = id
		}
	}
	return nil
}

const judgmentSelect = `SELECT j.id, j.project_id, p.name, j.delivery_id, j.round, j.ai_job_id, j.department_id, d.name, j.outcome, j.reason, j.evidence,
	j.question, j.answer, j.question_status, e.sequence, e.kind, e.payload, j.created_at
	FROM judgments j JOIN projects p ON p.id = j.project_id JOIN departments d ON d.id = j.department_id
	JOIN event_deliveries dl ON dl.id = j.delivery_id JOIN execution_events e ON e.sequence = dl.event_sequence`

func scanJudgment(s interface{ Scan(...any) error }) (Judgment, error) {
	var j Judgment
	var evidence, payload string
	err := s.Scan(&j.ID, &j.ProjectID, &j.ProjectName, &j.DeliveryID, &j.Round, &j.AIJobID, &j.DepartmentID, &j.DepartmentName, &j.Outcome, &j.Reason,
		&evidence, &j.Question, &j.Answer, &j.QuestionStatus, &j.EventSequence, &j.EventKind, &payload, &j.CreatedAt)
	json.Unmarshal([]byte(evidence), &j.Evidence)
	json.Unmarshal([]byte(payload), &j.Event)
	j.Evidence, j.Event.Categories = orEmpty(j.Evidence), orEmpty(j.Event.Categories)
	return j, err
}

// Judgments lists judgments of a service ("" for every active service),
// newest first; questionStatus filters ("open,manual" style lists work).
func (db *DB) Judgments(ctx context.Context, projectID, questionStatus string) ([]Judgment, error) {
	if projectID != "" {
		if err := db.CheckScope(ctx, projectID); err != nil {
			return nil, err
		}
	}
	q := judgmentSelect + ` WHERE p.status = 'active' AND (? = '' OR j.project_id = ?)`
	args := []any{projectID, projectID}
	if questionStatus != "" {
		statuses := strings.Split(questionStatus, ",")
		q += ` AND j.question_status IN (` + strings.TrimSuffix(strings.Repeat("?, ", len(statuses)), ", ") + `)`
		for _, s := range statuses {
			args = append(args, strings.TrimSpace(s))
		}
	}
	rows, err := db.sql.QueryContext(ctx, q+` ORDER BY j.created_at DESC, j.rowid DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Judgment{}
	for rows.Next() {
		j, err := scanJudgment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func judgmentTx(ctx context.Context, q queryer, projectID, id string) (Judgment, error) {
	j, err := scanJudgment(q.QueryRowContext(ctx, judgmentSelect+` WHERE j.id = ? AND j.project_id = ?`, id, projectID))
	if errors.Is(err, sql.ErrNoRows) {
		return j, fmt.Errorf("judgment %q: %w", id, ErrNotInProject)
	}
	return j, err
}

// AnswerJudgment records a person's answer to a round-1 question and
// queues round 2 of the same delivery in the same transaction.
func (db *DB) AnswerJudgment(ctx context.Context, projectID, judgmentID, answer string) (AIJob, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return AIJob{}, invalid("답변을 적어 주세요")
	}
	var round2 string
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		j, err := judgmentTx(ctx, c.Tx, projectID, judgmentID)
		if err != nil {
			return err
		}
		if j.QuestionStatus != "open" || j.Round != 1 {
			return invalid("이미 답했거나 답할 수 없는 질문입니다")
		}
		first, err := aiJobTx(ctx, c.Tx, j.AIJobID)
		if err != nil {
			return err
		}
		now := Now()
		if _, err := c.Tx.ExecContext(ctx, `UPDATE judgments SET answer = ?, question_status = 'answered', updated_at = ? WHERE id = ?`, answer, now, j.ID); err != nil {
			return err
		}
		round2 = NewID("aij")
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO work_ai_jobs (id, project_id, kind, lane, request_key, delivery_id, round, assignment_id,
			connection_id, model, status, input_snapshot, input_hash, created_at, updated_at)
			VALUES (?, ?, 'judgment', ?, ?, ?, 2, NULLIF(?, ''), NULLIF(?, ''), ?, 'pending', ?, ?, ?, ?)`,
			round2, projectID, first.Lane, fmt.Sprintf("judgment:%s:2", j.DeliveryID), j.DeliveryID, first.AssignmentID, first.ConnectionID,
			first.Model, string(first.InputSnapshot), first.InputHash, now, now); err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE event_deliveries SET status = 'processing', note = '', updated_at = ? WHERE id = ? AND status = 'waiting_answer'`,
			now, j.DeliveryID); err != nil {
			return err
		}
		return emitJobUpdate(c, projectID, round2, JobPending)
	})
	if err != nil {
		return AIJob{}, err
	}
	return db.AIJob(ctx, projectID, round2)
}

// CloseJudgmentQuestion closes a question a person decided without the AI
// (round 2 asked again, or the person chose not to answer).
func (db *DB) CloseJudgmentQuestion(ctx context.Context, projectID, judgmentID, note string) error {
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		j, err := judgmentTx(ctx, c.Tx, projectID, judgmentID)
		if err != nil {
			return err
		}
		if j.QuestionStatus != "open" && j.QuestionStatus != "manual" {
			return invalid("닫을 질문이 없습니다")
		}
		now := Now()
		if _, err := c.Tx.ExecContext(ctx, `UPDATE judgments SET question_status = 'closed', answer = ?, updated_at = ? WHERE id = ?`,
			strings.TrimSpace(note), now, j.ID); err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE event_deliveries SET status = 'done', note = ?, updated_at = ? WHERE id = ? AND status = 'waiting_answer'`,
			"사람이 직접 결정했습니다", now, j.DeliveryID); err != nil {
			return err
		}
		return c.Emit(projectID, "", "", "judgment.ready", map[string]string{"judgmentId": j.ID, "outcome": "closed"})
	})
	return err
}

// ---- proposals ----

const proposalSelect = `SELECT w.id, w.project_id, p.name, w.judgment_id, w.position, w.department_id, d.name, w.title, w.description, w.done_criteria,
	w.categories, w.priority, w.status, COALESCE(w.work_item_id, ''), w.duplicate_of, w.decision_note, j.reason, e.sequence, e.kind, e.payload, w.created_at
	FROM work_proposals w JOIN projects p ON p.id = w.project_id JOIN departments d ON d.id = w.department_id JOIN judgments j ON j.id = w.judgment_id
	JOIN event_deliveries dl ON dl.id = j.delivery_id JOIN execution_events e ON e.sequence = dl.event_sequence`

func scanProposal(s interface{ Scan(...any) error }) (Proposal, error) {
	var p Proposal
	var cats, payload string
	err := s.Scan(&p.ID, &p.ProjectID, &p.ProjectName, &p.JudgmentID, &p.Position, &p.DepartmentID, &p.DepartmentName, &p.Title, &p.Description,
		&p.DoneCriteria, &cats, &p.Priority, &p.Status, &p.WorkItemID, &p.DuplicateOf, &p.DecisionNote, &p.Reason, &p.EventSequence, &p.EventKind,
		&payload, &p.CreatedAt)
	json.Unmarshal([]byte(cats), &p.Categories)
	var ev WorkEvent
	json.Unmarshal([]byte(payload), &ev)
	p.Categories, p.EventTitle = orEmpty(p.Categories), ev.Title
	return p, err
}

// Proposals lists proposals of a service ("" for every active service),
// oldest event first so related proposals stay together.
func (db *DB) Proposals(ctx context.Context, projectID, status string) ([]Proposal, error) {
	if projectID != "" {
		if err := db.CheckScope(ctx, projectID); err != nil {
			return nil, err
		}
	}
	rows, err := db.sql.QueryContext(ctx, proposalSelect+` WHERE p.status = 'active' AND (? = '' OR w.project_id = ?) AND (? = '' OR w.status = ?)
		ORDER BY e.sequence, w.judgment_id, w.position`, projectID, projectID, status, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Proposal{}
	for rows.Next() {
		p, err := scanProposal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ProposalEdit is what a person may change before accepting a proposal.
// Empty fields keep the proposal's value.
type ProposalEdit struct {
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	DoneCriteria string   `json:"doneCriteria"`
	Categories   []string `json:"categories"`
	Priority     string   `json:"priority"`
}

// AcceptProposal registers the proposed work once: the work keeps the
// proposing department, the event that caused it, and the event's goal
// when that goal is still open. A second accept of the same proposal is
// refused.
func (db *DB) AcceptProposal(ctx context.Context, projectID, proposalID string, edit ProposalEdit) (domain.WorkItem, error) {
	var workID string
	_, err := db.Change(ctx, func(c *Change) error {
		tx := c.Tx
		if err := writableProjectTx(ctx, tx, projectID); err != nil {
			return err
		}
		p, err := scanProposal(tx.QueryRowContext(ctx, proposalSelect+` WHERE w.id = ? AND w.project_id = ?`, proposalID, projectID))
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("proposal %q: %w", proposalID, ErrNotInProject)
		}
		if err != nil {
			return err
		}
		if p.Status != "proposed" {
			return invalid("이미 처리한 제안입니다")
		}
		w := domain.WorkItem{ProjectID: projectID, Kind: domain.WorkTask, DepartmentID: p.DepartmentID, Title: p.Title, Description: p.Description,
			DoneCriteria: p.DoneCriteria, Categories: p.Categories, Priority: p.Priority, Status: domain.WorkReceived}
		if edit.Title != "" {
			w.Title = edit.Title
		}
		if edit.Description != "" {
			w.Description = edit.Description
		}
		if edit.DoneCriteria != "" {
			w.DoneCriteria = edit.DoneCriteria
		}
		if edit.Categories != nil {
			w.Categories = edit.Categories
		}
		if edit.Priority != "" {
			w.Priority = edit.Priority
		}
		var payload string
		tx.QueryRowContext(ctx, `SELECT payload FROM execution_events WHERE sequence = ?`, p.EventSequence).Scan(&payload)
		var ev WorkEvent
		json.Unmarshal([]byte(payload), &ev)
		if ev.GoalID == "" && ev.WorkItemID != "" { // document events name only their work
			if src, err := workTx(ctx, tx, projectID, ev.WorkItemID); err == nil {
				ev.GoalID = workEventFor(src).GoalID
			}
		}
		if ev.GoalID != "" {
			if g, err := workTx(ctx, tx, projectID, ev.GoalID); err == nil && g.Kind == domain.WorkGoal && !domain.WorkClosed(g.Status) {
				w.ParentID = g.ID
			}
		}
		if err := normalizeWork(&w); err != nil {
			return err
		}
		if err := checkWorkLinks(ctx, tx, w, true); err != nil {
			return err
		}
		created, err := insertWorkTx(ctx, c, w, workOrigin{Seq: p.EventSequence, Proposal: p.ID})
		if err != nil {
			return err
		}
		workID = created
		res, err := tx.ExecContext(ctx, `UPDATE work_proposals SET status = 'accepted', work_item_id = ?, decided_at = ? WHERE id = ? AND status = 'proposed'`,
			workID, Now(), proposalID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return invalid("이미 처리한 제안입니다")
		}
		return nil
	})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return db.WorkItem(ctx, projectID, workID)
}

// RejectProposal records that a person declined a proposal.
func (db *DB) RejectProposal(ctx context.Context, projectID, proposalID, note string) error {
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		res, err := c.Tx.ExecContext(ctx, `UPDATE work_proposals SET status = 'rejected', decision_note = ?, decided_at = ?
			WHERE id = ? AND project_id = ? AND status = 'proposed'`, strings.TrimSpace(note), Now(), proposalID, projectID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n != 1 {
			return invalid("이미 처리했거나 찾을 수 없는 제안입니다")
		}
		return c.Emit(projectID, "", "", "proposal.updated", map[string]string{"proposalId": proposalID, "status": "rejected"})
	})
	return err
}
