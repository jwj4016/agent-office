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

const collabNote = "협업 요청을 받았습니다. 수락·질문·거절은 협업 요청에서 결정합니다."

// collabInput is what the lead AI decides a request from, fixed when the
// job is made.
type collabInput struct {
	RequestID       string   `json:"requestId"`
	Question        string   `json:"question"`
	FromWorkTitle   string   `json:"fromWorkTitle"`
	FromWorkDetail  string   `json:"fromWorkDetail"`
	FromDepartment  string   `json:"fromDepartment"`
	DepartmentID    string   `json:"departmentId"`
	DepartmentName  string   `json:"departmentName"`
	Responsibility  string   `json:"responsibility"`
	InputVersionIDs []string `json:"inputVersionIds"`
}

// autoPolicyAllows reports whether the service's auto policy and the
// department's rules let automatic work use a connection.
func autoPolicyAllows(p domain.Project, a domain.DepartmentAutomation, connectionID string) bool {
	var policy design.AutoPolicy
	json.Unmarshal(p.AutoPolicy, &policy)
	return connectionID != "" && slices.Contains(policy.AllowedConnectionIDs, connectionID) && a.AllowsConnection(connectionID)
}

// leadAI is the department's lead when that is an AI.
func leadAI(ctx context.Context, db *storage.DB, projectID string, dept domain.Department) (domain.Assignment, bool, error) {
	team, err := db.Assignments(ctx, projectID)
	if err != nil {
		return domain.Assignment{}, false, err
	}
	i := slices.IndexFunc(team, func(a domain.Assignment) bool { return a.ID == dept.LeadAssignmentID && a.ActorKind == domain.ActorAI })
	if i < 0 {
		return domain.Assignment{}, false, nil
	}
	return team[i], true, nil
}

// autoCollabIssue says why the lead AI does not decide a request (a person
// does), or "" when it may (plan §4.5): the service is in auto mode, the
// department allows reference decisions, the request is reference and
// undecided, and the lead is an AI whose connection auto work may use.
func autoCollabIssue(ctx context.Context, db *storage.DB, r storage.CollabRequest) (string, domain.Assignment, error) {
	if r.Status != storage.CollabRequested {
		return "이미 결정 중인 요청입니다.", domain.Assignment{}, nil
	}
	if r.Requirement == storage.CollabRequired {
		return "완료에 필요한 협업은 사람이 결정합니다.", domain.Assignment{}, nil
	}
	p, err := db.Project(ctx, r.ProjectID)
	if err != nil {
		return "", domain.Assignment{}, err
	}
	if p.Mode != domain.ModeAuto {
		return "", domain.Assignment{}, errReviewMode
	}
	dept, err := db.Department(ctx, r.ProjectID, r.ToDepartmentID)
	if err != nil {
		return "", domain.Assignment{}, err
	}
	rules := domain.ParseAutomation(dept.Automation)
	if !rules.AutoCollab {
		return "", domain.Assignment{}, errReviewMode
	}
	lead, ok, err := leadAI(ctx, db, r.ProjectID, dept)
	if err != nil {
		return "", lead, err
	}
	if !ok {
		return "대표 담당자가 AI가 아니어서 사람이 결정합니다.", lead, nil
	}
	if !autoPolicyAllows(p, rules, lead.ConnectionID) {
		return fmt.Sprintf("%s의 AI 연결이 자동 실행 허용 범위에 없어 사람이 결정합니다.", lead.DisplayName), lead, nil
	}
	return "", lead, nil
}

// errReviewMode means nothing about the request is automatic: the plain
// note is enough.
var errReviewMode = errors.New("review mode")

// decideCollab handles a collaboration delivery. In auto mode, for a
// department that allows it, the lead AI decides a reference request in
// the background lane; otherwise the delivery is recorded and the person
// decides. A decision that would register work past the chain limits is
// held before any call.
func decideCollab(ctx context.Context, db *storage.DB, d storage.Delivery, wake func(string)) (Result, error) {
	if job, ok, err := latestDeliveryJob(ctx, db, d); err != nil {
		return Result{}, err
	} else if ok {
		return jobResult(job, wake)
	}
	r, err := db.CollabForDelivery(ctx, d.ProjectID, d.ID)
	if err != nil {
		return Result{}, err
	}
	issue, lead, err := autoCollabIssue(ctx, db, r)
	if errors.Is(err, errReviewMode) {
		return Result{Status: domain.DeliveryDone, Note: collabNote}, nil
	}
	if err != nil {
		return Result{}, err
	}
	if issue != "" {
		return Result{Status: domain.DeliveryDone, Note: "협업 요청을 받았습니다. " + issue}, nil
	}
	if hold, err := db.ChainHold(ctx, d.EventSequence); err != nil || hold != "" {
		return Result{Status: domain.DeliveryHeld, Note: hold}, err
	}
	dept, err := db.Department(ctx, d.ProjectID, d.DepartmentID)
	if err != nil {
		return Result{}, err
	}
	from, err := db.WorkItem(ctx, d.ProjectID, r.FromWorkItemID)
	if err != nil {
		return Result{}, err
	}
	in := collabInput{RequestID: r.ID, Question: r.Question, FromWorkTitle: from.Title,
		FromWorkDetail: strings.TrimSpace(from.Description + "\n완료 조건: " + orNone(from.DoneCriteria)), FromDepartment: r.FromDepartmentName,
		DepartmentID: dept.ID, DepartmentName: dept.Name, Responsibility: dept.Responsibility, InputVersionIDs: r.InputVersionIDs}
	snap, _ := json.Marshal(in)
	job, _, err := db.CreateAIJob(ctx, storage.AIJob{ProjectID: d.ProjectID, Kind: storage.JobCollabDecision, Lane: storage.LaneBackground,
		RequestKey: fmt.Sprintf("%s:%s:1", storage.JobCollabDecision, d.ID), DeliveryID: d.ID, Round: 1, AssignmentID: lead.ID,
		ConnectionID: lead.ConnectionID, Model: lead.Model, InputSnapshot: snap})
	if err != nil {
		return Result{}, err
	}
	return jobResult(job, wake)
}

// jobResult is what a delivery waiting on an AI job shows: finished,
// failed or held with the job, or still processing (the job is woken).
func jobResult(job storage.AIJob, wake func(string)) (Result, error) {
	switch job.Status {
	case storage.JobReady:
		return Result{Status: domain.DeliveryDone, Note: "이미 결정했습니다"}, nil
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

// CollabKind writes the lead AI's prompt for a collaboration decision,
// checks the answer and stores it.
type CollabKind struct {
	DB           *storage.DB
	ReadDocument func(ctx context.Context, projectID, versionID string) (string, error)
}

func (k CollabKind) Prepare(ctx context.Context, job storage.AIJob) (design.JSONCall, error) {
	var in collabInput
	if err := json.Unmarshal(job.InputSnapshot, &in); err != nil {
		return design.JSONCall{}, err
	}
	layers, err := k.DB.InstructionLayers(ctx, job.ProjectID, job.AssignmentID)
	if err != nil {
		return design.JSONCall{}, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# 협업 요청 결정: %s\n너는 %s를 대표해, %s가 보낸 협업 요청을 수락할지, 거절할지, 먼저 질문할지 정한다.\n", in.DepartmentName, in.DepartmentName, in.FromDepartment)
	b.WriteString("이 부서의 책임에 맞고 해낼 수 있는 일이면 수락한다. 책임 밖이거나 할 수 없으면 이유와 함께 거절한다. 결정에 꼭 필요한 정보가 없을 때만 질문한다.\n\n")
	fmt.Fprintf(&b, "## 부서의 책임\n%s\n\n## 요청 내용\n%s\n\n## 요청한 업무: %s\n%s\n\n", orNone(in.Responsibility), in.Question, in.FromWorkTitle, in.FromWorkDetail)
	budget := maxJudgeMaterial
	for _, v := range in.InputVersionIDs {
		if k.ReadDocument == nil || budget <= 0 {
			break
		}
		text, err := k.ReadDocument(ctx, job.ProjectID, v)
		if err != nil {
			continue
		}
		if len(text) > budget {
			text = clipText(text, budget) + "\n…(이하 생략)"
		}
		budget -= len(text)
		fmt.Fprintf(&b, "## 함께 보낸 자료\n````\n%s\n````\n\n", text)
	}
	open, err := k.DB.WorkItems(ctx, job.ProjectID, storage.WorkFilter{Status: "open", DepartmentID: in.DepartmentID})
	if err != nil {
		return design.JSONCall{}, err
	}
	b.WriteString("## 이 부서의 진행 중인 업무\n")
	for _, w := range open {
		fmt.Fprintf(&b, "- %s\n", w.Title)
	}
	if len(open) == 0 {
		b.WriteString("없음\n")
	}
	if job.Round == 2 {
		r, err := k.DB.CollabRequest(ctx, job.ProjectID, in.RequestID)
		if err != nil {
			return design.JSONCall{}, err
		}
		var q, a string
		for _, h := range r.History {
			switch h.Action {
			case "asked":
				q = h.Body
			case "answered":
				a = h.Body
			}
		}
		fmt.Fprintf(&b, "\n## 앞서 한 질문과 답변\n질문: %s\n답변: %s\n이번에는 이 답을 바탕으로 결정한다.\n", q, a)
	}
	b.WriteString(`
## 출력
결과 파일에 JSON 객체 하나를 쓴다.
{"decision": "accept" | "decline" | "question", "reason": 결정 이유,
 "question": decision이 question일 때 요청한 부서에 물을 한 가지,
 "title": accept일 때 이 부서가 맡을 업무 제목, "doneCriteria": accept일 때 그 업무의 완료 조건}
`)
	return design.JSONCall{What: in.DepartmentName + " 협업 결정 AI", Instructions: domain.ComposeInstructions(layers), Prompt: b.String(),
		Schema: collabSchema(), Timeout: 10 * time.Minute, Answer: "질문하지 말고, 꼭 필요하면 decision을 question으로 하세요."}, nil
}

func (k CollabKind) Accept(ctx context.Context, job storage.AIJob, raw []byte) (json.RawMessage, error) {
	var r storage.CollabDecisionResult
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("협업 결정을 읽을 수 없습니다: %w", err)
	}
	r.Reason, r.Question, r.Title, r.DoneCriteria = strings.TrimSpace(r.Reason), strings.TrimSpace(r.Question), strings.TrimSpace(r.Title), strings.TrimSpace(r.DoneCriteria)
	if r.Reason == "" {
		return nil, errors.New("협업 결정에 이유가 없습니다")
	}
	switch r.Decision {
	case storage.CollabDecideAccept:
		r.Question = ""
	case storage.CollabDecideDecline:
		r.Question, r.Title, r.DoneCriteria = "", "", ""
	case storage.CollabDecideQuestion:
		if r.Question == "" {
			return nil, errors.New("질문으로 결정했지만 질문이 없습니다")
		}
		r.Title, r.DoneCriteria = "", ""
	default:
		return nil, fmt.Errorf("알 수 없는 협업 결정입니다: %q", r.Decision)
	}
	return json.Marshal(r)
}

func (k CollabKind) Complete(ctx context.Context, job storage.AIJob, result json.RawMessage) error {
	var r storage.CollabDecisionResult
	if err := json.Unmarshal(result, &r); err != nil {
		return err
	}
	return k.DB.CompleteCollabDecision(ctx, job, r)
}

func collabSchema() json.RawMessage {
	b, _ := json.Marshal(map[string]any{"type": "object", "required": []string{"decision", "reason"},
		"properties": map[string]any{
			"decision":     map[string]any{"type": "string", "enum": []string{"accept", "decline", "question"}},
			"reason":       map[string]any{"type": "string"},
			"question":     map[string]any{"type": "string"},
			"title":        map[string]any{"type": "string"},
			"doneCriteria": map[string]any{"type": "string"},
		}})
	return b
}
