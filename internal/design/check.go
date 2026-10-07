package design

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"agent-office/internal/domain"
)

// Context is what the designer and the checker know about the world.
type Context struct {
	Roles       []domain.Role
	Connections []ConnectionInfo
	// Existing assignments when designing for an existing project.
	Assignments []domain.Assignment
	// HumanTasks are tasks the person said they will do themselves.
	HumanTasks []string
}

// ConnectionInfo describes a usable connection without any secret.
type ConnectionInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Usable   bool   `json:"usable"`
	Coding   bool   `json:"coding"`
}

// Diff is what applying the proposal would change, for the person.
type Diff struct {
	ReuseRoles  []RoleRef        `json:"reuseRoles"`
	NewRoles    []ProposedRole   `json:"newRoles"`
	RoleChanges []RoleChangeView `json:"roleChanges"`
	Assignments []AssignmentView `json:"assignments"`
	Steps       []StepView       `json:"steps"`
	Missing     []MissingItem    `json:"missing"`
}

type RoleRef struct {
	Ref  string `json:"ref"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

type RoleChangeView struct {
	RoleChange
	RoleName string `json:"roleName"`
	Current  string `json:"current"`
}

type AssignmentView struct {
	ProposedAssignment
	RoleName       string `json:"roleName"`
	ConnectionName string `json:"connectionName"`
	Connected      bool   `json:"connected"`
}

type StepView struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Kind     string `json:"kind"`
	Assignee string `json:"assignee"`
	Human    bool   `json:"human"`
}

// Result is the checked proposal: issues use the workflow severities
// (SevError blocks applying, SevRun blocks starting).
type Result struct {
	Proposal *Proposal      `json:"proposal"`
	Issues   []domain.Issue `json:"issues"`
	Diff     Diff           `json:"diff"`
	CanApply bool           `json:"canApply"`
	CanStart bool           `json:"canStart"`
}

var refPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

func issue(sev domain.Severity, code, format string, args ...any) domain.Issue {
	return domain.Issue{Code: code, Message: fmt.Sprintf(format, args...), Severity: sev}
}

func norm(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// Check validates a proposal against the real organization and
// connections and normalizes it: same-name new roles become reuse,
// unusable connections are cleared and reported as missing, people are
// always local-owner.
func Check(p *Proposal, c Context, newProject bool) Result {
	var issues []domain.Issue
	add := func(i domain.Issue) { issues = append(issues, i) }
	roleByID := map[string]domain.Role{}
	roleByName := map[string]domain.Role{}
	for _, r := range c.Roles {
		roleByID[r.ID] = r
		roleByName[norm(r.Name)] = r
	}
	conns := map[string]ConnectionInfo{}
	for _, ci := range c.Connections {
		conns[ci.ID] = ci
	}

	if newProject && strings.TrimSpace(p.Project.Name) == "" {
		add(issue(domain.SevError, "missing_project_name", "새 서비스 이름이 없습니다"))
	}

	// Roles: reuse first; a "new" role with an existing name is reuse.
	refs := map[string]string{} // proposed ref -> existing role id ("" for new)
	var diff Diff
	for i := range p.Roles {
		r := &p.Roles[i]
		if !refPattern.MatchString(r.Ref) {
			add(issue(domain.SevError, "bad_role_ref", "역할 참조 %q 형식이 올바르지 않습니다", r.Ref))
			continue
		}
		if _, dup := refs[r.Ref]; dup {
			add(issue(domain.SevError, "duplicate_role_ref", "역할 참조 %q가 중복됩니다", r.Ref))
			continue
		}
		if r.ReuseRoleID == "" {
			if existing, ok := roleByName[norm(r.Name)]; ok {
				r.ReuseRoleID = existing.ID
			}
		}
		if r.ReuseRoleID != "" {
			existing, ok := roleByID[r.ReuseRoleID]
			if !ok {
				add(issue(domain.SevError, "unknown_role", "재사용할 역할 %q가 회사 조직에 없습니다", r.ReuseRoleID))
				continue
			}
			refs[r.Ref] = existing.ID
			diff.ReuseRoles = append(diff.ReuseRoles, RoleRef{Ref: r.Ref, ID: existing.ID, Name: existing.Name})
			continue
		}
		if strings.TrimSpace(r.Name) == "" {
			add(issue(domain.SevError, "missing_role_name", "새 역할 %q의 이름이 없습니다", r.Ref))
			continue
		}
		refs[r.Ref] = ""
		diff.NewRoles = append(diff.NewRoles, *r)
	}
	for _, r := range diff.NewRoles {
		if r.ParentRef == "" {
			continue
		}
		if _, ok := refs[r.ParentRef]; !ok {
			if _, ok := roleByID[r.ParentRef]; !ok {
				add(issue(domain.SevError, "unknown_parent_role", "역할 %q의 상위 역할 %q가 없습니다", r.Name, r.ParentRef))
			}
		}
	}
	if cyc := newRoleCycle(diff.NewRoles); cyc != "" {
		add(issue(domain.SevError, "role_cycle", "새 역할의 상위 관계에 순환이 있습니다 (%s)", cyc))
	}

	// Existing-role edits are only shown.
	for _, ch := range p.RoleChanges {
		r, ok := roleByID[ch.RoleID]
		if !ok {
			add(issue(domain.SevError, "unknown_role", "수정 제안 대상 역할 %q가 없습니다", ch.RoleID))
			continue
		}
		diff.RoleChanges = append(diff.RoleChanges, RoleChangeView{RoleChange: ch, RoleName: r.Name, Current: r.Instructions})
	}

	// Assignments: people are always the local owner; AI connections must
	// be real and usable or they are cleared and reported missing.
	infos := map[string]domain.AssignmentInfo{}
	byRef := map[string]*ProposedAssignment{}
	for i := range p.Assignments {
		a := &p.Assignments[i]
		if !refPattern.MatchString(a.Ref) || byRef[a.Ref] != nil {
			add(issue(domain.SevError, "bad_assignment_ref", "담당자 참조 %q가 올바르지 않거나 중복됩니다", a.Ref))
			continue
		}
		existingRole, ok := refs[a.RoleRef]
		if !ok {
			add(issue(domain.SevError, "unknown_role_ref", "담당자 %q의 역할 %q가 설계안에 없습니다", a.DisplayName, a.RoleRef))
			continue
		}
		if strings.TrimSpace(a.DisplayName) == "" {
			a.DisplayName = a.Ref
		}
		view := AssignmentView{ProposedAssignment: *a}
		if existingRole != "" {
			view.RoleName = roleByID[existingRole].Name
		} else {
			for _, r := range diff.NewRoles {
				if r.Ref == a.RoleRef {
					view.RoleName = r.Name
				}
			}
		}
		switch a.ActorKind {
		case "human":
			a.ConnectionID, a.Model = "", ""
			view.ProposedAssignment, view.Connected = *a, true
		case "ai":
			ci, ok := conns[a.ConnectionID]
			if a.ConnectionID != "" && (!ok || !ci.Usable) {
				diff.Missing = append(diff.Missing, MissingItem{Kind: "connection", Detail: fmt.Sprintf("%s: 제안된 연결 %q를 사용할 수 없어 비워 두었습니다", a.DisplayName, a.ConnectionID)})
				a.ConnectionID = ""
			}
			if a.ConnectionID == "" {
				add(issue(domain.SevRun, "ai_not_connected", "%s에 사용할 수 있는 AI 연결이 없습니다", a.DisplayName))
			} else {
				view.ConnectionName, view.Connected = ci.Name, true
			}
			view.ProposedAssignment = *a
		default:
			add(issue(domain.SevError, "bad_actor_kind", "담당자 %q의 종류 %q는 ai 또는 human이어야 합니다", a.DisplayName, a.ActorKind))
			continue
		}
		byRef[a.Ref] = a
		infos[a.Ref] = domain.AssignmentInfo{ActorKind: a.ActorKind, Connected: view.Connected}
		diff.Assignments = append(diff.Assignments, view)
	}

	// Workflow: the same checks as a hand-made workflow.
	issues = append(issues, domain.ValidateStructure(&p.Workflow)...)
	issues = append(issues, domain.ValidateAssignments(&p.Workflow, infos)...)
	for _, n := range p.Workflow.Nodes {
		v := StepView{ID: n.ID, Title: n.Title, Kind: string(n.Kind)}
		if a := byRef[n.AssignmentID]; a != nil {
			v.Assignee, v.Human = a.DisplayName, a.ActorKind == "human"
		}
		diff.Steps = append(diff.Steps, v)
	}

	// The person's own tasks must really be assigned to the person.
	steps := map[string]*domain.Node{}
	for i := range p.Workflow.Nodes {
		steps[p.Workflow.Nodes[i].ID] = &p.Workflow.Nodes[i]
	}
	covered := map[string]bool{}
	for _, h := range p.HumanSteps {
		n := steps[h.StepID]
		if n == nil {
			add(issue(domain.SevError, "unknown_human_step", "사람 업무 %q가 가리키는 업무 %q가 흐름에 없습니다", h.Request, h.StepID))
			continue
		}
		if a := byRef[n.AssignmentID]; a == nil || a.ActorKind != "human" {
			add(issue(domain.SevError, "human_step_not_human", "%q(업무 %q)가 사람에게 배정되지 않았습니다", h.Request, n.Title))
			continue
		}
		covered[norm(h.Request)] = true
	}
	for _, task := range c.HumanTasks {
		if strings.TrimSpace(task) != "" && !covered[norm(task)] {
			add(issue(domain.SevError, "human_task_missing", "직접 맡겠다고 한 %q가 설계안에서 사람 업무로 지정되지 않았습니다", task))
		}
	}

	diff.Missing = append(diff.Missing, p.Missing...)
	for _, m := range p.Missing {
		add(issue(domain.SevRun, "missing_"+m.Kind, "부족한 항목: %s", m.Detail))
	}

	sort.SliceStable(issues, func(i, j int) bool { return issues[i].Severity < issues[j].Severity })
	r := Result{Proposal: p, Issues: issues, Diff: diff}
	r.CanApply = !domain.HasSeverity(issues, domain.SevError)
	r.CanStart = r.CanApply && !domain.HasSeverity(issues, domain.SevRun)
	if r.Issues == nil {
		r.Issues = []domain.Issue{}
	}
	return r
}

func newRoleCycle(roles []ProposedRole) string {
	parent := map[string]string{}
	for _, r := range roles {
		parent[r.Ref] = r.ParentRef
	}
	for _, r := range roles {
		seen := map[string]bool{}
		for cur := r.Ref; cur != ""; cur = parent[cur] {
			if seen[cur] {
				return cur
			}
			seen[cur] = true
		}
	}
	return ""
}

// WorkflowWithIDs returns the workflow JSON with assignment refs replaced
// by real assignment ids.
func WorkflowWithIDs(w domain.WorkflowSpec, ids map[string]string) (json.RawMessage, error) {
	out := w
	out.Nodes = append([]domain.Node(nil), w.Nodes...)
	for i := range out.Nodes {
		if id, ok := ids[out.Nodes[i].AssignmentID]; ok {
			out.Nodes[i].AssignmentID = id
		}
	}
	return json.Marshal(out)
}
