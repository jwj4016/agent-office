package design

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/providers"
)

// Request is what the person asked for.
type Request struct {
	Goal string `json:"goal"`
	// ProjectID is empty for a new service.
	ProjectID   string   `json:"projectId"`
	ProjectName string   `json:"projectName"`
	HumanTasks  []string `json:"humanTasks"`
	Mode        string   `json:"mode"` // review | auto
	// Scope for auto mode on a new service (an existing one uses its own).
	AutoPolicy AutoPolicy `json:"autoPolicy"`
}

// AutoPolicy is what auto mode may do without asking.
type AutoPolicy struct {
	AllowedConnectionIDs []string `json:"allowedConnectionIds"`
}

const instructions = `너는 Agent Office의 설계 담당이다. 사용자의 목표를 이루기 위한 회사 역할, 담당자, 업무 흐름을 JSON 설계안으로 제안한다.
설계안은 제안일 뿐이며 사람이 검토한다. 도구 권한, API 키, 연결을 새로 만들거나 요구하는 필드를 넣지 않는다.`

// BuildPrompt describes the goal, the real organization and connections
// and the exact output format.
func BuildPrompt(req Request, c Context, project *domain.Project) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# 목표\n%s\n\n", strings.TrimSpace(req.Goal))
	if project != nil {
		fmt.Fprintf(&b, "# 대상 서비스 (기존)\n이름: %s\n목표: %s\n\n", project.Name, project.Goal)
	} else {
		b.WriteString("# 대상 서비스\n새 서비스를 만든다. project.name에 짧은 이름을 정한다.")
		if req.ProjectName != "" {
			fmt.Fprintf(&b, " 사용자가 원한 이름: %s", req.ProjectName)
		}
		b.WriteString("\n\n")
	}
	b.WriteString("# 사람이 직접 맡을 업무\n")
	if len(req.HumanTasks) == 0 {
		b.WriteString("없음. 그래도 중요한 결정(방향·출시)은 사람의 승인(approval) 업무로 둔다.\n\n")
	} else {
		for _, t := range req.HumanTasks {
			fmt.Fprintf(&b, "- %s\n", t)
		}
		b.WriteString("각 업무를 사람 담당자(actorKind human)의 업무로 만들고, humanSteps에 {request: 위 문구 그대로, stepId}를 하나씩 적는다.\n\n")
	}
	b.WriteString("# 회사의 기존 역할 (먼저 재사용한다. 재사용할 때 reuseRoleId에 id를 적는다)\n")
	if len(c.Roles) == 0 {
		b.WriteString("없음\n")
	}
	for _, r := range c.Roles {
		fmt.Fprintf(&b, "- id=%s 이름=%s 책임=%s\n", r.ID, r.Name, r.Mission)
	}
	b.WriteString("\n# 사용할 수 있는 AI 연결 (AI 담당자의 connectionId는 여기서만 고른다)\n")
	any := false
	for _, ci := range c.Connections {
		if !ci.Usable {
			continue
		}
		any = true
		kind := "문서·분석"
		if ci.Coding {
			kind = "코드 작성 가능"
		}
		fmt.Fprintf(&b, "- id=%s 이름=%s 종류=%s (%s)\n", ci.ID, ci.Name, ci.Provider, kind)
	}
	if !any {
		b.WriteString("없음. connectionId는 비우고 missing에 kind=connection으로 적는다.\n")
	}
	if len(c.Assignments) > 0 {
		b.WriteString("\n# 이 서비스의 기존 담당자 (필요하면 같은 역할·이름으로 다시 제안한다)\n")
		for _, a := range c.Assignments {
			fmt.Fprintf(&b, "- %s (%s)\n", a.DisplayName, a.ActorKind)
		}
	}
	b.WriteString(`
# 업무 흐름 형식 (workflow)
{"schemaVersion": 1, "title": "...", "nodes": [노드...]}
노드: {"id": 영문 id, "title": 한국어 이름, "kind": task|review|approval, "assignmentId": assignments[].ref,
      "dependsOn": [선행 노드 id], "instructions": 이번 업무 지시,
      "inputs": [{"name": 이름, "fromStep": 선행(조상) 노드 id, "outputKey": 그 노드의 결과 key}],
      "outputs": [{"key": 영문 key, "type": markdown|json|report|code_change}],
      "reworkTargets": [반려 시 돌려보낼 선행 task·review 노드 id]}
규칙:
- 진행은 순환 없는 그래프다. 반려 후 되돌아가는 흐름은 연결선이 아니라 review·approval 노드의 reworkTargets로 표현한다.
- task·review는 결과(outputs)를 하나 이상 가진다. review 결과는 type report. approval은 결과가 없고 사람만 맡는다.
- inputs의 fromStep은 반드시 선행 조상 노드여야 한다.
- 코드 작성은 "코드 작성 가능" 연결의 AI에게 맡기고 결과는 code_change로 둔다.
- 법률·재무 판단은 AI가 정리하고 최종 결정은 사람의 승인으로 둔다. 실제 자료 연결이 없으면 조사했다고 꾸미지 않는다.
- 조건 분기(condition)·합류(join)는 쓰지 않는다.

# 출력
지정한 결과 파일(design)에 아래 키만 가진 JSON 객체 하나를 쓴다:
project{name,goal,instructions}, roles[{ref,reuseRoleId,name,mission,instructions,parentRef}],
assignments[{ref,roleRef,actorKind,displayName,connectionId,model,instructions}], workflow, humanSteps[{request,stepId}],
missing[{kind: connection|material|scope, detail}], roleChanges[{roleId,instructions,reason}], notes.
모든 키를 빠짐없이 쓰고 해당 없으면 빈 문자열이나 빈 배열을 쓴다. 기존 역할을 바꾸고 싶으면 roleChanges에 제안만 한다.
`)
	return b.String()
}

// Run asks a provider for a proposal and returns the raw JSON. Tool
// requests are declined (designing needs no tools) and questions get a
// fixed answer so the run never waits for a person.
func Run(ctx context.Context, prov providers.Provider, model, prompt string) ([]byte, error) {
	dir, err := os.MkdirTemp("", "agent-office-design-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	out := filepath.Join(dir, "out", "design.json")
	os.MkdirAll(filepath.Dir(out), 0o700)
	schema, _ := json.Marshal(Schema())
	// CLI agents only see the prompt, so it must name the exact file.
	prompt += "\n결과 파일 경로: " + out + "\n이 경로에 JSON 객체 하나만 담은 파일을 저장한 뒤 작업을 마친다. 다른 파일은 만들지 않는다.\n"
	req := providers.StartRequest{
		ProjectID: "design", RunID: "design", StepAttemptID: "design-" + time.Now().Format("150405.000"), StepID: "design",
		Instructions: instructions, Prompt: prompt, Workspace: dir, WritableDirs: []string{filepath.Dir(out)}, Model: model,
		Policy:     providers.Policy{Sandbox: "workspace-write"},
		OutputSpec: []providers.OutputSpec{{Key: "design", Type: "json", Required: true, Schema: schema, Path: out}},
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	s, err := prov.Start(ctx, req)
	if err != nil {
		return nil, err
	}
	var final providers.CompletedPayload
	for done := false; !done; {
		select {
		case <-ctx.Done():
			s.Cancel(context.Background())
			return nil, errors.New("설계 AI가 10분 안에 끝나지 않았습니다")
		case ev, ok := <-s.Events():
			if !ok {
				done = true
				break
			}
			switch ev.Kind {
			case providers.KindApprovalRequest, providers.KindQuestion:
				var p providers.RequestPayload
				json.Unmarshal(ev.Payload, &p)
				s.Respond(context.Background(), providers.Response{RequestID: p.RequestID, Decision: providers.DecisionDecline,
					Answer: "질문하지 말고 합리적인 가정을 정해 notes에 적어 주세요."})
			case providers.KindCompleted:
				json.Unmarshal(ev.Payload, &final)
			}
		}
	}
	if final.Status != providers.StatusSucceeded {
		return nil, fmt.Errorf("설계 AI 실패: %s", firstNonEmpty(final.Error, final.Status))
	}
	if data, err := os.ReadFile(out); err == nil {
		return data, nil
	}
	if text, ok := jsonFromReply(final.Text); ok {
		return []byte(text), nil // the agent answered inline instead
	}
	snippet := strings.TrimSpace(final.Text)
	if len(snippet) > 200 {
		snippet = snippet[:200] + "…"
	}
	return nil, fmt.Errorf("설계 AI가 설계안 파일을 만들지 않았습니다 (마지막 응답: %q)", snippet)
}

// jsonFromReply accepts a reply that is JSON, or that contains one JSON
// object in a ```json fenced block.
func jsonFromReply(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if json.Valid([]byte(text)) {
		return text, true
	}
	if i := strings.Index(text, "```"); i >= 0 {
		rest := text[i+3:]
		rest = strings.TrimPrefix(rest, "json")
		if j := strings.Index(rest, "```"); j >= 0 {
			block := strings.TrimSpace(rest[:j])
			if json.Valid([]byte(block)) {
				return block, true
			}
		}
	}
	return "", false
}

func firstNonEmpty(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return "알 수 없는 오류"
}

// AutoCheck lists why a checked proposal may not apply and start on its
// own (spec §3.1 auto mode). An empty result means auto may proceed.
func AutoCheck(r Result, policy AutoPolicy, hasBudget bool) []domain.Issue {
	var out []domain.Issue
	add := func(code, format string, args ...any) {
		out = append(out, domain.Issue{Code: code, Message: fmt.Sprintf(format, args...), Severity: domain.SevRun})
	}
	if !r.CanStart {
		add("auto_not_startable", "설계안에 해결할 문제가 있어 자동으로 시작할 수 없습니다")
	}
	if !hasBudget {
		add("auto_needs_budget", "자동 실행에는 서비스 예산(토큰 또는 비용)을 미리 정해야 합니다")
	}
	allowed := map[string]bool{}
	for _, id := range policy.AllowedConnectionIDs {
		allowed[id] = true
	}
	for _, a := range r.Proposal.Assignments {
		if a.ActorKind == "ai" && a.ConnectionID != "" && !allowed[a.ConnectionID] {
			add("auto_connection_not_allowed", "%s의 연결은 자동 실행 허용 목록에 없습니다", a.DisplayName)
		}
	}
	if len(r.Diff.Missing) > 0 {
		add("auto_missing", "부족한 연결·자료가 있어 자동으로 시작할 수 없습니다")
	}
	return out
}
