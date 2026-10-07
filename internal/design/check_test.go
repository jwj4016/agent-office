package design

import (
	"os"
	"strings"
	"testing"

	"agent-office/internal/domain"
)

func fixture(t *testing.T, edit func(s string) string) *Proposal {
	t.Helper()
	data, err := os.ReadFile("../../tests/fixtures/design/game-launch.json")
	if err != nil {
		t.Fatal(err)
	}
	s := strings.NewReplacer("CONN_AI", "conn-claude", "CONN_CODE", "conn-codex").Replace(string(data))
	if edit != nil {
		s = edit(s)
	}
	p, err := Parse([]byte(s))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

var ctx = Context{
	Roles: []domain.Role{{ID: "role-plan", Name: "기획", Instructions: "기존 기획 지침"}, {ID: "role-cto", Name: "CTO"}},
	Connections: []ConnectionInfo{
		{ID: "conn-claude", Name: "내 Claude", Provider: "claude", Usable: true, Coding: true},
		{ID: "conn-codex", Name: "내 Codex", Provider: "codex", Usable: true, Coding: true},
		{ID: "conn-broken", Name: "확인 안 된 연결", Provider: "codex"},
	},
	HumanTasks: []string{"코드 리뷰"},
}

func codes(r Result) string {
	var out []string
	for _, i := range r.Issues {
		out = append(out, i.Code)
	}
	return strings.Join(out, ",")
}

func TestGameLaunchProposal(t *testing.T) {
	r := Check(fixture(t, nil), ctx, true)
	if !r.CanApply || !r.CanStart {
		t.Fatalf("issues: %v", r.Issues)
	}
	if len(r.Diff.ReuseRoles) != 1 || r.Diff.ReuseRoles[0].ID != "role-plan" {
		t.Fatalf("existing 기획 role not reused: %+v", r.Diff.ReuseRoles)
	}
	if len(r.Diff.NewRoles) != 5 {
		t.Fatalf("new roles = %d", len(r.Diff.NewRoles))
	}
	for _, s := range r.Diff.Steps {
		if s.ID == "review" && (!s.Human || s.Assignee != "나") {
			t.Fatalf("review step = %+v", s)
		}
	}
}

func TestProposalRejectsUnknownFields(t *testing.T) {
	data, _ := os.ReadFile("../../tests/fixtures/design/game-launch.json")
	bad := strings.Replace(string(data), `"notes":`, `"tools": ["bash"], "apiKey": "sk-x", "notes":`, 1)
	if _, err := Parse([]byte(bad)); err == nil {
		t.Fatal("proposal with tools/apiKey accepted")
	}
}

// T02 guard: what the person said they would do must go to the person,
// whatever the model wrote.
func TestHumanTaskMustBeHuman(t *testing.T) {
	toAI := fixture(t, func(s string) string {
		return strings.Replace(s, `"id": "review", "title": "코드 리뷰", "kind": "review", "assignmentId": "a-owner"`,
			`"id": "review", "title": "코드 리뷰", "kind": "review", "assignmentId": "a-qa"`, 1)
	})
	if r := Check(toAI, ctx, true); r.CanApply || !strings.Contains(codes(r), "human_step_not_human") {
		t.Fatalf("review given to AI accepted: %s", codes(r))
	}
	dropped := fixture(t, func(s string) string {
		return strings.Replace(s, `"humanSteps": [{"request": "코드 리뷰", "stepId": "review"}]`, `"humanSteps": []`, 1)
	})
	if r := Check(dropped, ctx, true); r.CanApply || !strings.Contains(codes(r), "human_task_missing") {
		t.Fatalf("missing human task accepted: %s", codes(r))
	}
}

func TestUnusableConnectionIsClearedAndBlocksStart(t *testing.T) {
	p := fixture(t, func(s string) string { return strings.Replace(s, "conn-codex", "conn-broken", 1) })
	r := Check(p, ctx, true)
	if !r.CanApply || r.CanStart || len(r.Diff.Missing) == 0 {
		t.Fatalf("canApply=%v canStart=%v missing=%v", r.CanApply, r.CanStart, r.Diff.Missing)
	}
	for _, a := range r.Proposal.Assignments {
		if a.Ref == "a-dev" && a.ConnectionID != "" {
			t.Fatal("unusable connection kept")
		}
	}
}

func TestHumanAssignmentHasNoConnection(t *testing.T) {
	p := fixture(t, func(s string) string {
		return strings.Replace(s, `"actorKind": "human", "displayName": "나", "connectionId": ""`, `"actorKind": "human", "displayName": "나", "connectionId": "conn-claude"`, 1)
	})
	r := Check(p, ctx, true)
	for _, a := range r.Proposal.Assignments {
		if a.ActorKind == "human" && a.ConnectionID != "" {
			t.Fatal("human assignment kept a connection")
		}
	}
}

func TestStructuralProblemsBlockApply(t *testing.T) {
	roleCycle := fixture(t, func(s string) string {
		return strings.Replace(s, `"ref": "legal", "reuseRoleId": "", "name": "법률 검토", "mission": "출시 관련 법적 쟁점 정리", "instructions": "최종 판단은 사람이 한다고 명시한다", "parentRef": ""`,
			`"ref": "legal", "reuseRoleId": "", "name": "법률 검토", "mission": "", "instructions": "", "parentRef": "qa"`, 1)
	})
	roleCycle.Roles[4].ParentRef = "legal" // qa -> legal -> qa
	if r := Check(roleCycle, ctx, true); r.CanApply || !strings.Contains(codes(r), "role_cycle") {
		t.Fatalf("role cycle accepted: %s", codes(r))
	}
	flowCycle := fixture(t, func(s string) string {
		return strings.Replace(s, `"kind": "task", "assignmentId": "a-planner", "dependsOn": []`, `"kind": "task", "assignmentId": "a-planner", "dependsOn": ["package"]`, 1)
	})
	if r := Check(flowCycle, ctx, true); r.CanApply || !strings.Contains(codes(r), "cycle") {
		t.Fatalf("workflow cycle accepted: %s", codes(r))
	}
	unknownRole := fixture(t, func(s string) string { return strings.Replace(s, `"reuseRoleId": ""`, `"reuseRoleId": "role-nope"`, 1) })
	if r := Check(unknownRole, ctx, true); r.CanApply {
		t.Fatal("unknown reused role accepted")
	}
}

func TestRoleChangesAreOnlyShown(t *testing.T) {
	p := fixture(t, func(s string) string {
		return strings.Replace(s, `"roleChanges": []`, `"roleChanges": [{"roleId": "role-plan", "instructions": "게임 장르 지식을 포함한다", "reason": "게임 기획"}]`, 1)
	})
	r := Check(p, ctx, true)
	if !r.CanApply || len(r.Diff.RoleChanges) != 1 || r.Diff.RoleChanges[0].Current != "기존 기획 지침" {
		t.Fatalf("role change view = %+v", r.Diff.RoleChanges)
	}
}
