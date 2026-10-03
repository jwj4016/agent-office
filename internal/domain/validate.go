package domain

import (
	"encoding/json"
	"fmt"
	"sort"
)

// Severity says what an issue blocks.
type Severity string

const (
	// SevError blocks confirming a workflow version. Drafts still save.
	SevError Severity = "error"
	// SevRun allows a version but blocks starting a run (e.g. an AI
	// assignment without a working connection, T04).
	SevRun Severity = "run"
)

// Issue is one validation finding. Code is stable for tests and UI
// logic; Message is shown to the user.
type Issue struct {
	NodeID   string   `json:"nodeId,omitempty"`
	Field    string   `json:"field,omitempty"`
	Code     string   `json:"code"`
	Message  string   `json:"message"`
	Severity Severity `json:"severity"`
}

func (i Issue) String() string {
	return fmt.Sprintf("%s[%s] %s: %s", i.NodeID, i.Field, i.Code, i.Message)
}

// HasErrors reports whether any issue has the given severity.
func HasSeverity(issues []Issue, sev Severity) bool {
	for _, i := range issues {
		if i.Severity == sev {
			return true
		}
	}
	return false
}

type validator struct {
	w      *WorkflowSpec
	byID   map[string]*Node
	issues []Issue
	anc    map[string]map[string]bool
}

func (v *validator) add(node, field, code, format string, args ...any) {
	v.issues = append(v.issues, Issue{NodeID: node, Field: field, Code: code, Message: fmt.Sprintf(format, args...), Severity: SevError})
}

// ValidateStructure checks the graph on its own (spec §6.3): ids, kinds,
// DAG, input/output references, rework targets and branch shape.
func ValidateStructure(w *WorkflowSpec) []Issue {
	v := &validator{w: w, byID: map[string]*Node{}}
	if w.SchemaVersion != WorkflowSchemaVersion {
		v.add("", "schemaVersion", "schema_version", "지원하지 않는 schemaVersion %d 입니다 (지원: %d)", w.SchemaVersion, WorkflowSchemaVersion)
	}
	if len(w.Nodes) == 0 {
		v.add("", "nodes", "empty", "업무가 하나도 없습니다")
		return v.issues
	}
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if !idPattern.MatchString(n.ID) {
			v.add(n.ID, "id", "bad_id", "업무 ID %q 형식이 올바르지 않습니다", n.ID)
			continue
		}
		if v.byID[n.ID] != nil {
			v.add(n.ID, "id", "duplicate_id", "업무 ID %q가 중복됩니다", n.ID)
			continue
		}
		v.byID[n.ID] = n
	}
	for i := range w.Nodes {
		v.checkNode(&w.Nodes[i])
	}
	if !v.checkAcyclic() {
		return v.issues
	}
	v.anc = map[string]map[string]bool{}
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if v.byID[n.ID] == n {
			v.checkReferences(n)
		}
	}
	v.checkBranches()
	return sortIssues(v.issues)
}

// sortIssues orders issues deterministically for display and tests.
func sortIssues(issues []Issue) []Issue {
	sort.SliceStable(issues, func(i, j int) bool {
		a, b := issues[i], issues[j]
		if a.NodeID != b.NodeID {
			return a.NodeID < b.NodeID
		}
		return a.Code < b.Code
	})
	return issues
}

func (v *validator) checkNode(n *Node) {
	if n.Title == "" {
		v.add(n.ID, "title", "missing_title", "업무 이름이 없습니다")
	}
	seenDep := map[string]bool{}
	for _, d := range n.DependsOn {
		switch {
		case d == n.ID:
			v.add(n.ID, "dependsOn", "self_dependency", "자기 자신을 선행 업무로 지정할 수 없습니다")
		case v.byID[d] == nil:
			v.add(n.ID, "dependsOn", "unknown_dependency", "선행 업무 %q가 없습니다", d)
		case seenDep[d]:
			v.add(n.ID, "dependsOn", "duplicate_dependency", "선행 업무 %q가 중복됩니다", d)
		}
		seenDep[d] = true
	}
	keys := map[string]bool{}
	for _, o := range n.Outputs {
		if !idPattern.MatchString(o.Key) {
			v.add(n.ID, "outputs", "bad_output_key", "결과 키 %q 형식이 올바르지 않습니다", o.Key)
		} else if keys[o.Key] {
			v.add(n.ID, "outputs", "duplicate_output_key", "결과 키 %q가 중복됩니다", o.Key)
		}
		keys[o.Key] = true
		if !outputTypes[o.Type] {
			v.add(n.ID, "outputs", "bad_output_type", "결과 형식 %q는 지원하지 않습니다", o.Type)
		}
		if len(o.Schema) > 0 {
			var obj map[string]any
			if json.Unmarshal(o.Schema, &obj) != nil {
				v.add(n.ID, "outputs", "bad_output_schema", "결과 %q의 schema는 JSON 객체여야 합니다", o.Key)
			}
		}
	}
	if n.Limits != nil {
		if err := parseDuration(n.Limits.Timeout); err != nil {
			v.add(n.ID, "limits", "bad_timeout", "제한 시간 %q가 올바르지 않습니다", n.Limits.Timeout)
		}
		for name, p := range map[string]*int{"maxRevisions": n.Limits.MaxRevisions, "maxRetries": n.Limits.MaxRetries, "maxTokens": n.Limits.MaxTokens} {
			if p != nil && *p < 0 {
				v.add(n.ID, "limits", "bad_limit", "%s는 0 이상이어야 합니다", name)
			}
		}
	}
	if n.Completion != nil {
		for _, c := range n.Completion.Commands {
			if c.Executable == "" {
				v.add(n.ID, "completion", "bad_command", "검증 명령의 실행 파일이 없습니다")
			}
			if err := parseDuration(c.Timeout); err != nil {
				v.add(n.ID, "completion", "bad_timeout", "검증 명령 제한 시간 %q가 올바르지 않습니다", c.Timeout)
			}
		}
	}

	switch n.Kind {
	case KindTask, KindReview:
		if len(n.Outputs) == 0 {
			v.add(n.ID, "outputs", "missing_outputs", "결과를 하나 이상 정의해야 합니다")
		}
	case KindApproval:
		if len(n.Outputs) > 0 {
			v.add(n.ID, "outputs", "approval_outputs", "승인 업무는 결과를 만들지 않습니다")
		}
	case KindCondition, KindJoin:
		if n.AssignmentID != "" {
			v.add(n.ID, "assignmentId", "engine_node_assignment", "조건·합류 업무에는 담당자를 지정하지 않습니다")
		}
		if len(n.Outputs) > 0 || len(n.Inputs) > 0 {
			v.add(n.ID, "outputs", "engine_node_io", "조건·합류 업무는 입력·결과를 갖지 않습니다")
		}
	default:
		v.add(n.ID, "kind", "bad_kind", "업무 종류 %q는 지원하지 않습니다", n.Kind)
	}
	if n.Kind == KindCondition && n.Routing == nil {
		v.add(n.ID, "routing", "missing_routing", "조건 업무에는 분기 규칙이 필요합니다")
	}
	if n.Kind != KindCondition && n.Routing != nil {
		v.add(n.ID, "routing", "unexpected_routing", "분기 규칙은 조건 업무에만 지정합니다")
	}
	if len(n.ReworkTargets) > 0 && n.Kind != KindReview && n.Kind != KindApproval {
		v.add(n.ID, "reworkTargets", "rework_on_wrong_kind", "수정 요청 대상은 리뷰·승인 업무에만 지정합니다")
	}
}

// checkAcyclic reports cycles with Kahn's algorithm.
func (v *validator) checkAcyclic() bool {
	indeg := map[string]int{}
	children := map[string][]string{}
	for id, n := range v.byID {
		indeg[id] += 0
		for _, d := range n.DependsOn {
			if v.byID[d] != nil && d != id {
				indeg[id]++
				children[d] = append(children[d], id)
			}
		}
	}
	var queue []string
	for id, d := range indeg {
		if d == 0 {
			queue = append(queue, id)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, c := range children[id] {
			if indeg[c]--; indeg[c] == 0 {
				queue = append(queue, c)
			}
		}
	}
	if visited == len(indeg) {
		return true
	}
	var cyc []string
	for id, d := range indeg {
		if d > 0 {
			cyc = append(cyc, id)
		}
	}
	sort.Strings(cyc)
	v.add(cyc[0], "dependsOn", "cycle", "선행 관계에 순환이 있습니다: %v (반려는 연결선이 아니라 수정 요청 대상으로 지정하세요)", cyc)
	return false
}

// ancestors returns every transitive dependency of id. Requires a DAG.
func (v *validator) ancestors(id string) map[string]bool {
	if a, ok := v.anc[id]; ok {
		return a
	}
	a := map[string]bool{}
	if n := v.byID[id]; n != nil {
		for _, d := range n.DependsOn {
			if v.byID[d] == nil || d == id {
				continue
			}
			a[d] = true
			for x := range v.ancestors(d) {
				a[x] = true
			}
		}
	}
	v.anc[id] = a
	return a
}

func (v *validator) output(step, key string) *Output {
	n := v.byID[step]
	if n == nil {
		return nil
	}
	for i := range n.Outputs {
		if n.Outputs[i].Key == key {
			return &n.Outputs[i]
		}
	}
	return nil
}

func (v *validator) checkReferences(n *Node) {
	anc := v.ancestors(n.ID)
	for _, in := range n.Inputs {
		switch {
		case !anc[in.FromStep]:
			v.add(n.ID, "inputs", "input_not_ancestor", "입력 %q의 출처 %q는 선행 업무여야 합니다", in.Name, in.FromStep)
		case v.output(in.FromStep, in.OutputKey) == nil:
			v.add(n.ID, "inputs", "unknown_output", "업무 %q에 결과 %q가 없습니다", in.FromStep, in.OutputKey)
		}
	}
	for _, t := range n.ReworkTargets {
		tn := v.byID[t]
		switch {
		case tn == nil || !anc[t]:
			v.add(n.ID, "reworkTargets", "rework_not_ancestor", "수정 요청 대상 %q는 선행 업무여야 합니다", t)
		case tn.Kind != KindTask && tn.Kind != KindReview:
			v.add(n.ID, "reworkTargets", "rework_bad_target", "수정 요청 대상 %q는 작업·리뷰 업무여야 합니다", t)
		}
	}
	if r := n.Routing; r != nil {
		src := r.Source
		switch {
		case !anc[src.FromStep]:
			v.add(n.ID, "routing", "route_source_not_ancestor", "분기 기준 %q는 선행 업무여야 합니다", src.FromStep)
		default:
			o := v.output(src.FromStep, src.OutputKey)
			if o == nil {
				v.add(n.ID, "routing", "unknown_output", "업무 %q에 결과 %q가 없습니다", src.FromStep, src.OutputKey)
			} else if o.Type != OutJSON && o.Type != OutReport {
				v.add(n.ID, "routing", "route_source_not_json", "분기 기준 결과는 json 또는 report 형식이어야 합니다")
			} else if !o.IsRequired() {
				v.add(n.ID, "routing", "route_source_optional", "분기 기준 결과는 필수 결과여야 합니다")
			}
		}
		if !fieldPathPattern.MatchString(src.FieldPath) {
			v.add(n.ID, "routing", "bad_field_path", "fieldPath %q 형식이 올바르지 않습니다", src.FieldPath)
		}
		if len(r.Branches) == 0 {
			v.add(n.ID, "routing", "no_branches", "분기가 하나 이상 필요합니다")
		}
		for _, b := range r.Branches {
			switch b.Operator {
			case OpEq, OpNe:
				if len(b.Value) == 0 {
					v.add(n.ID, "routing", "missing_value", "%s 비교에는 value가 필요합니다", b.Operator)
				}
			case OpIn:
				var arr []any
				if json.Unmarshal(b.Value, &arr) != nil {
					v.add(n.ID, "routing", "bad_in_value", "in 비교의 value는 배열이어야 합니다")
				}
			case OpExists:
			default:
				v.add(n.ID, "routing", "bad_operator", "비교 방식 %q는 지원하지 않습니다 (eq, ne, in, exists)", b.Operator)
			}
		}
	}
}

// checkBranches enforces the first-release branch shape (spec §6.1):
// every branch starts right after its condition, depends only on itself,
// meets the others at the join node and contains no nested condition.
func (v *validator) checkBranches() {
	for i := range v.w.Nodes {
		c := &v.w.Nodes[i]
		if c.Kind != KindCondition || c.Routing == nil || v.byID[c.ID] != c {
			continue
		}
		r := c.Routing
		join := v.byID[r.JoinStep]
		if join == nil || join.Kind != KindJoin {
			v.add(c.ID, "routing", "bad_join", "joinStep %q는 같은 흐름의 합류 업무여야 합니다", r.JoinStep)
			continue
		}
		targets := map[string]bool{}
		for _, t := range append(branchTargets(r), r.DefaultTarget) {
			if t == "" {
				v.add(c.ID, "routing", "missing_target", "분기 대상과 defaultTarget을 모두 지정해야 합니다")
				continue
			}
			if v.byID[t] == nil {
				v.add(c.ID, "routing", "unknown_target", "분기 대상 %q가 없습니다", t)
				continue
			}
			targets[t] = true
		}
		// Every child of the condition is a branch start and vice versa.
		for id, n := range v.byID {
			if contains(n.DependsOn, c.ID) && !targets[id] {
				v.add(id, "dependsOn", "untargeted_child", "조건 %q 뒤의 업무는 분기 대상으로 지정해야 합니다", c.ID)
			}
		}
		inBranch := map[string]string{} // node -> branch start
		for t := range targets {
			tn := v.byID[t]
			if !contains(tn.DependsOn, c.ID) {
				v.add(t, "dependsOn", "target_missing_condition", "분기 대상은 조건 %q를 선행 업무로 가져야 합니다", c.ID)
				continue
			}
			if t == join.ID {
				continue // empty branch: go straight to the join
			}
			if len(tn.DependsOn) != 1 {
				v.add(t, "dependsOn", "ambiguous_branch_dependency", "분기 시작 업무는 조건 %q만 선행 업무로 가져야 합니다", c.ID)
			}
			set := v.branchClosure(t, join.ID)
			reachesJoin := false
			for id := range set {
				if other, ok := inBranch[id]; ok && other != t {
					v.add(id, "dependsOn", "overlapping_branches", "업무 %q가 두 분기에 동시에 속합니다", id)
				}
				inBranch[id] = t
				if contains(join.DependsOn, id) {
					reachesJoin = true
				}
				if v.byID[id].Kind == KindCondition {
					v.add(id, "kind", "nested_condition", "첫 버전에서는 분기 안의 추가 분기를 지원하지 않습니다")
				}
			}
			if !reachesJoin {
				v.add(t, "routing", "branch_not_joined", "분기 %q가 합류 업무 %q로 이어지지 않습니다", t, join.ID)
			}
			for id := range set {
				for _, d := range v.byID[id].DependsOn {
					if id != t && !set[d] {
						v.add(id, "dependsOn", "ambiguous_branch_dependency", "분기 안의 업무가 분기 밖 업무 %q에 의존합니다", d)
					}
				}
			}
		}
		for _, d := range join.DependsOn {
			if _, ok := inBranch[d]; !ok && !(d == c.ID && targets[join.ID]) {
				v.add(join.ID, "dependsOn", "join_mixed_dependencies", "합류 업무는 조건 %q의 분기만 선행 업무로 가집니다 (%q)", c.ID, d)
			}
		}
		// Nodes outside a branch must not require that branch's outputs:
		// the branch may be skipped (first release rejects this outright).
		for id, n := range v.byID {
			for _, in := range n.Inputs {
				b, fromBranch := inBranch[in.FromStep]
				if fromBranch && inBranch[id] != b && in.IsRequired() {
					v.add(id, "inputs", "ambiguous_required_input", "필수 입력 %q는 선택되지 않을 수 있는 분기의 결과입니다", in.Name)
				}
			}
		}
	}
}

// branchClosure collects nodes downstream of start up to (excluding) join.
func (v *validator) branchClosure(start, join string) map[string]bool {
	set := map[string]bool{start: true}
	for changed := true; changed; {
		changed = false
		for id, n := range v.byID {
			if set[id] || id == join {
				continue
			}
			for _, d := range n.DependsOn {
				if set[d] {
					set[id] = true
					changed = true
					break
				}
			}
		}
	}
	return set
}

func branchTargets(r *Routing) []string {
	out := make([]string, 0, len(r.Branches))
	for _, b := range r.Branches {
		out = append(out, b.TargetStep)
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// AssignmentInfo is what the validator needs to know about an assignment
// of the workflow's project.
type AssignmentInfo struct {
	ActorKind string // "ai" | "human"
	// Connected is true when an AI assignment has a usable connection.
	Connected bool
}

// ValidateAssignments checks node assignments against the project's
// assignments. Missing AI connections are SevRun: the version can be
// saved but no run may start (T04).
func ValidateAssignments(w *WorkflowSpec, assignments map[string]AssignmentInfo) []Issue {
	var issues []Issue
	add := func(n *Node, sev Severity, code, format string, args ...any) {
		issues = append(issues, Issue{NodeID: n.ID, Field: "assignmentId", Code: code, Message: fmt.Sprintf(format, args...), Severity: sev})
	}
	for i := range w.Nodes {
		n := &w.Nodes[i]
		if n.Kind == KindCondition || n.Kind == KindJoin {
			continue
		}
		if n.AssignmentID == "" {
			add(n, SevError, "missing_assignment", "담당자가 지정되지 않았습니다")
			continue
		}
		a, ok := assignments[n.AssignmentID]
		if !ok {
			add(n, SevError, "unknown_assignment", "담당자 %q는 이 프로젝트에 없습니다", n.AssignmentID)
			continue
		}
		if n.Kind == KindApproval && a.ActorKind != "human" {
			add(n, SevError, "approval_needs_human", "승인은 사람만 할 수 있습니다")
			continue
		}
		if a.ActorKind == "ai" && !a.Connected {
			add(n, SevRun, "ai_not_connected", "AI 담당자의 연결이 설정·확인되지 않아 실행할 수 없습니다")
		}
	}
	return issues
}
