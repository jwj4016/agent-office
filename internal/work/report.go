package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/providers"
)

// Completion reports (company requests, 5th revision). Once all the work a
// request registered is closed, its desk writes a report for the person:
// summary first, then what each piece of work produced, what is left and
// what the person has to decide. Without a usable desk, or after three
// failed calls, the app writes a plain report from the records so the
// request still ends.

const reportTries = 3

// drainReports writes the reports that are due; a held service waits.
func (in *Intake) drainReports(ctx context.Context) (int, error) {
	db := in.cfg.DB
	list, err := db.RequestsAwaitingReport(ctx, 10)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, r := range list {
		if ctx.Err() != nil {
			return n, ctx.Err()
		}
		if r, err = db.Request(ctx, r.ProjectID, r.ID); err != nil { // with its items
			return n, err
		}
		tries, err := db.ReportTries(ctx, r.ID)
		if err != nil {
			return n, err
		}
		works, err := in.requestWork(ctx, r)
		if err != nil {
			return n, err
		}
		if tries >= reportTries {
			if err := db.CompleteReport(ctx, r.ID, plainReport(works, "AI가 완료 보고를 쓰지 못해 앱이 기록으로 정리했습니다.")); err != nil {
				return n, err
			}
			n++
			continue
		}
		dc, status, why, err := in.gate(ctx, r)
		if err != nil {
			return n, err
		}
		if why != "" {
			if status == domain.ReqHeld {
				continue // the budget or an archived service: wait
			}
			if err := db.CompleteReport(ctx, r.ID, plainReport(works, "접수 담당이 없어 앱이 기록으로 정리했습니다.")); err != nil {
				return n, err
			}
			n++
			continue
		}
		if err := in.writeReport(ctx, r, dc, works); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// reportWork is one piece of a request's work, as the report sees it.
type reportWork struct {
	item    domain.WorkItem
	dept    string
	results []string
}

func (in *Intake) requestWork(ctx context.Context, r domain.Request) ([]reportWork, error) {
	db := in.cfg.DB
	depts, err := db.Departments(ctx, r.ProjectID)
	if err != nil {
		return nil, err
	}
	deptName := map[string]string{}
	for _, d := range depts {
		deptName[d.ID] = d.Name
	}
	var out []reportWork
	for _, it := range r.Items {
		if it.Kind != domain.ItemWork || it.WorkItemID == "" {
			continue
		}
		w, err := db.WorkItem(ctx, r.ProjectID, it.WorkItemID)
		if err != nil {
			return nil, err
		}
		rw := reportWork{item: w, dept: deptName[w.DepartmentID]}
		var runID string
		db.Read().QueryRowContext(ctx, `SELECT id FROM runs WHERE work_item_id = ? AND status = 'succeeded' ORDER BY ended_at DESC, rowid DESC LIMIT 1`, w.ID).Scan(&runID)
		if runID != "" {
			rows, err := db.Read().QueryContext(ctx, `SELECT id, output_key, type FROM artifacts WHERE run_id = ? AND validity = 'valid' ORDER BY created_at`, runID)
			if err != nil {
				return nil, err
			}
			type art struct{ id, key, typ string }
			var arts []art
			for rows.Next() {
				var a art
				rows.Scan(&a.id, &a.key, &a.typ)
				arts = append(arts, a)
			}
			rows.Close()
			for _, a := range arts {
				line := a.key
				if c, err := in.cfg.Engine.ReadArtifact(ctx, r.ProjectID, a.id); err == nil && !c.Binary {
					if s := resultSummary(a.typ, c.Content, 800); s != "" {
						line += ": " + strings.ReplaceAll(s, "\n", " ")
					}
				}
				rw.results = append(rw.results, line)
			}
		}
		out = append(out, rw)
	}
	return out, nil
}

var workStatusName = map[string]string{
	domain.WorkReceived: "접수", domain.WorkReady: "준비", domain.WorkInProgress: "진행", domain.WorkWaiting: "대기", domain.WorkBlocked: "막힘",
	domain.WorkDone: "완료", domain.WorkCancelled: "취소",
}

// plainReport is the report the app writes from the records.
func plainReport(works []reportWork, why string) string {
	done, cancelled := 0, 0
	var b strings.Builder
	for _, w := range works {
		switch w.item.Status {
		case domain.WorkDone:
			done++
		case domain.WorkCancelled:
			cancelled++
		}
	}
	fmt.Fprintf(&b, "## 요약\n요청에서 나온 업무 %d건이 끝났습니다(완료 %d건, 취소 %d건). %s\n\n## 업무\n", len(works), done, cancelled, why)
	for _, w := range works {
		fmt.Fprintf(&b, "- %s — %s", w.item.Title, workStatusName[w.item.Status])
		if w.dept != "" {
			fmt.Fprintf(&b, " (%s)", w.dept)
		}
		b.WriteString("\n")
		for _, r := range w.results {
			fmt.Fprintf(&b, "  - %s\n", clipText(r, 300))
		}
	}
	return b.String()
}

// writeReport has the desk write the report.
func (in *Intake) writeReport(ctx context.Context, r domain.Request, dc deskCall, works []reportWork) error {
	db := in.cfg.DB
	var b strings.Builder
	fmt.Fprintf(&b, "# 완료 보고: %s\n", r.ProjectName)
	b.WriteString("너는 이 서비스의 접수 담당이다. 대표가 맡긴 요청의 업무가 모두 끝났다. 대표에게 완료 보고를 쓴다. ")
	b.WriteString("아래 기록에 있는 것만 쓰고, 기록에 없는 결과는 지어내지 않는다. 결과물이 없는 업무는 결과가 없다고 쓴다.\n\n")
	fmt.Fprintf(&b, "## 요청\n%s\n", r.Body)
	if r.Reason != "" {
		fmt.Fprintf(&b, "접수 때 판단: %s\n", r.Reason)
	}
	if r.GoalTitle != "" {
		fmt.Fprintf(&b, "공동 목표: %s\n", r.GoalTitle)
	}
	b.WriteString("\n## 업무와 결과\n")
	for _, w := range works {
		fmt.Fprintf(&b, "- %s (상태: %s", w.item.Title, workStatusName[w.item.Status])
		if w.dept != "" {
			fmt.Fprintf(&b, ", 부서: %s", w.dept)
		}
		b.WriteString(")\n")
		if w.item.DoneCriteria != "" {
			fmt.Fprintf(&b, "  완료 조건: %s\n", clipText(w.item.DoneCriteria, 300))
		}
		if len(w.results) == 0 {
			b.WriteString("  결과물: 없음\n")
		}
		for _, res := range w.results {
			fmt.Fprintf(&b, "  결과물 %s\n", clipText(res, 900))
		}
	}
	b.WriteString(`
## 출력
결과 파일에 JSON 객체 하나를 쓴다.
{"summary": 대표가 먼저 읽을 요약 3~5문장(요청이 이뤄졌는지, 핵심 결과), "details": 업무별 결과, 남은 일, 대표가 결정하거나 확인할 것(짧은 목록)}
`)
	prompt := clipText(b.String(), intakePromptLimit)
	instructions := ""
	if layers, err := db.InstructionLayers(ctx, r.ProjectID, dc.assignmentID); err == nil {
		instructions = domain.ComposeInstructions(layers)
	}
	prov, err := in.cfg.Providers(dc.conn)
	if err != nil {
		return db.CompleteReport(ctx, r.ID, plainReport(works, "접수 담당의 AI 연결을 쓸 수 없어 앱이 기록으로 정리했습니다."))
	}
	id, err := db.StartReport(ctx, r, dc.conn.ID, dc.model)
	if err != nil {
		return err
	}
	str := map[string]any{"type": "string"}
	schema, _ := json.Marshal(map[string]any{"type": "object", "required": []string{"summary", "details"},
		"properties": map[string]any{"summary": str, "details": str}})
	raw, err := in.cfg.Call(ctx, prov, design.JSONCall{What: dc.desk.Name + " 보고 AI", Instructions: instructions, Prompt: prompt, Model: dc.model,
		Schema: schema, Timeout: 10 * time.Minute, Answer: "질문하지 말고 주어진 기록으로 쓰세요.",
		OnEvent: func(ev providers.Event) error {
			if ev.Kind == providers.KindUsage {
				return db.RecordReportUsage(context.Background(), id, ev.Payload)
			}
			return nil
		}})
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// Details may come back as text, a list or an object (models do not
	// always keep to the schema); each is written as a readable list.
	var out struct {
		Summary json.RawMessage `json:"summary"`
		Details json.RawMessage `json:"details"`
	}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	summary, details := strings.TrimSpace(renderReport(out.Summary, 0)), strings.TrimSpace(renderReport(out.Details, 0))
	if err == nil && summary == "" {
		err = errors.New("요약이 비어 있습니다")
	}
	if err != nil {
		return db.FinishReport(ctx, id, err.Error())
	}
	if err := db.FinishReport(ctx, id, ""); err != nil {
		return err
	}
	text := "## 요약\n" + summary
	if details != "" {
		text += "\n\n## 상세\n" + details
	}
	return db.CompleteReport(ctx, r.ID, text)
}

// renderReport writes a report field as Markdown: text as it is, a list
// as bullets, an object as labelled bullets, nested by indent.
func renderReport(raw json.RawMessage, depth int) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	indent := strings.Repeat("  ", depth)
	var b strings.Builder
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) == nil {
		for _, x := range list {
			if isScalar(x) {
				if text := strings.TrimSpace(renderReport(x, 0)); text != "" {
					fmt.Fprintf(&b, "%s- %s\n", indent, text)
				}
			} else if text := strings.TrimRight(renderReport(x, depth+1), "\n"); strings.TrimSpace(text) != "" {
				fmt.Fprintf(&b, "%s-\n%s\n", indent, text)
			}
		}
		return b.String()
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) == nil {
		keys := orderedKeys(raw)
		for _, k := range keys {
			v := obj[k]
			if isScalar(v) {
				fmt.Fprintf(&b, "%s- %s: %s\n", indent, k, strings.TrimSpace(renderReport(v, 0)))
			} else {
				fmt.Fprintf(&b, "%s- %s\n%s", indent, k, renderReport(v, depth+1))
			}
		}
		return b.String()
	}
	return strings.Trim(string(raw), `"`)
}

func isScalar(raw json.RawMessage) bool {
	t := strings.TrimSpace(string(raw))
	return t == "" || (t[0] != '{' && t[0] != '[')
}

// orderedKeys returns an object's keys in the order they were written.
func orderedKeys(raw json.RawMessage) []string {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	if _, err := dec.Token(); err != nil {
		return nil
	}
	var keys []string
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return keys
		}
		if k, ok := t.(string); ok {
			keys = append(keys, k)
		}
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return keys
		}
	}
	return keys
}
