package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-office/internal/domain"
)

// Collaboration request statuses and requirements (plan §4.5).
const (
	CollabRequested = "requested"
	CollabQuestion  = "question"
	CollabAccepted  = "accepted"
	CollabDeclined  = "declined"
	CollabCancelled = "cancelled"

	CollabReference = "reference"
	CollabRequired  = "required"
)

// CauseRequiredReopened is the impact check a person gets when a required
// condition of finished (or nearly finished) work is open again: the
// helping work was reopened or cancelled, or a goal that is done gained a
// required request that is not met.
const CauseRequiredReopened = "required_reopened"

// CollabRequest is one department asking another for help on a work item.
// Satisfied and Overdue are worked out when it is read.
type CollabRequest struct {
	ID                 string          `json:"id"`
	ProjectID          string          `json:"projectId"`
	ProjectName        string          `json:"projectName"`
	FromWorkItemID     string          `json:"fromWorkItemId"`
	FromWorkTitle      string          `json:"fromWorkTitle"`
	FromWorkStatus     string          `json:"fromWorkStatus"`
	GoalID             string          `json:"goalId"` // the asking work's goal (itself for a goal)
	FromDepartmentID   string          `json:"fromDepartmentId"`
	FromDepartmentName string          `json:"fromDepartmentName"`
	ToDepartmentID     string          `json:"toDepartmentId"`
	ToDepartmentName   string          `json:"toDepartmentName"`
	Question           string          `json:"question"`
	InputVersionIDs    []string        `json:"inputVersionIds"`
	Requirement        string          `json:"requirement"` // reference | required
	RequirementReason  string          `json:"requirementReason"`
	Status             string          `json:"status"`
	FollowUp           string          `json:"followUp"`  // the receiving department's open question
	DecidedBy          string          `json:"decidedBy"` // user | ai
	DecisionJobID      string          `json:"decisionJobId"`
	DecisionReason     string          `json:"decisionReason"`
	DecidedAt          string          `json:"decidedAt"`
	ReviewedAt         string          `json:"reviewedAt"` // when the person looked at an AI decision
	RevertedAt         string          `json:"revertedAt"`
	RevertReason       string          `json:"revertReason"`
	AIStatus           string          `json:"aiStatus"`  // failed | held: the lead AI did not decide this undecided request
	AIProblem          string          `json:"aiProblem"` // why; the person decides instead
	ResultWorkItemID   string          `json:"resultWorkItemId"`
	ResultWorkTitle    string          `json:"resultWorkTitle"`
	ResultWorkStatus   string          `json:"resultWorkStatus"`
	ReplacesID         string          `json:"replacesId"`
	ReplacedByID       string          `json:"replacedById"`
	DueAt              string          `json:"dueAt"`
	EventSequence      int64           `json:"eventSequence"`
	Satisfied          bool            `json:"satisfied"`
	Overdue            bool            `json:"overdue"`
	History            []CollabHistory `json:"history"`
	CreatedAt          string          `json:"createdAt"`
	UpdatedAt          string          `json:"updatedAt"`
}

// Blocks reports whether the request stops the asking work (and its goal)
// from being completed: it is required and not yet accepted with its work
// done.
func (r CollabRequest) Blocks() bool { return r.Requirement == CollabRequired && !r.Satisfied }

// BlockReason says, for a person, why a required request is not met.
func (r CollabRequest) BlockReason() string {
	var s string
	switch r.Status {
	case CollabRequested:
		s = "수락 대기"
	case CollabQuestion:
		s = "받는 부서의 질문에 답해야 함"
	case CollabDeclined:
		s = "거절됨, 필수 해제나 대체가 필요"
	case CollabCancelled:
		s = "취소됨, 필수 해제나 대체가 필요"
	default:
		s = fmt.Sprintf("수행 업무 ‘%s’ 미완료", r.ResultWorkTitle)
		if r.ResultWorkStatus == domain.WorkCancelled {
			s = fmt.Sprintf("수행 업무 ‘%s’ 취소됨", r.ResultWorkTitle)
		}
	}
	if r.Overdue {
		s += ", 기한 지남"
	}
	return r.ToDepartmentName + ": " + s
}

// CollabHistory is one thing that happened to a request. AsDepartmentID
// is the department the person acted for ("" for the person in charge of
// the asking work).
type CollabHistory struct {
	Action         string `json:"action"`
	Body           string `json:"body"`
	AsDepartmentID string `json:"asDepartmentId"`
	Author         string `json:"author"`
	CreatedAt      string `json:"createdAt"`
}

const collabSelect = `SELECT r.id, r.project_id, p.name, r.from_work_item_id, fw.title, fw.status,
	COALESCE(CASE WHEN fw.kind = 'goal' THEN fw.id ELSE fw.parent_id END, ''),
	r.from_department_id, fd.name, r.to_department_id, td.name, r.question, r.input_versions, r.requirement, r.requirement_reason,
	r.status, r.follow_up, r.decided_by, COALESCE(r.decision_job_id, ''), r.decision_reason, COALESCE(r.decided_at, ''),
	COALESCE(r.reviewed_at, ''), COALESCE(r.reverted_at, ''), r.revert_reason,
	COALESCE(ad.status, ''), COALESCE(CASE ad.status WHEN 'failed' THEN ad.error ELSE ad.hold_reason END, ''),
	COALESCE(r.result_work_item_id, ''), COALESCE(rw.title, ''), COALESCE(rw.status, ''), COALESCE(r.replaces_id, ''),
	COALESCE((SELECT n.id FROM collab_requests n WHERE n.replaces_id = r.id ORDER BY n.created_at DESC LIMIT 1), ''),
	COALESCE(r.due_at, ''), COALESCE(r.event_sequence, 0), r.created_at, r.updated_at
	FROM collab_requests r JOIN projects p ON p.id = r.project_id JOIN work_items fw ON fw.id = r.from_work_item_id
	JOIN departments fd ON fd.id = r.from_department_id JOIN departments td ON td.id = r.to_department_id
	LEFT JOIN work_items rw ON rw.id = r.result_work_item_id
	LEFT JOIN event_deliveries ad ON ad.event_sequence = r.event_sequence AND ad.department_id = r.to_department_id
		AND ad.purpose = 'collab_request' AND ad.status IN ('failed', 'held') AND r.status = 'requested'`

func scanCollab(s interface{ Scan(...any) error }) (CollabRequest, error) {
	var r CollabRequest
	var versions string
	err := s.Scan(&r.ID, &r.ProjectID, &r.ProjectName, &r.FromWorkItemID, &r.FromWorkTitle, &r.FromWorkStatus, &r.GoalID,
		&r.FromDepartmentID, &r.FromDepartmentName, &r.ToDepartmentID, &r.ToDepartmentName, &r.Question, &versions, &r.Requirement,
		&r.RequirementReason, &r.Status, &r.FollowUp, &r.DecidedBy, &r.DecisionJobID, &r.DecisionReason, &r.DecidedAt,
		&r.ReviewedAt, &r.RevertedAt, &r.RevertReason, &r.AIStatus, &r.AIProblem,
		&r.ResultWorkItemID, &r.ResultWorkTitle, &r.ResultWorkStatus, &r.ReplacesID, &r.ReplacedByID,
		&r.DueAt, &r.EventSequence, &r.CreatedAt, &r.UpdatedAt)
	json.Unmarshal([]byte(versions), &r.InputVersionIDs)
	r.InputVersionIDs, r.History = orEmpty(r.InputVersionIDs), []CollabHistory{}
	r.Satisfied = r.Status == CollabAccepted && r.ResultWorkStatus == domain.WorkDone
	r.Overdue = r.DueAt != "" && !r.Satisfied && r.Status != CollabCancelled && r.DueAt < time.Now().Format("2006-01-02")
	return r, err
}

// queryCollabs reads requests with their history.
func queryCollabs(ctx context.Context, q rowsQueryer, where string, args ...any) ([]CollabRequest, error) {
	rows, err := q.QueryContext(ctx, collabSelect+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	out := []CollabRequest{}
	index := map[string]int{}
	for rows.Next() {
		r, err := scanCollab(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		index[r.ID] = len(out)
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	ids := make([]any, 0, len(out))
	for _, r := range out {
		ids = append(ids, r.ID)
	}
	hrows, err := q.QueryContext(ctx, `SELECT request_id, action, body, as_department_id, author, created_at FROM collab_request_history
		WHERE request_id IN (`+strings.TrimSuffix(strings.Repeat("?, ", len(ids)), ", ")+`) ORDER BY created_at, rowid`, ids...)
	if err != nil {
		return nil, err
	}
	defer hrows.Close()
	for hrows.Next() {
		var id string
		var h CollabHistory
		if err := hrows.Scan(&id, &h.Action, &h.Body, &h.AsDepartmentID, &h.Author, &h.CreatedAt); err != nil {
			return nil, err
		}
		out[index[id]].History = append(out[index[id]].History, h)
	}
	return out, hrows.Err()
}

func collabTx(ctx context.Context, q rowsQueryer, projectID, id string) (CollabRequest, error) {
	list, err := queryCollabs(ctx, q, `r.id = ? AND r.project_id = ?`, id, projectID)
	if err != nil {
		return CollabRequest{}, err
	}
	if len(list) == 0 {
		return CollabRequest{}, fmt.Errorf("collaboration request %q: %w", id, ErrNotInProject)
	}
	return list[0], nil
}

// CollabRequest returns one request of the service with its history.
func (db *DB) CollabRequest(ctx context.Context, projectID, id string) (CollabRequest, error) {
	if err := db.CheckScope(ctx, projectID); err != nil {
		return CollabRequest{}, err
	}
	return collabTx(ctx, db.sql, projectID, id)
}

// CollabFilter narrows the request list. Empty fields match all.
type CollabFilter struct {
	// WorkItemID keeps requests the work asked; for a goal also those its
	// work asked.
	WorkItemID       string `json:"workItemId"`
	ResultWorkItemID string `json:"resultWorkItemId"`
	DepartmentID     string `json:"departmentId"` // asked of this department
	// Open keeps what waits on a person: requests to decide, questions to
	// answer, and required requests that were declined.
	Open bool `json:"open"`
	// AIUnreviewed keeps decisions the lead AI made that the person has
	// not looked at yet.
	AIUnreviewed bool `json:"aiUnreviewed"`
}

// CollabRequests lists a service's requests ("" for every active
// service), oldest first.
func (db *DB) CollabRequests(ctx context.Context, projectID string, f CollabFilter) ([]CollabRequest, error) {
	if projectID != "" {
		if err := db.CheckScope(ctx, projectID); err != nil {
			return nil, err
		}
	}
	where := `p.status = 'active' AND (? = '' OR r.project_id = ?)`
	args := []any{projectID, projectID}
	if f.WorkItemID != "" {
		where += ` AND (r.from_work_item_id = ? OR fw.parent_id = ?)`
		args = append(args, f.WorkItemID, f.WorkItemID)
	}
	if f.ResultWorkItemID != "" {
		where += ` AND r.result_work_item_id = ?`
		args = append(args, f.ResultWorkItemID)
	}
	if f.DepartmentID != "" {
		where += ` AND r.to_department_id = ?`
		args = append(args, f.DepartmentID)
	}
	if f.Open {
		where += ` AND (r.status IN ('requested', 'question') OR (r.status = 'declined' AND r.requirement = 'required'))`
	}
	if f.AIUnreviewed {
		where += ` AND r.decided_by = 'ai' AND r.reviewed_at IS NULL AND r.status IN ('accepted', 'declined')`
	}
	return queryCollabs(ctx, db.sql, where+` ORDER BY r.created_at, r.rowid`, args...)
}

// unmetRequiredTx lists the required requests that keep a work item from
// being completed: its own, and for a goal those of the work under it.
// A request counts whether or not work was ever made for it.
func unmetRequiredTx(ctx context.Context, q rowsQueryer, w domain.WorkItem) ([]CollabRequest, error) {
	list, err := queryCollabs(ctx, q, `r.project_id = ? AND r.requirement = 'required' AND (r.from_work_item_id = ? OR fw.parent_id = ?)
		AND NOT (r.status = 'accepted' AND COALESCE(rw.status, '') = 'done') ORDER BY r.created_at, r.rowid`, w.ProjectID, w.ID, w.ID)
	return list, err
}

// CompletionBlockers lists the required requests not yet met for a work
// item (and the work under a goal).
func (db *DB) CompletionBlockers(ctx context.Context, projectID, workItemID string) ([]CollabRequest, error) {
	w, err := db.WorkItem(ctx, projectID, workItemID)
	if err != nil {
		return nil, err
	}
	return unmetRequiredTx(ctx, db.sql, w)
}

// checkRequiredMetTx refuses to complete w while a required request is
// not met. It runs in the transaction that stores the completion.
func checkRequiredMetTx(ctx context.Context, tx *sql.Tx, w domain.WorkItem) error {
	unmet, err := unmetRequiredTx(ctx, tx, w)
	if err != nil || len(unmet) == 0 {
		return err
	}
	var why []string
	for _, r := range unmet {
		why = append(why, r.BlockReason())
	}
	return invalid("필수 협업이 끝나지 않아 완료할 수 없습니다. %s", strings.Join(why, "; "))
}

// requiredReachesTx reports whether from waits, through required requests
// whose work exists, on target. Only work items are on such paths: a goal
// is never the work of a request.
func requiredReachesTx(ctx context.Context, q rowsQueryer, from, target string) (bool, error) {
	seen := map[string]bool{from: true}
	next := []string{from}
	for len(next) > 0 {
		cur := next[0]
		next = next[1:]
		if cur == target {
			return true, nil
		}
		rows, err := q.QueryContext(ctx, `SELECT result_work_item_id FROM collab_requests
			WHERE from_work_item_id = ? AND requirement = 'required' AND status = 'accepted' AND result_work_item_id IS NOT NULL`, cur)
		if err != nil {
			return false, err
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return false, err
			}
			if !seen[id] {
				seen[id] = true
				next = append(next, id)
			}
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return false, err
		}
	}
	return false, nil
}

func checkNoCycleTx(ctx context.Context, tx *sql.Tx, r CollabRequest, resultID string) error {
	loop, err := requiredReachesTx(ctx, tx, resultID, r.FromWorkItemID)
	if err != nil {
		return err
	}
	if loop {
		return invalid("필수 협업이 서로의 완료를 기다리게 됩니다. 이 업무는 이미 ‘%s’의 결과를 필수로 기다립니다", r.FromWorkTitle)
	}
	return nil
}

func addCollabHistoryTx(ctx context.Context, tx *sql.Tx, projectID, requestID, action, body, asDept, now string) error {
	return addCollabHistoryByTx(ctx, tx, projectID, requestID, action, body, asDept, domain.LocalOwner, now)
}

// addCollabHistoryByTx records who acted: the local person, or "ai:<job>"
// for the department's lead AI.
func addCollabHistoryByTx(ctx context.Context, tx *sql.Tx, projectID, requestID, action, body, asDept, author, now string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO collab_request_history (id, project_id, request_id, action, body, as_department_id, author, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, NewID("chist"), projectID, requestID, action, strings.TrimSpace(body), asDept, author, now)
	return err
}

// openRequiredCheckTx asks a person to look at a work item again because
// a required condition is open again. An earlier check for the same
// request is opened again with the new summary.
func openRequiredCheckTx(ctx context.Context, c *Change, projectID, workID, requestID, summary string) error {
	now := Now()
	id := NewID("imp")
	if err := c.Tx.QueryRowContext(ctx, `INSERT INTO impact_checks (id, project_id, work_item_id, cause, ref_id, summary, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(work_item_id, cause, ref_id) DO UPDATE SET status = 'open', summary = excluded.summary, note = '', updated_at = excluded.updated_at
		RETURNING id`, id, projectID, workID, CauseRequiredReopened, requestID, summary, now, now).Scan(&id); err != nil {
		return err
	}
	return c.Emit(projectID, "", "", "impact.updated", map[string]string{"impactId": id, "status": "open"})
}

// flagDoneGoalTx asks for a review of the goal above w when that goal is
// done but a required request under it is not met. The goal is not
// reopened.
func flagDoneGoalTx(ctx context.Context, c *Change, w domain.WorkItem, r CollabRequest) error {
	goal := workEventFor(w).GoalID
	if goal == "" || goal == w.ID {
		return nil
	}
	g, err := workTx(ctx, c.Tx, w.ProjectID, goal)
	if err != nil || g.Status != domain.WorkDone {
		return err
	}
	return openRequiredCheckTx(ctx, c, w.ProjectID, g.ID, r.ID, fmt.Sprintf(
		"완료한 목표에 끝나지 않은 필수 협업이 생겼습니다(‘%s’ → %s). 목표는 완료 상태로 두었습니다. 다시 열지 검토하세요", w.Title, r.ToDepartmentName))
}

// requiredWorkLeftDoneTx runs when a work item that was done is reopened
// or cancelled. When it is the work of a required request, the asking
// work and its goal get an impact check; neither is reopened.
func requiredWorkLeftDoneTx(ctx context.Context, c *Change, w domain.WorkItem, status string) error {
	list, err := queryCollabs(ctx, c.Tx, `r.project_id = ? AND r.result_work_item_id = ? AND r.requirement = 'required' AND r.status = 'accepted'`,
		w.ProjectID, w.ID)
	if err != nil {
		return err
	}
	what := "다시 열렸습니다"
	if status == domain.WorkCancelled {
		what = "취소되었습니다"
	}
	for _, r := range list {
		from, err := workTx(ctx, c.Tx, w.ProjectID, r.FromWorkItemID)
		if err != nil {
			return err
		}
		head := fmt.Sprintf("필수 협업 업무 ‘%s’(%s)가 %s.", w.Title, r.ToDepartmentName, what)
		tail := " 다시 완료될 때까지 이 업무를 완료할 수 없습니다."
		if from.Status == domain.WorkDone {
			tail = " 완료한 이 업무는 그대로 두었습니다. 결과를 다시 확인하고 필요하면 다시 여세요."
		}
		if err := openRequiredCheckTx(ctx, c, w.ProjectID, from.ID, r.ID, head+tail); err != nil {
			return err
		}
		goal := workEventFor(from).GoalID
		if goal == "" || goal == from.ID {
			continue
		}
		g, err := workTx(ctx, c.Tx, w.ProjectID, goal)
		if err != nil {
			return err
		}
		tail = " 다시 완료될 때까지 목표를 완료할 수 없습니다."
		if g.Status == domain.WorkDone {
			tail = " 완료한 목표는 그대로 두었습니다. 결과를 다시 확인하고 필요하면 다시 여세요."
		}
		if err := openRequiredCheckTx(ctx, c, w.ProjectID, g.ID, r.ID, fmt.Sprintf("‘%s’의 %s", from.Title, head)+tail); err != nil {
			return err
		}
	}
	return nil
}

// NewCollabRequest is what a department asks of another.
type NewCollabRequest struct {
	FromWorkItemID    string   `json:"fromWorkItemId"`
	ToDepartmentID    string   `json:"toDepartmentId"`
	Question          string   `json:"question"`
	InputVersionIDs   []string `json:"inputVersionIds"`
	Requirement       string   `json:"requirement"` // reference (default) | required
	RequirementReason string   `json:"requirementReason"`
	DueAt             string   `json:"dueAt"` // YYYY-MM-DD or empty
}

// RequestCollab stores a request from the work's department to another
// and emits work.collab_requested, which reaches the receiving department
// whatever it subscribes to. Material sent with it must be visible to
// that department.
func (db *DB) RequestCollab(ctx context.Context, projectID string, n NewCollabRequest) (CollabRequest, error) {
	var id string
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		var err error
		id, err = insertCollabTx(ctx, c, projectID, n, "")
		return err
	})
	if err != nil {
		return CollabRequest{}, err
	}
	return db.CollabRequest(ctx, projectID, id)
}

func insertCollabTx(ctx context.Context, c *Change, projectID string, n NewCollabRequest, replaces string) (string, error) {
	tx := c.Tx
	from, err := workTx(ctx, tx, projectID, n.FromWorkItemID)
	if err != nil {
		return "", err
	}
	if domain.WorkClosed(from.Status) {
		return "", invalid("닫힌 업무에서는 협업을 요청할 수 없습니다. 업무를 다시 연 뒤 요청하세요")
	}
	if from.DepartmentID == "" {
		return "", invalid("협업을 요청하려면 이 업무에 담당 부서가 있어야 합니다")
	}
	to, err := departmentTx(ctx, tx, projectID, n.ToDepartmentID)
	if err != nil {
		return "", err
	}
	if to.Status != domain.StatusActive {
		return "", invalid("사용하지 않는 부서에는 협업을 요청할 수 없습니다")
	}
	if to.ID == from.DepartmentID {
		return "", invalid("같은 부서에는 협업을 요청할 수 없습니다")
	}
	question := strings.TrimSpace(n.Question)
	if question == "" {
		return "", invalid("무엇을 도와 달라는지 적어 주세요")
	}
	if n.Requirement == "" {
		n.Requirement = CollabReference
	}
	if n.Requirement != CollabReference && n.Requirement != CollabRequired {
		return "", invalid("협업은 reference 또는 required입니다")
	}
	if n.DueAt != "" {
		if _, err := time.Parse("2006-01-02", n.DueAt); err != nil {
			return "", invalid("기한은 YYYY-MM-DD 형식이어야 합니다")
		}
	}
	versions := orEmpty(dedupe(n.InputVersionIDs))
	for _, vid := range versions {
		v, err := versionTx(ctx, tx, projectID, vid)
		if err != nil {
			return "", err
		}
		doc, err := documentTx(ctx, tx, projectID, v.DocumentID)
		if err != nil {
			return "", err
		}
		if v.WithdrawnAt != "" || doc.Status != "active" {
			return "", invalid("철회된 자료는 보낼 수 없습니다: %s", doc.Title)
		}
		if !doc.VisibleTo(to.ID) {
			return "", invalid("%s는 ‘%s’ 자료를 볼 수 없습니다. 공유 범위를 넓히거나 자료를 빼세요", to.Name, doc.Title)
		}
	}
	raw, _ := json.Marshal(versions)
	id, now := NewID("collab"), Now()
	reason := strings.TrimSpace(n.RequirementReason)
	if _, err := tx.ExecContext(ctx, `INSERT INTO collab_requests (id, project_id, from_work_item_id, from_department_id, to_department_id, question,
		input_versions, requirement, requirement_reason, replaces_id, due_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULLIF(?, ''), NULLIF(?, ''), ?, ?)`,
		id, projectID, from.ID, from.DepartmentID, to.ID, question, string(raw), n.Requirement, reason, replaces, n.DueAt, now, now); err != nil {
		return "", err
	}
	if err := addCollabHistoryTx(ctx, tx, projectID, id, "requested", question, from.DepartmentID, now); err != nil {
		return "", err
	}
	if n.Requirement == CollabRequired {
		if err := addCollabHistoryTx(ctx, tx, projectID, id, "required", reason, "", now); err != nil {
			return "", err
		}
	}
	ev := workEventFor(from)
	ev.TargetDepartmentID, ev.CollabRequestID, ev.Note = to.ID, id, question
	rec, err := emitWorkEventTx(ctx, c, projectID, domain.EventWorkCollabRequested, ev)
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collab_requests SET event_sequence = ? WHERE id = ?`, rec.Sequence, id); err != nil {
		return "", err
	}
	if n.Requirement == CollabRequired {
		r, err := collabTx(ctx, tx, projectID, id)
		if err != nil {
			return "", err
		}
		return id, flagDoneGoalTx(ctx, c, from, r)
	}
	return id, nil
}

// changeCollab runs fn on one request of a writable service and tells
// open screens about the change.
func (db *DB) changeCollab(ctx context.Context, projectID, id string, fn func(c *Change, r CollabRequest, now string) error) (CollabRequest, error) {
	_, err := db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		r, err := collabTx(ctx, c.Tx, projectID, id)
		if err != nil {
			return err
		}
		now := Now()
		if err := fn(c, r, now); err != nil {
			return err
		}
		if r, err = collabTx(ctx, c.Tx, projectID, id); err != nil {
			return err
		}
		if err := settleCollabDeliveryTx(ctx, c, r, now); err != nil {
			return err
		}
		return c.Emit(projectID, "", "", "collab.updated", map[string]string{"collabRequestId": id})
	})
	if err != nil {
		return CollabRequest{}, err
	}
	return db.CollabRequest(ctx, projectID, id)
}

// settleCollabDeliveryTx closes a request's failed or held lead-AI
// decision once the person decided the request or made it required:
// nothing is left for the AI, and a retry would only spend a call.
func settleCollabDeliveryTx(ctx context.Context, c *Change, r CollabRequest, now string) error {
	if r.Status == CollabRequested && r.Requirement != CollabRequired {
		return nil
	}
	note := "사람이 결정해 AI 결정이 필요 없습니다"
	if r.Status == CollabRequested {
		note = "완료에 필요한 협업이 되어 사람이 결정합니다"
	}
	var id string
	err := c.Tx.QueryRowContext(ctx, `SELECT id FROM event_deliveries WHERE event_sequence = ? AND department_id = ? AND purpose = 'collab_request'
		AND status IN ('failed', 'held')`, r.EventSequence, r.ToDepartmentID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := c.Tx.ExecContext(ctx, `UPDATE event_deliveries SET status = ?, note = ?, error = '', hold_reason = '', updated_at = ? WHERE id = ?`,
		domain.DeliveryDone, note, now, id); err != nil {
		return err
	}
	return c.Emit(r.ProjectID, "", "", "delivery.updated", map[string]string{"deliveryId": id, "status": domain.DeliveryDone})
}

func undecided(r CollabRequest) error {
	if r.Status != CollabRequested && r.Status != CollabQuestion {
		return invalid("이미 결정한 협업 요청입니다")
	}
	return nil
}

// CollabAcceptance is how the receiving department takes a request on:
// new work (the fields, or defaults from the request), or one of its open
// work items.
type CollabAcceptance struct {
	ExistingWorkItemID string `json:"existingWorkItemId"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	DoneCriteria       string `json:"doneCriteria"`
	Priority           string `json:"priority"`
	Note               string `json:"note"`
}

// AcceptCollab records that the person, acting for the receiving
// department, took the request on. The department's work is created
// once (or an open work item of it is chosen), starts from the request's
// event, sits under the asking work's goal while that is open, and links
// the material sent with the request.
func (db *DB) AcceptCollab(ctx context.Context, projectID, id string, a CollabAcceptance) (CollabRequest, error) {
	return db.changeCollab(ctx, projectID, id, func(c *Change, r CollabRequest, now string) error {
		if err := undecided(r); err != nil {
			return err
		}
		return acceptCollabTx(ctx, c, r, a, collabDecider{}, now)
	})
}

// collabDecider is who decides a request: the person (zero value), or the
// receiving department's lead AI through one AI job.
type collabDecider struct{ JobID string }

func (d collabDecider) by() string {
	if d.JobID != "" {
		return "ai"
	}
	return "user"
}

func (d collabDecider) author() string {
	if d.JobID != "" {
		return "ai:" + d.JobID
	}
	return domain.LocalOwner
}

// acceptCollabTx takes an undecided request on. Work the lead AI
// registers continues the request event's chain, so the chain limits
// count it.
func acceptCollabTx(ctx context.Context, c *Change, r CollabRequest, a CollabAcceptance, who collabDecider, now string) error {
	tx := c.Tx
	projectID := r.ProjectID
	to, err := departmentTx(ctx, tx, projectID, r.ToDepartmentID)
	if err != nil {
		return err
	}
	if to.Status != domain.StatusActive {
		return invalid("사용하지 않는 부서는 협업을 수락할 수 없습니다")
	}
	from, err := workTx(ctx, tx, projectID, r.FromWorkItemID)
	if err != nil {
		return err
	}
	var result domain.WorkItem
	var body string
	if a.ExistingWorkItemID != "" {
		if result, err = workTx(ctx, tx, projectID, a.ExistingWorkItemID); err != nil {
			return err
		}
		switch {
		case result.Kind != domain.WorkTask:
			return invalid("공동 목표는 협업 업무로 고를 수 없습니다")
		case result.DepartmentID != r.ToDepartmentID:
			return invalid("%s의 업무만 고를 수 있습니다", r.ToDepartmentName)
		case domain.WorkClosed(result.Status):
			return invalid("닫힌 업무는 협업 업무로 고를 수 없습니다")
		case result.ID == from.ID:
			return invalid("요청한 업무를 그대로 고를 수 없습니다")
		}
		var used int
		tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM collab_requests WHERE result_work_item_id = ?`, result.ID).Scan(&used)
		if used > 0 {
			return invalid("‘%s’는 이미 다른 협업 요청의 업무입니다", result.Title)
		}
		if r.Requirement == CollabRequired {
			if err := checkNoCycleTx(ctx, tx, r, result.ID); err != nil {
				return err
			}
		}
		body = fmt.Sprintf("기존 업무 ‘%s’로 수락", result.Title)
	} else {
		result = domain.WorkItem{ProjectID: projectID, Kind: domain.WorkTask, DepartmentID: to.ID, Title: a.Title,
			Description: a.Description, DoneCriteria: a.DoneCriteria, Priority: a.Priority, Categories: from.Categories, DueAt: r.DueAt}
		if strings.TrimSpace(result.Title) == "" {
			result.Title = fmt.Sprintf("%s 협업: %s", r.FromDepartmentName, from.Title)
		}
		if strings.TrimSpace(result.Description) == "" {
			result.Description = fmt.Sprintf("%s의 ‘%s’ 업무에서 요청했습니다.\n\n%s", r.FromDepartmentName, from.Title, r.Question)
		}
		if result.Priority == "" {
			result.Priority = from.Priority
		}
		if goal := workEventFor(from).GoalID; goal != "" {
			if g, err := workTx(ctx, tx, projectID, goal); err == nil && !domain.WorkClosed(g.Status) {
				result.ParentID = g.ID
			}
		}
		if err := normalizeWork(&result); err != nil {
			return err
		}
		if err := checkWorkLinks(ctx, tx, result, true); err != nil {
			return err
		}
		if result.ID, err = insertWorkTx(ctx, c, result, workOrigin{Seq: r.EventSequence, Automated: who.JobID != ""}); err != nil {
			return err
		}
		body = fmt.Sprintf("새 업무 ‘%s’ 등록", result.Title)
	}
	for _, v := range r.InputVersionIDs {
		// Material withdrawn since the request is no longer linked.
		if err := linkDocumentTx(ctx, tx, projectID, result, v); err != nil && !errors.Is(err, ErrInvalid) {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE collab_requests SET status = 'accepted', follow_up = '', decided_by = ?, decision_job_id = NULLIF(?, ''),
			decision_reason = ?, decided_at = ?, result_work_item_id = ?, updated_at = ? WHERE id = ?`,
		who.by(), who.JobID, strings.TrimSpace(a.Note), now, result.ID, now, r.ID); err != nil {
		return err
	}
	if note := strings.TrimSpace(a.Note); note != "" {
		body += ": " + note
	}
	return addCollabHistoryByTx(ctx, tx, projectID, r.ID, "accepted", body, r.ToDepartmentID, who.author(), now)
}

// AskCollab records the receiving department's question about a request;
// the asking department answers before it is decided.
func (db *DB) AskCollab(ctx context.Context, projectID, id, question string) (CollabRequest, error) {
	question = strings.TrimSpace(question)
	if question == "" {
		return CollabRequest{}, invalid("질문을 적어 주세요")
	}
	return db.changeCollab(ctx, projectID, id, func(c *Change, r CollabRequest, now string) error {
		if err := undecided(r); err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE collab_requests SET status = 'question', follow_up = ?, updated_at = ? WHERE id = ?`,
			question, now, r.ID); err != nil {
			return err
		}
		return addCollabHistoryTx(ctx, c.Tx, projectID, r.ID, "asked", question, r.ToDepartmentID, now)
	})
}

// AnswerCollab records the asking department's answer; the request waits
// for a decision again.
func (db *DB) AnswerCollab(ctx context.Context, projectID, id, answer string) (CollabRequest, error) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return CollabRequest{}, invalid("답변을 적어 주세요")
	}
	return db.changeCollab(ctx, projectID, id, func(c *Change, r CollabRequest, now string) error {
		if r.Status != CollabQuestion {
			return invalid("답할 질문이 없습니다")
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE collab_requests SET status = 'requested', follow_up = '', updated_at = ? WHERE id = ?`, now, r.ID); err != nil {
			return err
		}
		if err := addCollabHistoryTx(ctx, c.Tx, projectID, r.ID, "answered", answer, r.FromDepartmentID, now); err != nil {
			return err
		}
		return queueCollabRound2Tx(ctx, c, r, now)
	})
}

// queueCollabRound2Tx lets the lead AI decide once more when its round-1
// question was just answered. After a second question the person decides.
func queueCollabRound2Tx(ctx context.Context, c *Change, r CollabRequest, now string) error {
	var deliveryID string
	err := c.Tx.QueryRowContext(ctx, `SELECT id FROM event_deliveries WHERE event_sequence = ? AND department_id = ? AND purpose = 'collab_request'
		AND status = 'waiting_answer'`, r.EventSequence, r.ToDepartmentID).Scan(&deliveryID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	first, err := scanAIJob(c.Tx.QueryRowContext(ctx, `SELECT `+aiJobCols+` FROM work_ai_jobs j WHERE j.delivery_id = ? AND j.round = 1`, deliveryID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	id := NewID("aij")
	if _, err := c.Tx.ExecContext(ctx, `INSERT INTO work_ai_jobs (id, project_id, kind, lane, request_key, delivery_id, round, assignment_id,
		connection_id, model, status, input_snapshot, input_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 2, NULLIF(?, ''), NULLIF(?, ''), ?, 'pending', ?, ?, ?, ?)`,
		id, r.ProjectID, first.Kind, first.Lane, fmt.Sprintf("%s:%s:2", first.Kind, deliveryID), deliveryID,
		first.AssignmentID, first.ConnectionID, first.Model, string(first.InputSnapshot), first.InputHash, now, now); err != nil {
		return err
	}
	if _, err := c.Tx.ExecContext(ctx, `UPDATE event_deliveries SET status = 'processing', note = '', updated_at = ? WHERE id = ?`, now, deliveryID); err != nil {
		return err
	}
	return emitJobUpdate(c, r.ProjectID, id, JobPending)
}

// DeclineCollab records that the receiving department will not help. A
// declined required request still blocks completion until the person in
// charge releases or replaces it.
func (db *DB) DeclineCollab(ctx context.Context, projectID, id, reason string) (CollabRequest, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return CollabRequest{}, invalid("거절 이유가 필요합니다")
	}
	return db.changeCollab(ctx, projectID, id, func(c *Change, r CollabRequest, now string) error {
		if err := undecided(r); err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE collab_requests SET status = 'declined', follow_up = '', decided_by = 'user', decision_reason = ?,
			decided_at = ?, updated_at = ? WHERE id = ?`, reason, now, now, r.ID); err != nil {
			return err
		}
		return addCollabHistoryTx(ctx, c.Tx, projectID, r.ID, "declined", reason, r.ToDepartmentID, now)
	})
}

// CancelCollab withdraws a request that was not taken on. Like a decline,
// a cancelled required request keeps blocking until it is released or
// replaced.
func (db *DB) CancelCollab(ctx context.Context, projectID, id, reason string) (CollabRequest, error) {
	return db.changeCollab(ctx, projectID, id, func(c *Change, r CollabRequest, now string) error {
		if r.Status != CollabRequested && r.Status != CollabQuestion && r.Status != CollabDeclined {
			return invalid("수락했거나 이미 취소한 협업 요청은 취소할 수 없습니다")
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE collab_requests SET status = 'cancelled', follow_up = '', decided_at = ?, updated_at = ? WHERE id = ?`,
			now, now, r.ID); err != nil {
			return err
		}
		return addCollabHistoryTx(ctx, c.Tx, projectID, r.ID, "cancelled", reason, r.FromDepartmentID, now)
	})
}

// SetCollabRequirement marks a request required or back to reference.
// Releasing a required request needs a reason; making one required is
// refused when it would make work wait on itself. A done goal that gains
// an unmet required request is flagged for review, not reopened.
func (db *DB) SetCollabRequirement(ctx context.Context, projectID, id, requirement, reason string) (CollabRequest, error) {
	reason = strings.TrimSpace(reason)
	if requirement != CollabReference && requirement != CollabRequired {
		return CollabRequest{}, invalid("협업은 reference 또는 required입니다")
	}
	if requirement == CollabReference && reason == "" {
		return CollabRequest{}, invalid("필수 조건을 해제하는 이유가 필요합니다")
	}
	return db.changeCollab(ctx, projectID, id, func(c *Change, r CollabRequest, now string) error {
		if r.Requirement == requirement {
			return nil
		}
		if requirement == CollabRequired {
			if err := requireableTx(ctx, c.Tx, r); err != nil {
				return err
			}
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE collab_requests SET requirement = ?, requirement_reason = ?, updated_at = ? WHERE id = ?`,
			requirement, reason, now, r.ID); err != nil {
			return err
		}
		if err := addCollabHistoryTx(ctx, c.Tx, projectID, r.ID, requirement, reason, "", now); err != nil {
			return err
		}
		r.Requirement = requirement
		if requirement != CollabRequired || r.Satisfied {
			return nil
		}
		from, err := workTx(ctx, c.Tx, projectID, r.FromWorkItemID)
		if err != nil {
			return err
		}
		return flagDoneGoalTx(ctx, c, from, r)
	})
}

// requireableTx checks that r may become required.
func requireableTx(ctx context.Context, tx *sql.Tx, r CollabRequest) error {
	if domain.WorkClosed(r.FromWorkStatus) {
		return invalid("닫힌 업무의 협업은 필수로 바꿀 수 없습니다. 업무를 다시 연 뒤 지정하세요")
	}
	if r.DecidedBy == "ai" {
		return invalid("AI가 결정한 협업은 필수로 바꾸기 전에 사람이 다시 결정해야 합니다")
	}
	if r.ResultWorkItemID != "" && r.Status == CollabAccepted {
		return checkNoCycleTx(ctx, tx, r, r.ResultWorkItemID)
	}
	return nil
}

// ReplaceCollab hands a required request over to a new one (usually to
// another department after a decline): in one transaction the old request
// becomes reference with the reason and the new one is sent as required.
func (db *DB) ReplaceCollab(ctx context.Context, projectID, id string, n NewCollabRequest, reason string) (CollabRequest, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return CollabRequest{}, invalid("대체하는 이유가 필요합니다")
	}
	var newID string
	_, err := db.changeCollab(ctx, projectID, id, func(c *Change, r CollabRequest, now string) error {
		if r.Requirement != CollabRequired {
			return invalid("필수 협업만 대체할 수 있습니다")
		}
		if r.ReplacedByID != "" {
			return invalid("이미 대체한 협업 요청입니다")
		}
		n.FromWorkItemID, n.Requirement = r.FromWorkItemID, CollabRequired
		if n.RequirementReason == "" {
			n.RequirementReason = reason
		}
		var err error
		if newID, err = insertCollabTx(ctx, c, projectID, n, r.ID); err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE collab_requests SET requirement = 'reference', requirement_reason = ?, updated_at = ? WHERE id = ?`,
			reason, now, r.ID); err != nil {
			return err
		}
		next, err := collabTx(ctx, c.Tx, projectID, newID)
		if err != nil {
			return err
		}
		return addCollabHistoryTx(ctx, c.Tx, projectID, r.ID, "replaced", fmt.Sprintf("%s에 새 필수 요청: %s", next.ToDepartmentName, reason), "", now)
	})
	if err != nil {
		return CollabRequest{}, err
	}
	return db.CollabRequest(ctx, projectID, newID)
}

// RequiredNotice is an open required-condition impact check with what it
// is about, for the inbox.
type RequiredNotice struct {
	ImpactCheck
	ProjectName string `json:"projectName"`
	WorkTitle   string `json:"workTitle"`
	WorkKind    string `json:"workKind"`
}

// OpenRequiredNotices lists open required-condition checks of every
// active service, newest first.
func (db *DB) OpenRequiredNotices(ctx context.Context) ([]RequiredNotice, error) {
	rows, err := db.sql.QueryContext(ctx, `SELECT c.id, c.project_id, c.work_item_id, c.cause, c.ref_id, COALESCE(c.document_id, ''), c.summary,
		c.status, c.note, c.created_at, p.name, w.title, w.kind
		FROM impact_checks c JOIN projects p ON p.id = c.project_id JOIN work_items w ON w.id = c.work_item_id
		WHERE p.status = 'active' AND c.status = 'open' AND c.cause = ? ORDER BY c.updated_at DESC, c.rowid DESC`, CauseRequiredReopened)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RequiredNotice{}
	for rows.Next() {
		var n RequiredNotice
		if err := rows.Scan(&n.ID, &n.ProjectID, &n.WorkItemID, &n.Cause, &n.RefID, &n.DocumentID, &n.Summary, &n.Status, &n.Note, &n.CreatedAt,
			&n.ProjectName, &n.WorkTitle, &n.WorkKind); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// flagMovedUnderDoneGoalTx flags a done goal that w was just moved under
// when w carries required requests that are not met.
func flagMovedUnderDoneGoalTx(ctx context.Context, c *Change, w domain.WorkItem) error {
	unmet, err := unmetRequiredTx(ctx, c.Tx, w)
	if err != nil {
		return err
	}
	for _, r := range unmet {
		if err := flagDoneGoalTx(ctx, c, w, r); err != nil {
			return err
		}
	}
	return nil
}
