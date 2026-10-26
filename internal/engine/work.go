package engine

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// maxPreviousResult bounds the earlier result quoted in a change request.
const maxPreviousResult = 16 << 10

// StartWork runs a work item's plan as a one-step workflow version of its
// own and returns the run id. With feedback, the step is also given the
// person's change request and the latest result it is about, so a new
// run can rework it (service operations plan §4.6, stage 3). The shared
// documents the work links are given to the step and pinned to the run.
//
// A plan that names a confirmed version of one of the service's workflows
// runs that version as it is; it takes no change request.
func (e *Engine) StartWork(ctx context.Context, projectID, workItemID, feedback string) (string, error) {
	plan, ok, err := e.db.WorkPlan(ctx, projectID, workItemID)
	if err != nil {
		return "", err
	}
	if ok && plan.WorkflowVersionID != "" {
		if strings.TrimSpace(feedback) != "" {
			return "", fmt.Errorf("%w: 기존 업무 흐름으로 실행하는 업무는 수정 요청을 받지 않습니다. 흐름을 고치거나 작업 계획으로 바꾸세요", storage.ErrInvalid)
		}
		if _, err := e.db.WorkflowChoice(ctx, projectID, plan.WorkflowVersionID); err != nil {
			return "", err
		}
		return e.startRun(ctx, projectID, plan.WorkflowVersionID, workItemID, nil)
	}
	extra, pinned, err := e.WorkDocumentsSection(ctx, projectID, workItemID)
	if err != nil {
		return "", err
	}
	if feedback = strings.TrimSpace(feedback); feedback != "" {
		extra += "\n\n## 이전 결과에 대한 수정 요청\n" + feedback
		if prev := e.previousWorkResult(ctx, projectID, workItemID); prev != "" {
			extra += "\n\n## 이전 결과\n" + prev
		}
	}
	spec, err := e.db.ComposeWorkSpec(ctx, projectID, workItemID, extra)
	if err != nil {
		return "", err
	}
	v, err := e.db.WorkVersion(ctx, projectID, workItemID, spec)
	if err != nil {
		return "", err
	}
	return e.startRun(ctx, projectID, v.ID, workItemID, pinned)
}

// previousWorkResult is the text of the latest run's valid results, or ""
// when there is none to quote.
func (e *Engine) previousWorkResult(ctx context.Context, projectID, workItemID string) string {
	runs, err := e.db.WorkRunIDs(ctx, projectID, workItemID)
	if err != nil || len(runs) == 0 {
		return ""
	}
	d, err := e.RunDetail(ctx, projectID, runs[0])
	if err != nil {
		return ""
	}
	var b strings.Builder
	for _, a := range d.Step(domain.WorkStepID).Artifacts {
		if a.Validity != "valid" {
			continue
		}
		c, err := e.ReadArtifact(ctx, projectID, a.ID)
		if err != nil || c.Binary {
			continue
		}
		b.WriteString("### " + a.OutputKey + "\n" + c.Content + "\n\n")
		if b.Len() > maxPreviousResult {
			cut := b.String()[:maxPreviousResult]
			for !utf8.ValidString(cut) { // do not split a character
				cut = cut[:len(cut)-1]
			}
			return cut + "\n…(이하 생략)"
		}
	}
	return strings.TrimSpace(b.String())
}

// WorkRuns lists a work item's runs, newest first.
func (e *Engine) WorkRuns(ctx context.Context, projectID, workItemID string) ([]RunDetail, error) {
	ids, err := e.db.WorkRunIDs(ctx, projectID, workItemID)
	if err != nil {
		return nil, err
	}
	out := []RunDetail{}
	for _, id := range ids {
		d, err := e.RunDetail(ctx, projectID, id)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, nil
}

// ShareRunResults shares every valid result of a run as a draft of the
// run's work item, one document per result (a new version when the work
// already shared that result). causeSeq is the event the work came from:
// these shares are automatic and continue its chain. A result identical
// to the last version is skipped. It reports how many were shared.
func (e *Engine) ShareRunResults(ctx context.Context, projectID, runID string, causeSeq int64) (int, error) {
	var workID string
	if err := e.db.Read().QueryRowContext(ctx, `SELECT COALESCE(work_item_id, '') FROM runs WHERE id = ? AND project_id = ?`, runID, projectID).Scan(&workID); err != nil {
		return 0, err
	}
	if workID == "" {
		return 0, fmt.Errorf("%w: 업무의 실행이 아닙니다", storage.ErrInvalid)
	}
	w, err := e.db.WorkItem(ctx, projectID, workID)
	if err != nil {
		return 0, err
	}
	d, err := e.RunDetail(ctx, projectID, runID)
	if err != nil {
		return 0, err
	}
	docs, err := e.db.Documents(ctx, projectID, storage.DocumentFilter{IncludeWithdrawn: false})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, st := range d.Steps {
		for _, a := range st.Artifacts {
			if a.Validity != "valid" {
				continue
			}
			nv := storage.NewDocumentVersion{Title: w.Title + " · " + a.OutputKey, Categories: w.Categories, OwnerDepartmentID: w.DepartmentID,
				WorkItemID: w.ID, CauseSequence: causeSeq}
			for _, doc := range docs {
				if doc.WorkItemID == w.ID && doc.Title == nv.Title {
					nv.DocumentID = doc.ID
				}
			}
			if _, err := e.ShareArtifact(ctx, projectID, a.ID, nv); err != nil {
				if errors.Is(err, storage.ErrInvalid) {
					continue // unchanged since the last version, or not shareable
				}
				return n, err
			}
			n++
		}
	}
	return n, nil
}