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
	"agent-office/internal/storage"
)

// eventLabel names an event kind for prompts.
var eventLabel = map[string]string{
	domain.EventWorkCreated: "업무 등록됨", domain.EventWorkCompleted: "업무 완료됨", domain.EventWorkReopened: "업무 재개됨",
	domain.EventWorkCollabRequested: "협업 요청됨", domain.EventDocumentShared: "자료 공유됨", domain.EventDocumentRevised: "자료 개정됨",
	domain.EventDocumentConfirmed: "자료 확정됨", domain.EventDocumentWithdrawn: "자료 철회됨", domain.EventDepartmentCheck: "부서 점검 요청됨",
}

// judgeInput is what a department judges, fixed when the job is made.
type judgeInput struct {
	EventKind      string   `json:"eventKind"`
	EventSequence  int64    `json:"eventSequence"`
	EventTitle     string   `json:"eventTitle"`
	EventNote      string   `json:"eventNote"`
	Categories     []string `json:"categories"`
	DepartmentID   string   `json:"departmentId"`
	DepartmentName string   `json:"departmentName"`
	Responsibility string   `json:"responsibility"`
	Guidance       string   `json:"guidance"`
	WorkItemID     string   `json:"workItemId"`
	VersionID      string   `json:"versionId"`
}

// RequestJudgment queues the department's AI judgment of a delivery: the
// round-1 job, or the round-2 job once a question was answered. The same
// delivery handled twice returns the job already queued. A direct check
// made by a person goes to the interactive lane; everything else to the
// background lane.
func RequestJudgment(ctx context.Context, db *storage.DB, d storage.Delivery) (storage.AIJob, error) {
	if existing, ok, err := latestDeliveryJob(ctx, db, d); err != nil || ok {
		return existing, err
	}
	dept, err := db.Department(ctx, d.ProjectID, d.DepartmentID)
	if err != nil {
		return storage.AIJob{}, err
	}
	team, err := db.Assignments(ctx, d.ProjectID)
	if err != nil {
		return storage.AIJob{}, err
	}
	i := slices.IndexFunc(team, func(a domain.Assignment) bool { return a.ID == dept.LeadAssignmentID })
	if i < 0 || team[i].ActorKind != domain.ActorAI {
		return storage.AIJob{}, fmt.Errorf("%s에 AI 대표 담당자가 없어 판단할 수 없습니다", dept.Name)
	}
	lead := team[i]
	in := judgeInput{EventKind: d.EventKind, EventSequence: d.EventSequence, EventTitle: d.Event.Title, EventNote: d.Event.Note,
		Categories: d.Event.Categories, DepartmentID: dept.ID, DepartmentName: dept.Name, Responsibility: dept.Responsibility,
		WorkItemID: d.Event.WorkItemID, VersionID: d.Event.DocumentVersionID}
	if d.SubscriptionID != "" {
		subs, err := db.Subscriptions(ctx, d.ProjectID, d.DepartmentID)
		if err != nil {
			return storage.AIJob{}, err
		}
		for _, s := range subs {
			if s.ID == d.SubscriptionID {
				in.Guidance = s.Guidance
			}
		}
	}
	lane := storage.LaneBackground
	if d.EventKind == domain.EventDepartmentCheck && !d.Event.Automated {
		lane = storage.LaneInteractive
	}
	snap, _ := json.Marshal(in)
	job, _, err := db.CreateAIJob(ctx, storage.AIJob{ProjectID: d.ProjectID, Kind: storage.JobJudgment, Lane: lane,
		RequestKey: fmt.Sprintf("judgment:%s:1", d.ID), DeliveryID: d.ID, Round: 1, AssignmentID: lead.ID,
		ConnectionID: lead.ConnectionID, Model: lead.Model, InputSnapshot: snap})
	return job, err
}

// latestDeliveryJob returns the newest AI job (judgment or collaboration
// decision) of a delivery.
func latestDeliveryJob(ctx context.Context, db *storage.DB, d storage.Delivery) (storage.AIJob, bool, error) {
	for _, round := range []int{2, 1} {
		var id string
		err := db.Read().QueryRowContext(ctx, `SELECT id FROM work_ai_jobs WHERE delivery_id = ? AND round = ?`, d.ID, round).Scan(&id)
		if err == nil {
			j, err := db.AIJob(ctx, d.ProjectID, id)
			return j, err == nil, err
		}
	}
	return storage.AIJob{}, false, nil
}

// Judge handles deliveries: a judge delivery becomes a judgment job that
// finishes it later; notices are recorded. A collaboration request is
// recorded for the person to decide, or, in auto mode, handed to the lead
// AI as a decision job. Automatic chains stop at the depth limit, held
// for a person.
func Judge(db *storage.DB, wake func(lane string)) Handler {
	return func(ctx context.Context, d storage.Delivery) (Result, error) {
		switch d.Purpose {
		case domain.PurposeCollabRequest:
			return decideCollab(ctx, db, d, wake)
		case domain.PurposeNotify:
			return Acknowledge(ctx, d)
		}
		if d.Event.Automated && d.Event.ChainDepth >= domain.DefaultChainDepth {
			return Result{Status: domain.DeliveryHeld,
				Note: fmt.Sprintf("자동으로 이어진 사건이 %d단계를 넘어 판단을 멈췄습니다. 사람이 확인하세요", domain.DefaultChainDepth)}, nil
		}
		job, err := RequestJudgment(ctx, db, d)
		if err != nil {
			return Result{}, err
		}
		switch job.Status {
		case storage.JobReady:
			return Result{Status: domain.DeliveryDone, Note: "이미 판단했습니다"}, nil
		case storage.JobFailed:
			return Result{}, errors.New(job.Error)
		case storage.JobHeld:
			return Result{Status: domain.DeliveryHeld, Note: job.HoldReason}, nil
		}
		if wake != nil {
			wake(job.Lane)
		}
		return Result{Status: domain.DeliveryProcessing}, nil
	}
}

// JudgeKind writes a department's judgment prompt, checks the answer and
// stores it with its proposals.
type JudgeKind struct {
	DB *storage.DB
	// ReadDocument returns a shared version's text for the prompt.
	ReadDocument func(ctx context.Context, projectID, versionID string) (string, error)
}

const maxJudgeMaterial = 16 << 10

func (k JudgeKind) Prepare(ctx context.Context, job storage.AIJob) (design.JSONCall, error) {
	var in judgeInput
	if err := json.Unmarshal(job.InputSnapshot, &in); err != nil {
		return design.JSONCall{}, err
	}
	layers, err := k.DB.InstructionLayers(ctx, job.ProjectID, job.AssignmentID)
	if err != nil {
		return design.JSONCall{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# 부서 판단: %s\n너는 %s를 대표해, 아래 사건에 이 부서가 새로 맡을 일이 있는지 판단한다.\n", in.DepartmentName, in.DepartmentName)
	b.WriteString("할 일이 없으면 none으로 끝낸다. 꼭 필요한 일만 제안하고, 이미 진행 중이거나 검토 중인 일과 겹치게 제안하지 않는다.\n\n")
	fmt.Fprintf(&b, "## 부서의 책임\n%s\n\n", orNone(in.Responsibility))
	if in.Guidance != "" {
		fmt.Fprintf(&b, "## 이 부서가 보는 관점\n%s\n\n", in.Guidance)
	}
	label := eventLabel[in.EventKind]
	if label == "" {
		label = in.EventKind
	}
	fmt.Fprintf(&b, "## 사건: %s\n%s\n", label, in.EventTitle)
	if in.EventNote != "" {
		fmt.Fprintf(&b, "메모: %s\n", in.EventNote)
	}
	if len(in.Categories) > 0 {
		fmt.Fprintf(&b, "분류: %s\n", strings.Join(in.Categories, ", "))
	}
	b.WriteString("\n")
	if in.WorkItemID != "" {
		if w, err := k.DB.WorkItem(ctx, job.ProjectID, in.WorkItemID); err == nil {
			fmt.Fprintf(&b, "## 관련 업무\n%s\n%s\n완료 조건: %s\n\n", w.Title, w.Description, orNone(w.DoneCriteria))
		}
	}
	if in.VersionID != "" && k.ReadDocument != nil {
		if text, err := k.ReadDocument(ctx, job.ProjectID, in.VersionID); err == nil {
			if len(text) > maxJudgeMaterial {
				text = clipText(text, maxJudgeMaterial) + "\n…(이하 생략)"
			}
			fmt.Fprintf(&b, "## 공유 자료 내용\n````\n%s\n````\n\n", text)
		}
	}
	open, err := k.DB.WorkItems(ctx, job.ProjectID, storage.WorkFilter{Status: "open", DepartmentID: in.DepartmentID})
	if err != nil {
		return design.JSONCall{}, err
	}
	props, err := k.DB.Proposals(ctx, job.ProjectID, "proposed")
	if err != nil {
		return design.JSONCall{}, err
	}
	b.WriteString("## 이 부서의 진행 중인 업무와 검토 중인 제안\n")
	n := 0
	for _, w := range open {
		fmt.Fprintf(&b, "- 업무: %s\n", w.Title)
		n++
	}
	for _, p := range props {
		if p.DepartmentID == in.DepartmentID {
			fmt.Fprintf(&b, "- 제안: %s\n", p.Title)
			n++
		}
	}
	if n == 0 {
		b.WriteString("없음\n")
	}
	if job.Round == 2 {
		qs, err := k.DB.Judgments(ctx, job.ProjectID, "")
		if err != nil {
			return design.JSONCall{}, err
		}
		for _, q := range qs {
			if q.DeliveryID == job.DeliveryID && q.Round == 1 {
				fmt.Fprintf(&b, "\n## 앞서 한 질문과 사람의 답변\n질문: %s\n답변: %s\n이번에는 이 답을 바탕으로 결정한다.\n", q.Question, q.Answer)
			}
		}
	}
	cats, err := k.DB.ProjectCategories(ctx, job.ProjectID)
	if err != nil {
		return design.JSONCall{}, err
	}
	fmt.Fprintf(&b, `
## 출력
결과 파일에 JSON 객체 하나를 쓴다.
{"outcome": "none" | "propose" | "question", "reason": 판단 이유, "evidence": [근거가 된 사실들],
 "question": outcome이 question일 때 사람에게 물을 한 가지,
 "proposals": [{"title": 업무 제목, "description": 할 일, "doneCriteria": 완료 조건, "categories": [%s 중에서], "priority": "urgent"|"high"|"normal"|"low"}]}
- 제안은 %d개까지. 판단에 꼭 필요한 정보가 없을 때만 question을 쓴다.
`, strings.Join(cats, ", "), storage.MaxProposals)
	return design.JSONCall{What: in.DepartmentName + " 판단 AI", Instructions: domain.ComposeInstructions(layers), Prompt: b.String(),
		Schema: judgeSchema(), Timeout: 10 * time.Minute, Answer: "질문하지 말고, 꼭 필요하면 outcome을 question으로 하세요."}, nil
}

func (k JudgeKind) Accept(ctx context.Context, job storage.AIJob, raw []byte) (json.RawMessage, error) {
	var r storage.JudgmentResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("판단 결과를 읽을 수 없습니다: %w", err)
	}
	r.Reason, r.Question = strings.TrimSpace(r.Reason), strings.TrimSpace(r.Question)
	if r.Reason == "" {
		return nil, errors.New("판단 결과에 이유가 없습니다")
	}
	switch r.Outcome {
	case storage.OutcomeNone:
		r.Question, r.Proposals = "", nil
	case storage.OutcomeQuestion:
		if r.Question == "" {
			return nil, errors.New("질문으로 판단했지만 질문이 없습니다")
		}
		r.Proposals = nil
	case storage.OutcomePropose:
		cats, err := k.DB.ProjectCategories(ctx, job.ProjectID)
		if err != nil {
			return nil, err
		}
		var props []storage.ProposedWork
		for _, p := range r.Proposals {
			p.Title = strings.TrimSpace(p.Title)
			if p.Title == "" || len(props) == storage.MaxProposals {
				continue
			}
			if !slices.Contains(domain.WorkPriorities, p.Priority) {
				p.Priority = "normal"
			}
			p.Categories = slices.DeleteFunc(slices.Clone(p.Categories), func(c string) bool { return !slices.Contains(cats, c) })
			props = append(props, p)
		}
		if len(props) == 0 {
			return nil, errors.New("업무를 제안했지만 제목이 있는 제안이 없습니다")
		}
		r.Question, r.Proposals = "", props
	default:
		return nil, fmt.Errorf("알 수 없는 판단 결과입니다: %q", r.Outcome)
	}
	return json.Marshal(r)
}

func (k JudgeKind) Complete(ctx context.Context, job storage.AIJob, result json.RawMessage) error {
	var r storage.JudgmentResult
	if err := json.Unmarshal(result, &r); err != nil {
		return err
	}
	return k.DB.CompleteJudgment(ctx, job, r)
}

func clipText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}

func judgeSchema() json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": "object", "required": []string{"outcome", "reason"},
		"properties": map[string]any{
			"outcome":  map[string]any{"type": "string", "enum": []string{"none", "propose", "question"}},
			"reason":   map[string]any{"type": "string"},
			"evidence": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"question": map[string]any{"type": "string"},
			"proposals": map[string]any{"type": "array", "maxItems": storage.MaxProposals, "items": map[string]any{"type": "object", "required": []string{"title"},
				"properties": map[string]any{
					"title": map[string]any{"type": "string"}, "description": map[string]any{"type": "string"}, "doneCriteria": map[string]any{"type": "string"},
					"categories": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
					"priority":   map[string]any{"type": "string", "enum": domain.WorkPriorities},
				}}},
		}})
	return b
}
