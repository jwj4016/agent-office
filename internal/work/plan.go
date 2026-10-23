package work

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"agent-office/internal/design"
	"agent-office/internal/domain"
	"agent-office/internal/storage"
)

// planInput is what an AI work plan is based on. If any of it changes
// before the person applies the proposal, the proposal is stale.
type planInput struct {
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	DoneCriteria   string   `json:"doneCriteria"`
	Categories     []string `json:"categories"`
	DepartmentID   string   `json:"departmentId"`
	DepartmentName string   `json:"departmentName"`
	Responsibility string   `json:"responsibility"`
	AssignmentID   string   `json:"assignmentId"`
}

func (in planInput) hash() string {
	b, _ := json.Marshal(in)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// planInputFor reads the current input of a work item's plan; planner is
// the AI assignment that writes it.
func planInputFor(ctx context.Context, db *storage.DB, projectID, workItemID, planner string) (planInput, error) {
	w, err := db.WorkItem(ctx, projectID, workItemID)
	if err != nil {
		return planInput{}, err
	}
	in := planInput{Title: w.Title, Description: w.Description, DoneCriteria: w.DoneCriteria, Categories: w.Categories,
		DepartmentID: w.DepartmentID, AssignmentID: planner}
	if w.DepartmentID != "" {
		d, err := db.Department(ctx, projectID, w.DepartmentID)
		if err != nil {
			return planInput{}, err
		}
		in.DepartmentName, in.Responsibility = d.Name, d.Responsibility
	}
	return in, nil
}

// planner is the AI that proposes a work item's plan: the assignee of the
// saved plan when that is an AI, otherwise the department's lead.
func planner(ctx context.Context, db *storage.DB, projectID, workItemID string) (domain.Assignment, error) {
	w, err := db.WorkItem(ctx, projectID, workItemID)
	if err != nil {
		return domain.Assignment{}, err
	}
	team, err := db.Assignments(ctx, projectID)
	if err != nil {
		return domain.Assignment{}, err
	}
	find := func(id string) (domain.Assignment, bool) {
		i := slices.IndexFunc(team, func(a domain.Assignment) bool { return a.ID == id && a.ActorKind == domain.ActorAI })
		if i < 0 {
			return domain.Assignment{}, false
		}
		return team[i], true
	}
	if p, ok, err := db.WorkPlan(ctx, projectID, workItemID); err != nil {
		return domain.Assignment{}, err
	} else if ok {
		if a, ok := find(p.AssignmentID); ok {
			return a, nil
		}
	}
	if w.DepartmentID != "" {
		d, err := db.Department(ctx, projectID, w.DepartmentID)
		if err != nil {
			return domain.Assignment{}, err
		}
		if a, ok := find(d.LeadAssignmentID); ok {
			return a, nil
		}
	}
	return domain.Assignment{}, fmt.Errorf("%w: 계획을 제안할 AI 담당자가 없습니다. 부서의 대표 담당자를 AI로 정하거나 계획의 담당자를 AI로 고르세요", storage.ErrInvalid)
}

// RequestPlan queues an AI proposal for a work item's plan. requestID
// comes from the screen, one per click: the same click arriving twice
// returns the job already queued, a new click makes a new job.
func RequestPlan(ctx context.Context, db *storage.DB, projectID, workItemID, requestID string) (storage.AIJob, error) {
	if strings.TrimSpace(requestID) == "" {
		return storage.AIJob{}, fmt.Errorf("%w: 요청 ID가 필요합니다", storage.ErrInvalid)
	}
	w, err := db.WorkItem(ctx, projectID, workItemID)
	if err != nil {
		return storage.AIJob{}, err
	}
	if w.Kind != domain.WorkTask || domain.WorkClosed(w.Status) {
		return storage.AIJob{}, fmt.Errorf("%w: 진행 중인 업무에만 계획을 요청할 수 있습니다", storage.ErrInvalid)
	}
	a, err := planner(ctx, db, projectID, workItemID)
	if err != nil {
		return storage.AIJob{}, err
	}
	in, err := planInputFor(ctx, db, projectID, workItemID, a.ID)
	if err != nil {
		return storage.AIJob{}, err
	}
	snap, _ := json.Marshal(in)
	job, _, err := db.CreateAIJob(ctx, storage.AIJob{ProjectID: projectID, Kind: storage.JobPlan, Lane: storage.LaneInteractive,
		RequestKey: "plan:" + workItemID + ":" + requestID, WorkItemID: workItemID, AssignmentID: a.ID, ConnectionID: a.ConnectionID,
		Model: a.Model, InputSnapshot: snap, InputHash: in.hash()})
	return job, err
}

// PlanStale reports whether a plan job was made from input that has
// changed since, so its proposal may no longer fit the work.
func PlanStale(ctx context.Context, db *storage.DB, job storage.AIJob) (bool, error) {
	in, err := planInputFor(ctx, db, job.ProjectID, job.WorkItemID, job.AssignmentID)
	if err != nil {
		return false, err
	}
	return in.hash() != job.InputHash, nil
}

// ProposedPlan is what the planner returns.
type ProposedPlan struct {
	AssignmentID string          `json:"assignmentId"`
	Instructions string          `json:"instructions"`
	Materials    string          `json:"materials"`
	Outputs      []domain.Output `json:"outputs"`
	Notes        string          `json:"notes"`
}

// ApplyPlan saves a ready proposal as the work item's plan. A proposal
// made from input that has changed since is refused: ask again instead.
func ApplyPlan(ctx context.Context, db *storage.DB, projectID, jobID string) (domain.WorkPlan, error) {
	job, err := db.AIJob(ctx, projectID, jobID)
	if err != nil {
		return domain.WorkPlan{}, err
	}
	if job.Kind != storage.JobPlan || job.Status != storage.JobReady {
		return domain.WorkPlan{}, fmt.Errorf("%w: 준비된 계획 제안만 적용할 수 있습니다", storage.ErrInvalid)
	}
	stale, err := PlanStale(ctx, db, job)
	if err != nil {
		return domain.WorkPlan{}, err
	}
	if stale {
		return domain.WorkPlan{}, fmt.Errorf("%w: 제안을 만든 뒤 업무 내용이나 담당 부서가 바뀌었습니다. 계획을 다시 요청하세요", storage.ErrInvalid)
	}
	var p ProposedPlan
	if err := json.Unmarshal(job.Result, &p); err != nil {
		return domain.WorkPlan{}, err
	}
	saved, err := db.SaveWorkPlan(ctx, domain.WorkPlan{ProjectID: projectID, WorkItemID: job.WorkItemID, AssignmentID: p.AssignmentID,
		Instructions: p.Instructions, Materials: p.Materials, Outputs: p.Outputs})
	if err != nil {
		return saved, err
	}
	return saved, db.MarkAIJobApplied(ctx, projectID, jobID)
}

// PlanKind writes the prompt for a work plan and checks the proposal.
type PlanKind struct{ DB *storage.DB }

var planOutputTypes = []domain.OutputType{domain.OutMarkdown, domain.OutJSON, domain.OutReport, domain.OutFile, domain.OutCodeChange, domain.OutDiagram}

func (k PlanKind) Prepare(ctx context.Context, job storage.AIJob) (design.JSONCall, error) {
	var in planInput
	if err := json.Unmarshal(job.InputSnapshot, &in); err != nil {
		return design.JSONCall{}, err
	}
	layers, err := k.DB.InstructionLayers(ctx, job.ProjectID, job.AssignmentID)
	if err != nil {
		return design.JSONCall{}, err
	}
	team, err := k.DB.Assignments(ctx, job.ProjectID)
	if err != nil {
		return design.JSONCall{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# 작업 계획 제안\n아래 업무를 직접 수행하기 전에, 수행할 작은 작업 계획을 제안한다. 계획은 사람이 확인한 뒤 실행된다.\n\n")
	fmt.Fprintf(&b, "## 업무\n%s\n\n%s\n\n## 완료 조건\n%s\n\n", in.Title, in.Description, orNone(in.DoneCriteria))
	if in.DepartmentName != "" {
		fmt.Fprintf(&b, "## 담당 부서: %s\n%s\n\n", in.DepartmentName, orNone(in.Responsibility))
	}
	b.WriteString("## 이 서비스의 담당자 (assignmentId는 여기 있는 id만 쓴다)\n")
	for _, a := range team {
		kind := "AI"
		if a.ActorKind == domain.ActorHuman {
			kind = "사람"
		}
		fmt.Fprintf(&b, "- id=%s 이름=%s (%s)\n", a.ID, a.DisplayName, kind)
	}
	fmt.Fprintf(&b, `
## 출력
결과 파일에 JSON 객체 하나를 쓴다.
{"assignmentId": 수행할 담당자 id(보통 %s), "instructions": 수행자가 따를 작업 지침, "materials": 읽어 볼 자료(없으면 빈 문자열),
 "outputs": [{"key": 영문 소문자·숫자·밑줄 이름, "type": %s 중 하나}], "notes": 계획의 근거와 가정을 짧게}
- 업무 설명과 완료 조건은 실행할 때 자동으로 함께 전달되므로 instructions에 그대로 옮기지 않는다.
- 산출물은 완료 조건을 확인할 수 있는 최소한으로 정한다.
`, job.AssignmentID, typeList())
	return design.JSONCall{What: "작업 계획 AI", Instructions: domain.ComposeInstructions(layers), Prompt: b.String(),
		Schema: planSchema(), Timeout: 10 * time.Minute, Answer: "질문하지 말고 합리적인 가정을 정해 notes에 적어 주세요."}, nil
}

func (k PlanKind) Accept(ctx context.Context, job storage.AIJob, raw []byte) (json.RawMessage, error) {
	var p ProposedPlan
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("계획 제안을 읽을 수 없습니다: %w", err)
	}
	team, err := k.DB.Assignments(ctx, job.ProjectID)
	if err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(team, func(a domain.Assignment) bool { return a.ID == p.AssignmentID }) {
		p.AssignmentID = job.AssignmentID // an unknown id falls back to the planner
	}
	var outs []domain.Output
	for _, o := range p.Outputs {
		o.Key = strings.TrimSpace(o.Key)
		if o.Key != "" && slices.Contains(planOutputTypes, o.Type) && !slices.ContainsFunc(outs, func(x domain.Output) bool { return x.Key == o.Key }) {
			outs = append(outs, domain.Output{Key: o.Key, Type: o.Type})
		}
	}
	if len(outs) == 0 {
		outs = domain.DefaultWorkOutputs()
	}
	p.Outputs = outs
	p.Instructions, p.Materials, p.Notes = strings.TrimSpace(p.Instructions), strings.TrimSpace(p.Materials), strings.TrimSpace(p.Notes)
	if p.Instructions == "" {
		return nil, fmt.Errorf("계획 제안에 작업 지침이 없습니다")
	}
	return json.Marshal(p)
}

func orNone(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(적지 않음)"
	}
	return s
}

func typeList() string {
	var out []string
	for _, t := range planOutputTypes {
		out = append(out, string(t))
	}
	return strings.Join(out, ", ")
}

func planSchema() json.RawMessage {
	types := make([]string, 0, len(planOutputTypes))
	for _, t := range planOutputTypes {
		types = append(types, string(t))
	}
	b, _ := json.Marshal(map[string]any{"type": "object", "required": []string{"assignmentId", "instructions", "outputs"},
		"properties": map[string]any{
			"assignmentId": map[string]any{"type": "string"},
			"instructions": map[string]any{"type": "string"},
			"materials":    map[string]any{"type": "string"},
			"notes":        map[string]any{"type": "string"},
			"outputs": map[string]any{"type": "array", "items": map[string]any{"type": "object", "required": []string{"key", "type"},
				"properties": map[string]any{"key": map[string]any{"type": "string"}, "type": map[string]any{"type": "string", "enum": types}}}},
		}})
	return b
}
