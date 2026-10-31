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
	"agent-office/internal/storage"
)

// Desk staffing (company-requests plan, 3rd revision). When a request
// reaches a service without a desk, an AI looks at the service and writes
// the new desk's name and instructions; the app makes it an AI assignee in
// the one shared desk role ('접수 담당') and sets it as the desk. The
// connection is never the AI's choice: the app takes the verified one the
// service's assignees use most. The company desk is set up the same way,
// with a connection and model of its own instead of an assignee.

// ErrNoConnection means no verified AI connection could staff a desk.
var ErrNoConnection = errors.New("검증된 AI 연결이 없어 접수 담당을 배치하지 못했습니다. 연결 화면에서 실제 호출 시험을 통과한 연결을 만들어 주세요")

// staffTriesPerRequest bounds the automatic staffing calls one request may
// cause; after that it waits for the person.
const staffTriesPerRequest = 2

const (
	maxDeskName         = 40
	maxDeskInstructions = 3000
)

// StaffDesk has an AI set up a new desk for a service (projectID "" for
// the company). requestID names the request that needed it, if any.
func (in *Intake) StaffDesk(ctx context.Context, projectID, requestID string) (domain.IntakeDesk, error) {
	in.staffMu.Lock()
	defer in.staffMu.Unlock()
	db := in.cfg.DB
	conn, model, ok, err := db.PreferredConnection(ctx, projectID)
	if err != nil {
		return domain.IntakeDesk{}, err
	}
	if !ok {
		return domain.IntakeDesk{}, ErrNoConnection
	}
	cur, _, err := db.IntakeDesk(ctx, projectID)
	if err != nil {
		return domain.IntakeDesk{}, err
	}
	written, id, err := in.writeDesk(ctx, projectID, requestID, "create", conn, model, "")
	if err != nil {
		return domain.IntakeDesk{}, err
	}
	var desk domain.IntakeDesk
	if projectID == "" {
		desk, err = db.SaveIntakeDesk(ctx, domain.IntakeDesk{Name: written.Name, ConnectionID: conn.ID, Model: model,
			Instructions: written.Instructions, Budget: cur.Budget, Enabled: true})
	} else {
		var a domain.Assignment
		desk, a, err = db.CreateDeskAssignee(ctx, projectID, written.Name, conn.ID, model, written.Instructions)
		written.AssignmentID = a.ID
	}
	if err != nil {
		db.FinishStaffing(ctx, id, nil, "접수 담당을 저장하지 못했습니다: "+err.Error())
		return desk, err
	}
	return desk, db.FinishStaffing(ctx, id, &written, "")
}

// RewriteDesk has an AI write a desk's instructions again from the
// service as it is now; the desk keeps its name and connection.
func (in *Intake) RewriteDesk(ctx context.Context, projectID string) (domain.IntakeDesk, error) {
	in.staffMu.Lock()
	defer in.staffMu.Unlock()
	db := in.cfg.DB
	desk, ok, err := db.IntakeDesk(ctx, projectID)
	if err != nil {
		return desk, err
	}
	if !ok {
		return desk, errors.New("접수 담당이 아직 없습니다. 먼저 접수 담당을 정하세요")
	}
	var a domain.Assignment
	connID, model, current := desk.ConnectionID, desk.Model, desk.Instructions
	if projectID != "" {
		list, err := db.Assignments(ctx, projectID)
		if err != nil {
			return desk, err
		}
		for _, x := range list {
			if x.ID == desk.AssignmentID {
				a = x
			}
		}
		if a.ID == "" {
			return desk, errors.New("접수 담당으로 정한 담당자를 찾을 수 없습니다")
		}
		connID, model, current = a.ConnectionID, a.Model, a.Overrides.Instructions
	}
	conn, err := db.Connection(ctx, connID)
	if err != nil || !(conn.Provider == "test" || conn.Verified()) {
		var ok bool
		if conn, model, ok, err = db.PreferredConnection(ctx, projectID); err != nil {
			return desk, err
		} else if !ok {
			return desk, ErrNoConnection
		}
	}
	written, id, err := in.writeDesk(ctx, projectID, "", "rewrite", conn, model, current)
	if err != nil {
		return desk, err
	}
	if projectID == "" {
		desk.Instructions = written.Instructions
		desk, err = db.SaveIntakeDesk(ctx, desk)
	} else {
		a.Overrides.Instructions = written.Instructions
		_, err = db.SaveAssignment(ctx, a)
		written.Name, written.AssignmentID = a.DisplayName, a.ID
	}
	if err != nil {
		db.FinishStaffing(ctx, id, nil, "지침을 저장하지 못했습니다: "+err.Error())
		return desk, err
	}
	return desk, db.FinishStaffing(ctx, id, &written, "")
}

// writeDesk makes the desk-writing call and returns what it wrote, with
// the staffing record still open (the caller saves, then finishes it).
func (in *Intake) writeDesk(ctx context.Context, projectID, requestID, kind string, conn domain.ProviderConnection, model, current string) (storage.StaffingResult, string, error) {
	db := in.cfg.DB
	var out storage.StaffingResult
	prompt, err := in.staffingPrompt(ctx, projectID, kind, current)
	if err != nil {
		return out, "", err
	}
	prov, err := in.cfg.Providers(conn)
	if err != nil {
		return out, "", err
	}
	id, err := db.StartStaffing(ctx, storage.Staffing{ProjectID: projectID, RequestID: requestID, Kind: kind, ConnectionID: conn.ID, Model: model})
	if err != nil {
		return out, "", err
	}
	str := map[string]any{"type": "string"}
	schema, _ := json.Marshal(map[string]any{"type": "object", "required": []string{"name", "instructions"},
		"properties": map[string]any{"name": str, "instructions": str}})
	raw, err := in.cfg.Call(ctx, prov, design.JSONCall{What: "접수 담당 배치 AI", Prompt: prompt, Model: model, Schema: schema,
		Timeout: 10 * time.Minute, Answer: "질문하지 말고 주어진 정보로 쓰세요.",
		OnEvent: func(ev providers.Event) error {
			if ev.Kind == providers.KindUsage {
				return db.RecordStaffingUsage(context.Background(), id, ev.Payload)
			}
			return nil
		}})
	if ctx.Err() != nil {
		return out, id, ctx.Err() // stopping: recovery ends the record on the next start
	}
	if err == nil {
		err = json.Unmarshal(raw, &out)
	}
	out.Name, out.Instructions = strings.TrimSpace(out.Name), strings.TrimSpace(out.Instructions)
	if err == nil && (out.Name == "" || out.Instructions == "") {
		err = errors.New("접수 담당의 이름이나 지침이 비어 있습니다")
	}
	if err != nil {
		db.FinishStaffing(ctx, id, nil, err.Error())
		return out, id, fmt.Errorf("접수 담당을 배치하지 못했습니다: %w", err)
	}
	out.Name = clipRunes(out.Name, maxDeskName)
	out.Instructions = clipRunes(out.Instructions, maxDeskInstructions)
	return out, id, nil
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func (in *Intake) staffingPrompt(ctx context.Context, projectID, kind, current string) (string, error) {
	db := in.cfg.DB
	var b strings.Builder
	if projectID == "" {
		org, err := db.Organization(ctx)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "# 회사 접수 담당 배치: %s\n", org.Name)
		b.WriteString("대표가 회사에 한 요청을 읽고 어느 서비스의 일인지 가려 그 서비스의 접수 담당에게 넘기는 회사 접수 담당(비서실)을 둔다. ")
		b.WriteString("아래 회사 정보를 읽고 이름과 지침을 쓴다. 지침에는 서비스를 가를 때 볼 것(각 서비스가 맡는 일), 여러 서비스에 걸친 요청을 나누는 기준, 대표에게 되물어야 할 때를 쓴다.\n\n")
		fmt.Fprintf(&b, "## 회사 지침\n%s\n\n", orNone(clipText(org.Instructions, 2000)))
		rows, err := db.Read().QueryContext(ctx, `SELECT name, goal FROM projects WHERE status = 'active' ORDER BY name`)
		if err != nil {
			return "", err
		}
		b.WriteString("## 서비스\n")
		for rows.Next() {
			var name, goal string
			rows.Scan(&name, &goal)
			fmt.Fprintf(&b, "- %s: %s\n", name, orNone(clipText(goal, 400)))
		}
		rows.Close()
	} else {
		p, err := db.Project(ctx, projectID)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "# 접수 담당 배치: %s\n", p.Name)
		fmt.Fprintf(&b, "서비스 ‘%s’에 대표의 요청을 받을 접수 담당을 둔다. 아래 서비스 정보를 읽고 이 접수 담당의 이름과 이 서비스에 맞춘 지침을 쓴다.\n\n", p.Name)
		fmt.Fprintf(&b, "모든 서비스의 접수 담당에게 공통인 기준(역할 ‘%s’의 지침)은 이미 있으니 다시 쓰지 않는다.\n", storage.DeskRoleName)
		b.WriteString("지침에는 이 서비스에만 해당하는 것을 쓴다: 이 서비스가 무엇이고 지금 어느 단계인지, 요청을 판단할 때 볼 것(어느 부서·담당자가 무슨 일을 맡는지), 대표에게 올려야 하는 결정, 대표에게 보고하는 방식. ")
		b.WriteString("서비스 지침의 문장을 그대로 옮기지 말고 접수에 필요한 것으로 줄인다.\n\n")
		mode := "검토 모드"
		if p.Mode == domain.ModeAuto {
			mode = "자동 모드"
		}
		fmt.Fprintf(&b, "## 서비스\n목표: %s\n지침: %s\n모드: %s\n\n", orNone(clipText(p.Goal, 1500)), orNone(clipText(p.Instructions, 3000)), mode)
		depts, err := db.Departments(ctx, projectID)
		if err != nil {
			return "", err
		}
		b.WriteString("## 부서\n")
		n := 0
		for _, d := range depts {
			if d.Status == domain.StatusActive {
				fmt.Fprintf(&b, "- %s: %s\n", d.Name, orNone(clipText(d.Responsibility, 300)))
				n++
			}
		}
		if n == 0 {
			b.WriteString("없음\n")
		}
		roles, err := db.Roles(ctx)
		if err != nil {
			return "", err
		}
		roleName := map[string]string{}
		for _, r := range roles {
			roleName[r.ID] = r.Name
		}
		people, err := db.Assignments(ctx, projectID)
		if err != nil {
			return "", err
		}
		b.WriteString("\n## 담당자\n")
		for _, a := range people {
			kind := "AI"
			if a.ActorKind != domain.ActorAI {
				kind = "사람"
			}
			fmt.Fprintf(&b, "- %s (%s, 역할 %s)\n", a.DisplayName, kind, roleName[a.RoleID])
		}
		if runs, err := in.cfg.Engine.Runs(ctx, projectID); err == nil && len(runs) > 0 {
			b.WriteString("\n## 최근 실행\n")
			for i, r := range runs {
				if i == 5 {
					break
				}
				fmt.Fprintf(&b, "- %s (%s)\n", r.Title, r.Status)
			}
		}
	}
	if kind == "rewrite" {
		fmt.Fprintf(&b, "\n## 지금 지침\n%s\n이 지침을 지금 정보에 맞게 다시 쓴다. 이름은 바꾸지 않아도 되며 지금 이름을 그대로 적어도 된다.\n", orNone(current))
	}
	fmt.Fprintf(&b, `
## 출력
결과 파일에 JSON 객체 하나를 쓴다.
{"name": 접수 담당 이름(%d자 이내, 예: ‘서비스이름 접수 담당’), "instructions": 지침(1500자 이내, 짧은 문장 목록)}
`, maxDeskName)
	return b.String(), nil
}
