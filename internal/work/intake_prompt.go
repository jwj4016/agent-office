package work

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/storage"
)

// What a desk is given (plan §4.3): the app writes it down, and the desk
// reads nothing else. intakeFacts keeps the ids it was shown, so a
// decision naming anything else is refused.
type intakeFacts struct {
	services    map[string]string // id → name (company desk)
	departments map[string]string
	assignees   map[string]string
	goals       map[string]string
	categories  []string
	codeRuns    map[string]string // succeeded runs with code: id → title
	results     map[string]string // artifact id → label
	versions    map[string]string // confirmed workflow versions: id → label
}

// Bounds of what a desk is given.
const (
	intakePromptLimit   = 40 << 10
	intakeWorkShown     = 20
	intakeRunsShown     = 10
	intakeCodeChecked   = 5
	intakeResultsShown  = 30
	intakeVersionsShown = 10
	intakeMaxRoutes     = 3
	intakeMaxActions    = 3
	intakeMaxItems      = 5
)

// prepare writes the desk's prompt. A long input is cut down step by
// step (shorter result summaries, then none) and finally clipped, saying
// so.
func (in *Intake) prepare(ctx context.Context, r domain.Request, dc deskCall) (design.JSONCall, intakeFacts, error) {
	var prompt, instructions string
	var facts intakeFacts
	var err error
	for _, summary := range []int{1000, 300, 0} {
		if r.ProjectID == "" {
			prompt, facts, err = in.companyPrompt(ctx, r, dc)
		} else {
			prompt, facts, err = in.servicePrompt(ctx, r, dc, summary)
		}
		if err != nil {
			return design.JSONCall{}, facts, err
		}
		if len(prompt) <= intakePromptLimit || r.ProjectID == "" {
			break
		}
	}
	if len(prompt) > intakePromptLimit {
		prompt = clipText(prompt, intakePromptLimit) + "\n…(입력이 길어 여기까지만 적었습니다)\n" + outputSpec(r.ProjectID == "", facts)
	}
	if r.ProjectID != "" {
		layers, err := in.cfg.DB.InstructionLayers(ctx, r.ProjectID, dc.assignmentID)
		if err != nil {
			return design.JSONCall{}, facts, err
		}
		instructions = domain.ComposeInstructions(layers)
	} else if org, err := in.cfg.DB.Organization(ctx); err == nil {
		instructions = org.Instructions
	}
	return design.JSONCall{What: dc.desk.Name + " 접수 AI", Instructions: instructions, Prompt: prompt, Schema: intakeSchema(r.ProjectID == ""),
		Timeout: 10 * time.Minute, Answer: "질문하지 말고, 꼭 필요하면 outcome을 question으로 하세요."}, facts, nil
}

func writeRequest(b *strings.Builder, ctx context.Context, db *storage.DB, r domain.Request) {
	fmt.Fprintf(b, "## 요청\n%s\n\n", r.Body)
	if r.PreviousID != "" {
		if prev, err := db.Request(ctx, r.ProjectID, r.PreviousID); err == nil {
			fmt.Fprintf(b, "## 이어서 하는 앞 요청\n%s\n결과: %s\n", prev.Body, requestSummary(prev))
			b.WriteString("\n")
		}
	}
	if r.Round == 2 {
		fmt.Fprintf(b, "## 앞서 한 질문과 사람의 답\n질문: %s\n답: %s\n이번에는 이 답을 바탕으로 정한다. 다시 묻는 것은 정말 필요할 때만 한다.\n\n", r.Question, r.Reply)
	}
}

// requestSummary says in a line what came of an earlier request.
func requestSummary(r domain.Request) string {
	switch {
	case r.Answer != "":
		return "답변 — " + clipText(r.Answer, 600)
	case r.Reason != "":
		return clipText(r.Reason, 600)
	}
	var parts []string
	for _, it := range r.Items {
		var p struct {
			Title  string `json:"title"`
			Action string `json:"action"`
		}
		json.Unmarshal(it.Payload, &p)
		parts = append(parts, fmt.Sprintf("%s %s(%s)", it.Kind, p.Title+p.Action, it.Status))
	}
	if len(parts) == 0 {
		return r.Status
	}
	return strings.Join(parts, ", ")
}

func (in *Intake) companyPrompt(ctx context.Context, r domain.Request, dc deskCall) (string, intakeFacts, error) {
	db := in.cfg.DB
	facts := intakeFacts{services: map[string]string{}}
	var b strings.Builder
	fmt.Fprintf(&b, "# 회사 접수: %s\n", dc.desk.Name)
	b.WriteString("너는 이 회사의 접수 담당이다. 대표가 회사에 한 요청을 읽고, 어느 서비스의 일인지 가려 그 서비스의 접수 담당에게 넘긴다. ")
	b.WriteString("직접 일을 하지 않는다. 서비스 안에서 무엇을 할지는 서비스의 접수 담당이 정한다.\n\n")
	if dc.desk.Instructions != "" {
		fmt.Fprintf(&b, "## 접수 담당 지침\n%s\n\n", dc.desk.Instructions)
	}
	rows, err := db.Read().QueryContext(ctx, `SELECT p.id, p.name, p.goal, (SELECT COUNT(*) FROM intake_desks d WHERE d.project_id = p.id AND d.enabled = 1),
		COALESCE((SELECT group_concat(name, ', ') FROM departments d WHERE d.project_id = p.id AND d.status = 'active'), '')
		FROM projects p WHERE p.status = 'active' ORDER BY p.name`)
	if err != nil {
		return "", facts, err
	}
	b.WriteString("## 서비스 목록\n")
	for rows.Next() {
		var id, name, goal, depts string
		var desk int
		if err := rows.Scan(&id, &name, &goal, &desk, &depts); err != nil {
			rows.Close()
			return "", facts, err
		}
		facts.services[id] = name
		deskText := "있음"
		if desk == 0 {
			deskText = "없음(넘기면 접수 담당이 정해질 때까지 기다림)"
		}
		fmt.Fprintf(&b, "- id: %s, 이름: %s, 접수 담당: %s, 부서: %s\n  목표: %s\n", id, name, deskText, orNone(depts), orNone(clipText(goal, 300)))
	}
	rows.Close()
	if len(facts.services) == 0 {
		b.WriteString("없음\n")
	}
	b.WriteString("\n")
	writeRequest(&b, ctx, db, r)
	b.WriteString(outputSpec(true, facts))
	return b.String(), facts, nil
}

func (in *Intake) servicePrompt(ctx context.Context, r domain.Request, dc deskCall, summaryLen int) (string, intakeFacts, error) {
	db, eng := in.cfg.DB, in.cfg.Engine
	facts := intakeFacts{departments: map[string]string{}, assignees: map[string]string{}, goals: map[string]string{},
		codeRuns: map[string]string{}, results: map[string]string{}, versions: map[string]string{}}
	p, err := db.Project(ctx, r.ProjectID)
	if err != nil {
		return "", facts, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# 서비스 접수: %s\n", p.Name)
	fmt.Fprintf(&b, "너는 서비스 ‘%s’의 접수 담당(%s)이다. 대표의 요청을 읽고 처리 방법을 정한다. 직접 코드를 쓰거나 파일을 만들지 않는다.\n\n", p.Name, dc.desk.Name)
	b.WriteString(`## 처리 방법
- answer: 아래 정보만으로 답할 수 있는 질문에 답한다. 아래에 없는 내용은 지어내지 않는다. 답하려면 결과 파일을 읽어야 하면 그 일을 업무로 제안한다.
- work: 담당자가 해야 할 일을 업무로 제안한다. 부서나 담당자, 공동 목표를 정할 수 있으면 아래 목록의 id를 쓴다.
- action: 앱이 정해진 방식으로 하는 일(아래 ‘쓸 수 있는 조치’)을 제안한다. 대표가 승인해야 실행된다. 조치 뒤에 할 업무가 있으면 work 배열에 함께 넣는다.
- question: 꼭 필요한 정보가 없을 때만 한 가지를 묻는다.
- decline: 이 앱으로 할 수 없는 일이다. reason에 이유와 대표가 직접 할 수 있는 일을 적는다.
`)
	if dc.desk.Instructions != "" {
		fmt.Fprintf(&b, "\n## 접수 담당 지침\n%s\n", dc.desk.Instructions)
	}
	mode := "검토 모드(제안한 업무는 대표가 등록)"
	if p.Mode == domain.ModeAuto {
		mode = "자동 모드(제안한 업무가 바로 등록됨)"
	}
	folder := "없음(앱이 관리하는 저장소에서 작업)"
	if p.WorkspacePath != "" {
		folder = p.WorkspacePath
	}
	fmt.Fprintf(&b, "\n## 서비스\n목표: %s\n지침: %s\n모드: %s\n작업 폴더: %s\n", orNone(clipText(p.Goal, 1000)), orNone(clipText(p.Instructions, 2000)), mode, folder)

	depts, err := db.Departments(ctx, r.ProjectID)
	if err != nil {
		return "", facts, err
	}
	b.WriteString("\n## 부서\n")
	shown := 0
	for _, d := range depts {
		if d.Status != domain.StatusActive {
			continue
		}
		facts.departments[d.ID] = d.Name
		fmt.Fprintf(&b, "- id: %s, 이름: %s, 책임: %s\n", d.ID, d.Name, orNone(clipText(d.Responsibility, 200)))
		shown++
	}
	if shown == 0 {
		b.WriteString("없음(업무는 부서 없이 만든다)\n")
	}
	people, err := db.Assignments(ctx, r.ProjectID)
	if err != nil {
		return "", facts, err
	}
	b.WriteString("\n## 담당자\n")
	for _, a := range people {
		facts.assignees[a.ID] = a.DisplayName
		kind := "AI"
		if a.ActorKind != domain.ActorAI {
			kind = "사람"
		}
		fmt.Fprintf(&b, "- id: %s, 이름: %s (%s)\n", a.ID, a.DisplayName, kind)
	}
	if len(people) == 0 {
		b.WriteString("없음\n")
	}

	open, err := db.WorkItems(ctx, r.ProjectID, storage.WorkFilter{Status: "open"})
	if err != nil {
		return "", facts, err
	}
	b.WriteString("\n## 진행 중인 업무와 공동 목표\n")
	if len(open) == 0 {
		b.WriteString("없음\n")
	}
	for i, w := range open {
		if w.Kind == domain.WorkGoal {
			facts.goals[w.ID] = w.Title
			fmt.Fprintf(&b, "- 공동 목표 id: %s, %s (%s)\n", w.ID, w.Title, w.Status)
		} else if i < intakeWorkShown {
			fmt.Fprintf(&b, "- 업무: %s (%s, 부서 %s)\n", w.Title, w.Status, orNone(facts.departments[w.DepartmentID]))
		}
	}

	runs, err := eng.Runs(ctx, r.ProjectID)
	if err != nil {
		return "", facts, err
	}
	b.WriteString("\n## 최근 실행\n")
	if len(runs) == 0 {
		b.WriteString("없음\n")
	}
	checked := 0
	for i, run := range runs {
		if i >= intakeRunsShown {
			break
		}
		code := ""
		if run.Status == engine.RunSucceeded && run.Repo != nil && run.Repo.Kind == engine.WsBase && checked < intakeCodeChecked {
			checked++
			if fc, err := eng.FinalCode(ctx, r.ProjectID, run.ID); err == nil && len(fc.Steps) > 0 {
				facts.codeRuns[run.ID] = run.Title
				switch {
				case fc.Included:
					code = ", 코드 결과: 서비스 기준 코드에 들어 있음"
				default:
					code = ", 코드 결과: 아직 서비스 기준 코드에 들어 있지 않음(이 코드를 이어서 고치는 업무 전에 deliver_code가 먼저 필요)"
				}
				if len(fc.Deliveries) > 0 {
					code += ", 전달 기록 있음"
				}
			}
		}
		fmt.Fprintf(&b, "- id: %s, %s v%d, 상태: %s%s\n", run.ID, run.Title, run.VersionNumber, run.Status, code)
	}

	if err := in.writeResults(ctx, &b, r.ProjectID, summaryLen, &facts); err != nil {
		return "", facts, err
	}

	vrows, err := db.Read().QueryContext(ctx, `SELECT wv.id, wv.number, COALESCE(json_extract(wv.spec_json, '$.title'), w.title)
		FROM workflow_versions wv JOIN workflows w ON w.id = wv.workflow_id
		WHERE wv.project_id = ? AND w.work_item_id IS NULL ORDER BY wv.created_at DESC LIMIT ?`, r.ProjectID, intakeVersionsShown)
	if err != nil {
		return "", facts, err
	}
	b.WriteString("\n## 확정된 업무 흐름\n")
	for vrows.Next() {
		var id, title string
		var n int
		if err := vrows.Scan(&id, &n, &title); err != nil {
			vrows.Close()
			return "", facts, err
		}
		facts.versions[id] = fmt.Sprintf("%s v%d", title, n)
		fmt.Fprintf(&b, "- versionId: %s, %s v%d\n", id, title, n)
	}
	vrows.Close()
	if len(facts.versions) == 0 {
		b.WriteString("없음\n")
	}

	b.WriteString(`
## 쓸 수 있는 조치
- deliver_code: 성공한 실행의 코드를 서비스 기준 코드로 반영하고 대표에게 넘긴다(폴더로 꺼내기나 결과 브랜치). runId에 ‘최근 실행’ 중 코드 결과가 있는 실행의 id.
- save_result: 결과 하나를 대표가 고른 위치에 파일로 저장한다. artifactId에 ‘최근 결과’의 id.
- export_service: 서비스 전체를 ZIP으로 내보낸다(키 제외).
- start_workflow: 확정된 업무 흐름을 실행한다. AI 비용이 든다. versionId에 ‘확정된 업무 흐름’의 id.
코드를 이어서 고치는 업무를 제안할 때 마지막 코드 결과가 아직 서비스 기준 코드에 들어 있지 않으면, deliver_code를 먼저 제안하고 업무는 work 배열에 함께 넣는다.

`)
	if facts.categories, err = db.ProjectCategories(ctx, r.ProjectID); err != nil {
		return "", facts, err
	}
	writeRequest(&b, ctx, db, r)
	b.WriteString(outputSpec(false, facts))
	return b.String(), facts, nil
}

// writeResults lists the latest results with what they say about
// themselves (a summary field of a report, the start of a document).
func (in *Intake) writeResults(ctx context.Context, b *strings.Builder, projectID string, summaryLen int, facts *intakeFacts) error {
	rows, err := in.cfg.DB.Read().QueryContext(ctx, `SELECT a.id, a.run_id, s.step_id, a.output_key, a.type, a.size
		FROM artifacts a JOIN step_attempts s ON s.id = a.step_attempt_id
		WHERE a.project_id = ? AND a.validity = 'valid' ORDER BY a.created_at DESC, a.rowid DESC LIMIT ?`, projectID, intakeResultsShown)
	if err != nil {
		return err
	}
	type res struct {
		id, run, step, key, typ string
		size                    int64
	}
	var list []res
	for rows.Next() {
		var x res
		if err := rows.Scan(&x.id, &x.run, &x.step, &x.key, &x.typ, &x.size); err != nil {
			rows.Close()
			return err
		}
		list = append(list, x)
	}
	rows.Close()
	b.WriteString("\n## 최근 결과\n")
	if len(list) == 0 {
		b.WriteString("없음\n")
	}
	for _, x := range list {
		facts.results[x.id] = fmt.Sprintf("%s.%s", x.step, x.key)
		fmt.Fprintf(b, "- id: %s, 실행 %s, 단계 %s, 출력 %s, 형식 %s, %d바이트\n", x.id, x.run, x.step, x.key, x.typ, x.size)
		if summaryLen == 0 || x.typ == string(domain.OutDiagram) {
			continue
		}
		c, err := in.cfg.Engine.ReadArtifact(ctx, projectID, x.id)
		if err != nil || c.Binary {
			continue
		}
		if s := resultSummary(x.typ, c.Content, summaryLen); s != "" {
			fmt.Fprintf(b, "  요약: %s\n", strings.ReplaceAll(s, "\n", " "))
		}
	}
	return nil
}

// resultSummary is a result's own summary field, or the start of a
// document.
func resultSummary(typ, content string, n int) string {
	if typ == string(domain.OutMarkdown) || typ == string(domain.OutFile) {
		return clipText(strings.TrimSpace(content), n)
	}
	var m map[string]any
	if json.Unmarshal([]byte(content), &m) != nil {
		return ""
	}
	for _, k := range []string{"ceo_summary", "summary", "verdict"} {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			return clipText(strings.TrimSpace(s), n)
		}
	}
	return ""
}

func outputSpec(company bool, f intakeFacts) string {
	if company {
		return fmt.Sprintf(`## 출력
결과 파일에 JSON 객체 하나를 쓴다.
{"outcome": "answer" | "route" | "question" | "decline", "reason": 판단 이유,
 "answer": outcome이 answer일 때 대표에게 줄 답,
 "question": outcome이 question일 때 대표에게 물을 한 가지,
 "routes": [{"projectId": ‘서비스 목록’의 id, "body": 그 서비스 접수 담당에게 넘길 요청(그 서비스에 맞게 고쳐 쓴다), "reason": 왜 이 서비스의 일인지}]}
- 서비스의 일이면 route. 여러 서비스의 일이면 서비스마다 하나씩, %d개까지.
- 회사 전체에 관한 질문을 위 정보로 답할 수 있으면 answer.
- 어느 서비스의 일인지 알 수 없을 때만 question.
- 이 앱으로 할 수 없는 일은 decline.
`, intakeMaxRoutes)
	}
	return fmt.Sprintf(`## 출력
결과 파일에 JSON 객체 하나를 쓴다.
{"outcome": "answer" | "work" | "action" | "question" | "decline", "reason": 판단 이유(대표가 읽는다),
 "answer": outcome이 answer일 때 대표에게 줄 답,
 "question": outcome이 question일 때 대표에게 물을 한 가지,
 "actions": [{"action": "deliver_code" | "save_result" | "export_service" | "start_workflow", "runId": …, "artifactId": …, "versionId": …, "reason": 왜 이 조치인지}],
 "work": [{"title": 업무 제목, "description": 할 일, "doneCriteria": 완료 조건, "categories": [%s 중에서], "priority": "urgent"|"high"|"normal"|"low",
           "departmentId": ‘부서’의 id(없으면 비움), "assigneeAssignmentId": ‘담당자’의 id(없으면 비움), "goalId": ‘공동 목표’의 id(없으면 비움)}]}
- 조치는 %d개까지, 조치와 업무를 합쳐 %d개까지. id는 위 목록에 있는 것만 쓴다.
- outcome이 action이면 actions가, work이면 work가 하나 이상 있어야 한다.
`, strings.Join(f.categories, ", "), intakeMaxActions, intakeMaxItems)
}

func intakeSchema(company bool) json.RawMessage {
	str := map[string]any{"type": "string"}
	props := map[string]any{"reason": str, "answer": str, "question": str}
	if company {
		props["outcome"] = map[string]any{"type": "string", "enum": []string{"answer", "route", "question", "decline"}}
		props["routes"] = map[string]any{"type": "array", "maxItems": intakeMaxRoutes, "items": map[string]any{"type": "object", "required": []string{"projectId"},
			"properties": map[string]any{"projectId": str, "body": str, "reason": str}}}
	} else {
		props["outcome"] = map[string]any{"type": "string", "enum": []string{"answer", "work", "action", "question", "decline"}}
		props["actions"] = map[string]any{"type": "array", "maxItems": intakeMaxActions, "items": map[string]any{"type": "object", "required": []string{"action"},
			"properties": map[string]any{"action": map[string]any{"type": "string", "enum": domain.Actions}, "runId": str, "artifactId": str, "versionId": str, "reason": str}}}
		props["work"] = map[string]any{"type": "array", "maxItems": intakeMaxItems, "items": map[string]any{"type": "object", "required": []string{"title"},
			"properties": map[string]any{"title": str, "description": str, "doneCriteria": str,
				"categories": map[string]any{"type": "array", "items": str}, "priority": map[string]any{"type": "string", "enum": domain.WorkPriorities},
				"departmentId": str, "assigneeAssignmentId": str, "goalId": str}}}
	}
	b, _ := json.Marshal(map[string]any{"type": "object", "required": []string{"outcome", "reason"}, "properties": props})
	return b
}

// acceptIntake checks a desk's answer against what it was shown.
func acceptIntake(r domain.Request, f intakeFacts, raw []byte) (domain.IntakeDecision, error) {
	var d domain.IntakeDecision
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("접수 판단을 읽을 수 없습니다: %w", err)
	}
	d.Reason, d.Answer, d.Question = strings.TrimSpace(d.Reason), strings.TrimSpace(d.Answer), strings.TrimSpace(d.Question)
	company := r.ProjectID == ""
	allowed := []string{domain.OutcomeAnswer, domain.OutcomeWork, domain.OutcomeAction, domain.OutcomeQuestion, domain.OutcomeDecline}
	if company {
		allowed = []string{domain.OutcomeAnswer, domain.OutcomeRoute, domain.OutcomeQuestion, domain.OutcomeDecline}
	}
	if !slices.Contains(allowed, d.Outcome) {
		return d, fmt.Errorf("이 접수 담당이 쓸 수 없는 판단입니다: %q", d.Outcome)
	}
	switch d.Outcome {
	case domain.OutcomeAnswer:
		if d.Answer == "" {
			return d, errors.New("답하기로 했지만 답이 없습니다")
		}
		d.Routes, d.Work, d.Actions, d.Question = nil, nil, nil, ""
		return d, nil
	case domain.OutcomeQuestion:
		if d.Question == "" {
			return d, errors.New("묻기로 했지만 질문이 없습니다")
		}
		d.Routes, d.Work, d.Actions, d.Answer = nil, nil, nil, ""
		return d, nil
	case domain.OutcomeDecline:
		if d.Reason == "" {
			return d, errors.New("처리할 수 없다고 했지만 이유가 없습니다")
		}
		d.Routes, d.Work, d.Actions = nil, nil, nil
		return d, nil
	case domain.OutcomeRoute:
		var routes []domain.RoutePayload
		seen := map[string]bool{}
		for _, rt := range d.Routes {
			if _, ok := f.services[rt.ProjectID]; !ok {
				return d, fmt.Errorf("서비스 목록에 없는 서비스로 넘기려 했습니다: %q", rt.ProjectID)
			}
			if seen[rt.ProjectID] || len(routes) == intakeMaxRoutes {
				continue
			}
			seen[rt.ProjectID] = true
			rt.Body, rt.Reason = strings.TrimSpace(rt.Body), strings.TrimSpace(rt.Reason)
			if rt.Body == "" {
				rt.Body = r.Body
			}
			routes = append(routes, rt)
		}
		if len(routes) == 0 {
			return d, errors.New("넘기기로 했지만 넘길 서비스가 없습니다")
		}
		d.Routes, d.Work, d.Actions = routes, nil, nil
		return d, nil
	}
	var actions []domain.ActionPayload
	for _, a := range d.Actions {
		if len(actions) == intakeMaxActions {
			break
		}
		a.Reason = strings.TrimSpace(a.Reason)
		switch a.Action {
		case domain.ActionDeliverCode:
			title, ok := f.codeRuns[a.RunID]
			if !ok {
				return d, fmt.Errorf("코드 결과가 있는 실행이 아닙니다: %q", a.RunID)
			}
			a.Title, a.ArtifactID, a.VersionID = title, "", ""
		case domain.ActionSaveResult:
			title, ok := f.results[a.ArtifactID]
			if !ok {
				return d, fmt.Errorf("최근 결과에 없는 결과입니다: %q", a.ArtifactID)
			}
			a.Title, a.RunID, a.VersionID = title, "", ""
		case domain.ActionExportService:
			a.Title, a.RunID, a.ArtifactID, a.VersionID = "", "", "", ""
		case domain.ActionStartWorkflow:
			title, ok := f.versions[a.VersionID]
			if !ok {
				return d, fmt.Errorf("확정된 업무 흐름이 아닙니다: %q", a.VersionID)
			}
			a.Title, a.RunID, a.ArtifactID = title, "", ""
		default:
			return d, fmt.Errorf("알 수 없는 조치입니다: %q", a.Action)
		}
		actions = append(actions, a)
	}
	var work []domain.WorkProposalPayload
	for _, w := range d.Work {
		w.Title = strings.TrimSpace(w.Title)
		if w.Title == "" || len(actions)+len(work) == intakeMaxItems {
			continue
		}
		if !slices.Contains(domain.WorkPriorities, w.Priority) {
			w.Priority = "normal"
		}
		w.Categories = slices.DeleteFunc(slices.Clone(w.Categories), func(c string) bool { return !slices.Contains(f.categories, c) })
		if w.Categories == nil {
			w.Categories = []string{}
		}
		for _, ref := range []struct {
			id    string
			known map[string]string
			what  string
		}{{w.DepartmentID, f.departments, "부서"}, {w.AssigneeAssignmentID, f.assignees, "담당자"}, {w.GoalID, f.goals, "공동 목표"}} {
			if _, ok := ref.known[ref.id]; ref.id != "" && !ok {
				return d, fmt.Errorf("목록에 없는 %s입니다: %q", ref.what, ref.id)
			}
		}
		work = append(work, w)
	}
	if d.Outcome == domain.OutcomeAction && len(actions) == 0 {
		return d, errors.New("조치를 제안하기로 했지만 조치가 없습니다")
	}
	if d.Outcome == domain.OutcomeWork && len(work) == 0 {
		return d, errors.New("업무를 제안하기로 했지만 제목이 있는 업무가 없습니다")
	}
	d.Routes, d.Actions, d.Work, d.Question = nil, actions, work, ""
	return d, nil
}
