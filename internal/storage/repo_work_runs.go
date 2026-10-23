package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"agent-office/internal/domain"
)

// ---- work plans ----

// WorkPlan returns a work item's plan and whether one was saved.
func (db *DB) WorkPlan(ctx context.Context, projectID, workItemID string) (domain.WorkPlan, bool, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefWorkItem, workItemID}); err != nil {
		return domain.WorkPlan{}, false, err
	}
	return workPlanTx(ctx, db.sql, workItemID)
}

func workPlanTx(ctx context.Context, q queryer, workItemID string) (domain.WorkPlan, bool, error) {
	p := domain.WorkPlan{WorkItemID: workItemID}
	var outputs string
	err := q.QueryRowContext(ctx, `SELECT project_id, COALESCE(assignment_id, ''), instructions, materials, outputs, updated_at
		FROM work_plans WHERE work_item_id = ?`, workItemID).Scan(&p.ProjectID, &p.AssignmentID, &p.Instructions, &p.Materials, &outputs, &p.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return p, false, nil
	}
	if err != nil {
		return p, false, err
	}
	json.Unmarshal([]byte(outputs), &p.Outputs)
	if p.Outputs == nil {
		p.Outputs = []domain.Output{}
	}
	return p, true, nil
}

// SaveWorkPlan stores the plan of a work item. The assignee must be an
// assignment of the same service; outputs default to one Markdown result.
// Output keys and types are checked again when a run's version is made.
func (db *DB) SaveWorkPlan(ctx context.Context, p domain.WorkPlan) (domain.WorkPlan, error) {
	if len(p.Outputs) == 0 {
		p.Outputs = domain.DefaultWorkOutputs()
	}
	var keys []string
	for i, o := range p.Outputs {
		o.Key = strings.TrimSpace(o.Key)
		if o.Key == "" {
			return p, invalid("산출물 이름이 필요합니다")
		}
		if slices.Contains(keys, o.Key) {
			return p, invalid("산출물 이름 %q가 겹칩니다", o.Key)
		}
		keys = append(keys, o.Key)
		p.Outputs[i] = o
	}
	outputs, _ := json.Marshal(p.Outputs)
	err := db.Write(ctx, func(tx *sql.Tx) error {
		if err := writableProjectTx(ctx, tx, p.ProjectID); err != nil {
			return err
		}
		if _, err := workTx(ctx, tx, p.ProjectID, p.WorkItemID); err != nil {
			return err
		}
		if p.AssignmentID != "" {
			if err := CheckScopeTx(ctx, tx, p.ProjectID, Ref{RefAssignment, p.AssignmentID}); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx, `INSERT INTO work_plans (work_item_id, project_id, assignment_id, instructions, materials, outputs, updated_at)
			VALUES (?, ?, NULLIF(?, ''), ?, ?, ?, ?)
			ON CONFLICT(work_item_id) DO UPDATE SET assignment_id = excluded.assignment_id, instructions = excluded.instructions,
				materials = excluded.materials, outputs = excluded.outputs, updated_at = excluded.updated_at`,
			p.WorkItemID, p.ProjectID, p.AssignmentID, strings.TrimSpace(p.Instructions), strings.TrimSpace(p.Materials), string(outputs), Now())
		return err
	})
	if err != nil {
		return p, err
	}
	saved, _, err := db.WorkPlan(ctx, p.ProjectID, p.WorkItemID)
	return saved, err
}

// ComposeWorkSpec turns a work item and its plan into the one-step
// workflow its run executes. The step's instructions carry the work,
// its done criteria, the department's responsibility (when the assignee
// is not a member and so lacks the department layer), the plan and any
// extra text, such as a person's change request.
func (db *DB) ComposeWorkSpec(ctx context.Context, projectID, workItemID, extra string) (domain.WorkflowSpec, error) {
	w, err := db.WorkItem(ctx, projectID, workItemID)
	if err != nil {
		return domain.WorkflowSpec{}, err
	}
	if w.Kind != domain.WorkTask {
		return domain.WorkflowSpec{}, invalid("공동 목표는 직접 실행하지 않습니다. 목표 아래 업무를 실행하세요")
	}
	if domain.WorkClosed(w.Status) {
		return domain.WorkflowSpec{}, invalid("닫힌 업무는 실행할 수 없습니다. 먼저 다시 여세요")
	}
	plan, ok, err := db.WorkPlan(ctx, projectID, workItemID)
	if err != nil {
		return domain.WorkflowSpec{}, err
	}
	if !ok {
		return domain.WorkflowSpec{}, invalid("작업 계획을 먼저 저장하세요")
	}
	if plan.AssignmentID == "" {
		return domain.WorkflowSpec{}, invalid("작업 계획에 담당자를 정하세요")
	}
	section := func(title, body string) string {
		if body = strings.TrimSpace(body); body == "" {
			return ""
		}
		return "## " + title + "\n" + body
	}
	parts := []string{section("업무", strings.TrimSpace(w.Title+"\n\n"+w.Description)), section("완료 조건", w.DoneCriteria)}
	if w.DepartmentID != "" {
		d, err := db.Department(ctx, projectID, w.DepartmentID)
		if err != nil {
			return domain.WorkflowSpec{}, err
		}
		if !slices.Contains(d.MemberIDs, plan.AssignmentID) {
			parts = append(parts, section("부서 책임 ("+d.Name+")", d.Responsibility))
		}
	}
	parts = append(parts, section("작업 지침", plan.Instructions), section("참고 자료", plan.Materials), strings.TrimSpace(extra))
	parts = slices.DeleteFunc(parts, func(s string) bool { return s == "" })
	outputs := plan.Outputs
	if len(outputs) == 0 {
		outputs = domain.DefaultWorkOutputs()
	}
	return domain.WorkflowSpec{
		SchemaVersion: domain.WorkflowSchemaVersion,
		Title:         "업무: " + w.Title,
		Nodes: []domain.Node{{ID: domain.WorkStepID, Title: w.Title, Kind: domain.KindTask, AssignmentID: plan.AssignmentID,
			DependsOn: []string{}, Instructions: strings.Join(parts, "\n\n"), Outputs: outputs}},
	}, nil
}

// WorkVersion confirms spec as a new version of the work item's own
// workflow, creating that hidden workflow on first use. Every run gets a
// version of its own, so the record shows exactly what each run was
// asked to do.
func (db *DB) WorkVersion(ctx context.Context, projectID, workItemID string, spec domain.WorkflowSpec) (domain.WorkflowVersion, error) {
	draft, err := json.Marshal(spec)
	if err != nil {
		return domain.WorkflowVersion{}, err
	}
	var wfID string
	var rev int
	_, err = db.Change(ctx, func(c *Change) error {
		if err := writableProjectTx(ctx, c.Tx, projectID); err != nil {
			return err
		}
		if _, err := workTx(ctx, c.Tx, projectID, workItemID); err != nil {
			return err
		}
		err := c.Tx.QueryRowContext(ctx, `SELECT id, revision FROM workflows WHERE work_item_id = ? AND project_id = ?`, workItemID, projectID).Scan(&wfID, &rev)
		if err == nil {
			return nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		wfID, rev = NewID("wf"), 1
		now := Now()
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO workflows (id, project_id, title, draft_json, revision, work_item_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, 1, ?, ?, ?)`, wfID, projectID, spec.Title, string(draft), workItemID, now, now); err != nil {
			return err
		}
		return c.Emit(projectID, "", "", "workflow.created", map[string]string{"workflowId": wfID, "workItemId": workItemID})
	})
	if err != nil {
		return domain.WorkflowVersion{}, err
	}
	wf, err := db.SaveWorkflowDraft(ctx, projectID, wfID, rev, draft)
	if err != nil {
		return domain.WorkflowVersion{}, err
	}
	v, res, err := db.ConfirmVersion(ctx, projectID, wfID, wf.Revision)
	var nv *ErrNotVersionable
	if errors.As(err, &nv) && len(res.Issues) > 0 {
		return v, invalid("작업 계획을 실행할 수 없습니다: %s", res.Issues[0].Message)
	}
	return v, err
}

// ---- runs of work items ----

// StartWorkRunTx links a run being inserted in c to its work item and
// marks the item in progress. It refuses a closed item, another
// service's item and a second active run.
func StartWorkRunTx(ctx context.Context, c *Change, projectID, workItemID, runID string) error {
	w, err := workTx(ctx, c.Tx, projectID, workItemID)
	if err != nil {
		return err
	}
	if domain.WorkClosed(w.Status) {
		return invalid("닫힌 업무는 실행할 수 없습니다. 먼저 다시 여세요")
	}
	var active int
	c.Tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE work_item_id = ? AND id <> ? AND status IN ('running', 'waiting', 'paused')`,
		workItemID, runID).Scan(&active)
	if active > 0 {
		return invalid("이 업무는 이미 실행 중입니다. 끝나거나 취소된 뒤에 다시 실행하세요")
	}
	if _, err := c.Tx.ExecContext(ctx, `UPDATE runs SET work_item_id = ? WHERE id = ? AND project_id = ?`, workItemID, runID, projectID); err != nil {
		return err
	}
	return setWorkStatusTx(ctx, c, w, domain.WorkInProgress, "실행을 시작했습니다", "run:"+runID)
}

// workStatusForRun is the work status a run status leads to, with the
// reason kept in the item's history. A finished run waits for a person
// to check the result: success alone never completes work.
func workStatusForRun(runStatus string) (string, string) {
	switch runStatus {
	case "running":
		return domain.WorkInProgress, "실행 중입니다"
	case "waiting":
		return domain.WorkWaiting, "실행이 사람의 작업이나 승인을 기다립니다"
	case "paused":
		return domain.WorkWaiting, "실행이 일시 정지되었습니다"
	case "succeeded":
		return domain.WorkWaiting, "실행이 끝났습니다. 결과를 확인하고 완료하거나 수정을 요청하세요"
	case "failed":
		return domain.WorkBlocked, "실행이 실패했습니다. 실행 화면에서 원인을 확인하세요"
	case "interrupted":
		return domain.WorkBlocked, "실행이 중단되었습니다. 다시 시도하거나 새로 실행하세요"
	case "cancelled":
		return domain.WorkReady, "실행이 취소되었습니다"
	}
	return "", ""
}

// SyncWorkRunTx follows a run's new status on its work item, in the
// transaction that changed the run. Only the item's latest run counts,
// so a late status of an older run cannot move the item, and a closed
// item is left alone.
func SyncWorkRunTx(ctx context.Context, c *Change, runID, runStatus string) error {
	var projectID string
	var workID sql.NullString
	if err := c.Tx.QueryRowContext(ctx, `SELECT project_id, work_item_id FROM runs WHERE id = ?`, runID).Scan(&projectID, &workID); err != nil {
		return err
	}
	if !workID.Valid {
		return nil
	}
	var latest string
	if err := c.Tx.QueryRowContext(ctx, `SELECT id FROM runs WHERE work_item_id = ? ORDER BY started_at DESC, rowid DESC LIMIT 1`,
		workID.String).Scan(&latest); err != nil {
		return err
	}
	if latest != runID {
		return nil
	}
	w, err := workTx(ctx, c.Tx, projectID, workID.String)
	if err != nil {
		return err
	}
	status, reason := workStatusForRun(runStatus)
	if status == "" || domain.WorkClosed(w.Status) {
		return nil
	}
	if !domain.WorkNeedsReason(status) && w.Status == status {
		return nil
	}
	return setWorkStatusTx(ctx, c, w, status, reason, "run:"+runID)
}

// WorkRunIDs lists a work item's runs, newest first.
func (db *DB) WorkRunIDs(ctx context.Context, projectID, workItemID string) ([]string, error) {
	if err := db.CheckScope(ctx, projectID, Ref{RefWorkItem, workItemID}); err != nil {
		return nil, err
	}
	rows, err := db.sql.QueryContext(ctx, `SELECT id FROM runs WHERE project_id = ? AND work_item_id = ? ORDER BY started_at DESC, rowid DESC`,
		projectID, workItemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}
