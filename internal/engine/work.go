package engine

import (
	"context"
	"strings"
	"unicode/utf8"

	"agent-office/internal/domain"
)

// maxPreviousResult bounds the earlier result quoted in a change request.
const maxPreviousResult = 16 << 10

// StartWork runs a work item's plan as a one-step workflow version of its
// own and returns the run id. With feedback, the step is also given the
// person's change request and the latest result it is about, so a new
// run can rework it (service operations plan §4.6, stage 3).
func (e *Engine) StartWork(ctx context.Context, projectID, workItemID, feedback string) (string, error) {
	extra := ""
	if feedback = strings.TrimSpace(feedback); feedback != "" {
		extra = "## 이전 결과에 대한 수정 요청\n" + feedback
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
	return e.startRun(ctx, projectID, v.ID, workItemID)
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
