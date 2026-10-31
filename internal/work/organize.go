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
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// Organizing an existing service (company-requests plan, 4th revision):
// an AI groups the service's assignees that are in no department yet into
// departments, and writes an intake desk when there is none. The person
// sees the proposal and applies it. Applying also moves open work without
// a department into its assignee's new department, so work that waited
// for department rules can go on.

// ProposeOrg asks an AI for a service's organization and keeps the checked
// proposal for the person. It waits for the AI.
func (in *Intake) ProposeOrg(ctx context.Context, projectID string) (storage.OrgProposal, error) {
	db := in.cfg.DB
	conn, model, ok, err := db.PreferredConnection(ctx, projectID)
	if err != nil {
		return storage.OrgProposal{}, err
	}
	if !ok {
		return storage.OrgProposal{}, ErrNoConnection
	}
	prompt, eligible, hasDesk, err := in.orgPrompt(ctx, projectID)
	if err != nil {
		return storage.OrgProposal{}, err
	}
	if len(eligible) == 0 && hasDesk {
		return storage.OrgProposal{}, errors.New("모든 담당자가 이미 부서에 있고 접수 담당도 있어 제안할 조직이 없습니다")
	}
	prov, err := in.cfg.Providers(conn)
	if err != nil {
		return storage.OrgProposal{}, err
	}
	id, err := db.StartOrgProposal(ctx, projectID, conn.ID, model)
	if err != nil {
		return storage.OrgProposal{}, err
	}
	raw, err := in.cfg.Call(ctx, prov, design.JSONCall{What: "조직 정비 AI", Prompt: prompt, Model: model, Schema: orgSchema(),
		Timeout: 10 * time.Minute, Answer: "질문하지 말고 주어진 정보로 쓰세요.",
		OnEvent: func(ev providers.Event) error {
			if ev.Kind == providers.KindUsage {
				return db.RecordOrgProposalUsage(context.Background(), id, ev.Payload)
			}
			return nil
		}})
	if ctx.Err() != nil {
		return storage.OrgProposal{}, ctx.Err()
	}
	var plan domain.OrgPlan
	if err == nil {
		plan, err = acceptOrg(raw, eligible, hasDesk)
	}
	if err != nil {
		db.FinishOrgProposal(ctx, id, nil, err.Error())
		return storage.OrgProposal{}, fmt.Errorf("조직을 제안받지 못했습니다: %w", err)
	}
	if err := db.FinishOrgProposal(ctx, id, &plan, ""); err != nil {
		return storage.OrgProposal{}, err
	}
	return db.OrgProposal(ctx, projectID, id)
}

// ApplyOrgProposal creates a ready proposal's departments and desk, lets
// auto mode use the connections the person chose, and moves open work
// into the new departments.
func (in *Intake) ApplyOrgProposal(ctx context.Context, projectID, id string, allow []string) (storage.OrgApplied, error) {
	db := in.cfg.DB
	p, err := db.OrgProposal(ctx, projectID, id)
	if err != nil {
		return storage.OrgApplied{}, err
	}
	if p.Status != "ready" {
		return storage.OrgApplied{}, errors.New("이미 적용했거나 버린 조직 제안입니다")
	}
	if len(allow) > 0 {
		proj, err := db.Project(ctx, projectID)
		if err != nil {
			return storage.OrgApplied{}, err
		}
		var policy design.AutoPolicy
		json.Unmarshal(proj.AutoPolicy, &policy)
		for _, c := range allow {
			if _, err := db.Connection(ctx, c); err != nil {
				return storage.OrgApplied{}, fmt.Errorf("AI 연결을 찾을 수 없습니다: %s", c)
			}
			if !slices.Contains(policy.AllowedConnectionIDs, c) {
				policy.AllowedConnectionIDs = append(policy.AllowedConnectionIDs, c)
			}
		}
		raw, _ := json.Marshal(policy)
		if err := db.SetAutoPolicy(ctx, projectID, raw); err != nil {
			return storage.OrgApplied{}, err
		}
	}
	applied, err := db.ApplyOrgPlan(ctx, projectID, p.Plan, nil)
	if err != nil {
		return applied, err
	}
	if applied.AdoptedWork, err = db.AdoptWorkIntoDepartments(ctx, projectID); err != nil {
		applied.Problems = append(applied.Problems, "업무를 부서로 옮기지 못했습니다: "+err.Error())
	}
	return applied, db.CloseOrgProposal(ctx, projectID, id, "applied", &applied)
}

func (in *Intake) orgPrompt(ctx context.Context, projectID string) (string, map[string]bool, bool, error) {
	db := in.cfg.DB
	p, err := db.Project(ctx, projectID)
	if err != nil {
		return "", nil, false, err
	}
	depts, err := db.Departments(ctx, projectID)
	if err != nil {
		return "", nil, false, err
	}
	people, err := db.Assignments(ctx, projectID)
	if err != nil {
		return "", nil, false, err
	}
	roles, err := db.Roles(ctx)
	if err != nil {
		return "", nil, false, err
	}
	_, hasDesk, err := db.IntakeDesk(ctx, projectID)
	if err != nil {
		return "", nil, false, err
	}
	roleName := map[string]string{}
	for _, r := range roles {
		roleName[r.ID] = r.Name
	}
	placed := map[string]string{}
	for _, d := range depts {
		for _, m := range d.MemberIDs {
			placed[m] = d.Name
		}
	}
	eligible := map[string]bool{}
	var b strings.Builder
	fmt.Fprintf(&b, "# 조직 정비: %s\n", p.Name)
	b.WriteString("이 서비스는 작은 회사처럼 일한다. 아직 부서에 들어가지 않은 담당자를 함께 일하는 팀(부서)으로 묶는다.")
	if !hasDesk {
		b.WriteString(" 대표의 요청을 받을 접수 담당도 함께 제안한다.")
	}
	b.WriteString("\n\n")
	mode := "검토 모드"
	if p.Mode == domain.ModeAuto {
		mode = "자동 모드(부서가 자동으로 업무를 시작한다)"
	}
	fmt.Fprintf(&b, "## 서비스\n목표: %s\n지침: %s\n모드: %s\n\n", orNone(clipText(p.Goal, 1500)), orNone(clipText(p.Instructions, 3000)), mode)
	b.WriteString("## 이미 있는 부서(바꾸지 않는다, 같은 이름을 쓰지 않는다)\n")
	if len(depts) == 0 {
		b.WriteString("없음\n")
	}
	for _, d := range depts {
		fmt.Fprintf(&b, "- %s: %s\n", d.Name, orNone(clipText(d.Responsibility, 200)))
	}
	b.WriteString("\n## 부서에 넣을 수 있는 담당자(id는 members·lead에 쓴다)\n")
	for _, a := range people {
		if _, in := placed[a.ID]; in || roleName[a.RoleID] == storage.DeskRoleName {
			continue
		}
		eligible[a.ID] = a.ActorKind == domain.ActorAI
		kind := "AI"
		if a.ActorKind != domain.ActorAI {
			kind = "사람"
		}
		fmt.Fprintf(&b, "- id: %s, %s (%s, 역할 %s)\n", a.ID, a.DisplayName, kind, roleName[a.RoleID])
	}
	if len(eligible) == 0 {
		b.WriteString("없음\n")
	}
	fmt.Fprintf(&b, `
## 출력
결과 파일에 JSON 객체 하나를 쓴다.
{"departments": [{"name": 부서 이름, "responsibility": 이 부서가 책임지는 일 한두 문장, "members": [위 담당자 id], "lead": members 중 AI 담당자 id,
   "watches": [{"eventKinds": [%s 중에서], "guidance": 그 사건을 볼 때 이 부서가 확인할 것}]}],
 "intakeDesk": {"name": 접수 담당 이름, "instructions": 이 서비스에 맞춘 접수 지침}}
- 부서는 보통 2~5개. AI 담당자는 모두 어느 부서에 넣는다. 사람 담당자는 넣지 않아도 된다.
- watches는 다른 부서의 일이나 자료를 보고 이 부서가 할 일이 생기는 경우에만 둔다(예: 검증 부서는 개발 부서의 자료 공유를 본다).
`, strings.Join(domain.WatchableEvents, "|"))
	if hasDesk {
		b.WriteString("- 접수 담당은 이미 있으니 intakeDesk의 name과 instructions는 빈 문자열로 둔다.\n")
	} else {
		b.WriteString("- intakeDesk.instructions에는 서비스 단계, 요청을 어느 부서·담당자에게 맡길지, 대표에게 올릴 결정, 보고 방식을 쓴다.\n")
	}
	return b.String(), eligible, hasDesk, nil
}

func orgSchema() json.RawMessage {
	str := map[string]any{"type": "string"}
	arr := func(item map[string]any) map[string]any { return map[string]any{"type": "array", "items": item} }
	b, _ := json.Marshal(map[string]any{"type": "object", "required": []string{"departments", "intakeDesk"}, "properties": map[string]any{
		"departments": arr(map[string]any{"type": "object", "required": []string{"name", "responsibility", "members", "lead", "watches"},
			"properties": map[string]any{"name": str, "responsibility": str, "members": arr(str), "lead": str,
				"watches": arr(map[string]any{"type": "object", "required": []string{"eventKinds", "guidance"},
					"properties": map[string]any{"eventKinds": arr(map[string]any{"type": "string", "enum": domain.WatchableEvents}), "guidance": str}})}}),
		"intakeDesk": map[string]any{"type": "object", "required": []string{"name", "instructions"}, "properties": map[string]any{"name": str, "instructions": str}},
	}})
	return b
}

// acceptOrg checks a proposed organization against the assignees the AI
// was shown.
func acceptOrg(raw []byte, eligible map[string]bool, hasDesk bool) (domain.OrgPlan, error) {
	var plan domain.OrgPlan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return plan, fmt.Errorf("조직 제안을 읽을 수 없습니다: %w", err)
	}
	if hasDesk || (plan.IntakeDesk != nil && strings.TrimSpace(plan.IntakeDesk.Name) == "") {
		plan.IntakeDesk = nil
	}
	if problems := plan.Check(eligible); len(problems) > 0 {
		return plan, errors.New(strings.Join(problems, "; "))
	}
	if len(plan.Departments) == 0 && plan.IntakeDesk == nil {
		return plan, errors.New("제안된 부서도 접수 담당도 없습니다")
	}
	return plan, nil
}
