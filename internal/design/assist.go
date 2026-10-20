package design

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// Workflow assistant jobs: an AI-proposed edit of a workflow draft, or a
// pipeline diagram of it made with the bundled diagram-design skill.
const (
	AssistEdit    = "edit"
	AssistDiagram = "diagram"

	AssistWorking   = "working"
	AssistReady     = "ready"
	AssistFailed    = "failed"
	AssistApplied   = "applied"
	AssistDiscarded = "discarded"
)

// AssistInput is what the person asked for.
type AssistInput struct {
	ProjectID    string `json:"projectId"`
	WorkflowID   string `json:"workflowId"`
	Kind         string `json:"kind"`
	Request      string `json:"request"`
	ConnectionID string `json:"connectionId"`
	Model        string `json:"model"`
	ParentID     string `json:"parentId"`
	ScopeNodeID  string `json:"scopeNodeId"`
}

// EditResult is a proposed draft with its check and what it changes.
type EditResult struct {
	Workflow   json.RawMessage `json:"workflow"`
	Notes      string          `json:"notes"`
	Issues     []domain.Issue  `json:"issues"`
	CanVersion bool            `json:"canVersion"`
	Changes    AssistChanges   `json:"changes"`
}

// AssistChanges lists step titles added, removed and changed.
type AssistChanges struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Changed []string `json:"changed"`
}

// DiagramResult points at the diagram file inside the data folder.
type DiagramResult struct {
	Path string `json:"path"`
	// Checked is true when the skill's self-check ran (Python found).
	Checked bool `json:"checked"`
}

const assistInstructions = `너는 Agent Office의 업무 흐름 도우미다. 사람이 요청한 대로 기존 업무 흐름을 고치거나 그림으로 그린다.
결과는 제안일 뿐이며 사람이 검토한다. 도구 권한, API 키, 연결, 새 담당자를 만들거나 요구하지 않는다.`

// StartAssist records a job and runs it in the background; the record
// moves to ready or failed.
func (s *Service) StartAssist(ctx context.Context, in AssistInput) (storage.AssistRecord, error) {
	in.Request = strings.TrimSpace(in.Request)
	switch in.Kind {
	case AssistEdit:
		if in.Request == "" {
			return storage.AssistRecord{}, fmt.Errorf("%w: 어떻게 고칠지 입력하세요", ErrInvalid)
		}
	case AssistDiagram:
	default:
		return storage.AssistRecord{}, fmt.Errorf("%w: 알 수 없는 요청 종류 %q", ErrInvalid, in.Kind)
	}
	p, err := s.DB.Project(ctx, in.ProjectID)
	if err != nil {
		return storage.AssistRecord{}, err
	}
	if p.Status != "active" {
		return storage.AssistRecord{}, storage.ErrArchived
	}
	w, err := s.DB.Workflow(ctx, in.ProjectID, in.WorkflowID)
	if err != nil {
		return storage.AssistRecord{}, err
	}
	base := w.Draft
	if in.ParentID != "" {
		parent, err := s.DB.Assist(ctx, in.ParentID)
		if err != nil {
			return storage.AssistRecord{}, err
		}
		if parent.ProjectID != in.ProjectID || parent.WorkflowID != in.WorkflowID {
			return storage.AssistRecord{}, storage.ErrNotInProject
		}
		if in.Kind != AssistEdit || parent.Kind != AssistEdit || parent.Status != AssistReady {
			return storage.AssistRecord{}, ErrNotReady
		}
		if parent.BaseRevision != w.Revision {
			return storage.AssistRecord{}, ErrAssistConflict
		}
		var proposal EditResult
		if err := json.Unmarshal(parent.Result, &proposal); err != nil {
			return storage.AssistRecord{}, err
		}
		w.Draft = proposal.Workflow
		if len(parent.BaseWorkflow) > 2 {
			base = parent.BaseWorkflow
		}
		in.ScopeNodeID = parent.ScopeNodeID
	}
	if in.ScopeNodeID != "" {
		spec, err := domain.ParseWorkflow(w.Draft)
		if err != nil {
			return storage.AssistRecord{}, err
		}
		found := false
		for _, n := range spec.Nodes {
			if n.ID == in.ScopeNodeID {
				found = true
			}
		}
		if in.Kind != AssistEdit || !found {
			return storage.AssistRecord{}, fmt.Errorf("%w: 수정할 업무를 찾을 수 없습니다", ErrInvalid)
		}
	}
	conn, err := s.DB.Connection(ctx, in.ConnectionID)
	if err != nil {
		return storage.AssistRecord{}, err
	}
	if !(conn.Provider == "test" || conn.Verified()) {
		return storage.AssistRecord{}, fmt.Errorf("%w: 이 연결은 실제 호출 시험을 통과하지 않았습니다", ErrInvalid)
	}
	if in.Kind == AssistDiagram && (conn.Provider == "claude_api" || conn.Provider == "openai_api") {
		return storage.AssistRecord{}, fmt.Errorf("%w: 다이어그램은 스킬 파일을 읽을 수 있는 Claude·Codex 연결로만 만들 수 있습니다", ErrInvalid)
	}
	rec, err := s.DB.CreateAssist(ctx, storage.AssistRecord{ProjectID: in.ProjectID, WorkflowID: in.WorkflowID, Kind: in.Kind,
		Request: in.Request, BaseRevision: w.Revision, ConnectionID: in.ConnectionID, Model: in.Model,
		BaseWorkflow: base, ParentID: in.ParentID, ScopeNodeID: in.ScopeNodeID})
	if err != nil {
		return rec, err
	}
	s.assistEvent(rec, "assist.request", map[string]any{"text": in.Request})
	workCtx := s.Ctx
	if workCtx == nil {
		workCtx = context.Background()
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.assist(workCtx, rec, p, w, conn)
	}()
	return rec, nil
}

func (s *Service) assist(ctx context.Context, rec storage.AssistRecord, p domain.Project, w domain.Workflow, conn domain.ProviderConnection) {
	fail := func(err error) {
		if _, uerr := s.DB.UpdateAssist(context.Background(), rec.ID, AssistFailed, nil, err.Error(), AssistWorking); uerr != nil {
			s.logf("assist %s: %v", rec.ID, uerr)
		}
		s.assistEvent(rec, "assist.failed", map[string]any{"text": err.Error()})
	}
	prov, err := s.Providers(conn)
	if err != nil {
		fail(err)
		return
	}
	team, err := s.DB.Assignments(ctx, p.ID)
	if err != nil {
		fail(err)
		return
	}
	conns, err := s.Connections(ctx)
	if err != nil {
		fail(err)
		return
	}
	var result any
	if rec.Kind == AssistEdit {
		result, err = s.proposeEdit(ctx, prov, rec, w, team, conns)
	} else {
		result, err = s.drawPipeline(ctx, prov, rec, p, w, team)
	}
	if err != nil {
		fail(err)
		return
	}
	raw, _ := json.Marshal(result)
	if _, err := s.DB.UpdateAssist(context.Background(), rec.ID, AssistReady, raw, "", AssistWorking); err != nil {
		s.logf("assist %s: %v", rec.ID, err)
	}
	s.assistEvent(rec, "assist.ready", map[string]any{"text": "결과가 준비되었습니다. 검토 후 적용하세요."})
}

func (s *Service) assistEvent(rec storage.AssistRecord, kind string, payload map[string]any) error {
	payload["assistId"] = rec.ID
	_, err := s.DB.Change(context.Background(), func(c *storage.Change) error { return c.Emit(rec.ProjectID, "", "", kind, payload) })
	return err
}

func (s *Service) assistRecorder(rec storage.AssistRecord) (*storage.OutputRecorder, func(providers.Event) error) {
	r := storage.NewOutputRecorder(func(text string) error { return s.assistEvent(rec, "activity.output", map[string]any{"text": text}) })
	return r, func(ev providers.Event) error {
		if err := r.Add(ev); err != nil {
			return err
		}
		switch ev.Kind {
		case providers.KindMessage, providers.KindTool, providers.KindError, providers.KindCompleted, providers.KindQuestion, providers.KindApprovalRequest:
			var p map[string]any
			if err := json.Unmarshal(ev.Payload, &p); err != nil {
				return err
			}
			return s.assistEvent(rec, "provider."+ev.Kind, p)
		}
		return nil
	}
}

// teamList describes the service's assignees for a prompt.
func teamList(team []domain.Assignment, conns []ConnectionInfo) string {
	byID := map[string]ConnectionInfo{}
	for _, c := range conns {
		byID[c.ID] = c
	}
	var b strings.Builder
	for _, a := range team {
		kind := "사람"
		if a.ActorKind == domain.ActorAI {
			kind = "AI"
			if c, ok := byID[a.ConnectionID]; ok {
				kind += ", 연결 " + c.Name
				if c.Coding {
					kind += ", 코드 작성 가능"
				}
			}
		}
		fmt.Fprintf(&b, "- id=%s 이름=%s (%s)\n", a.ID, a.DisplayName, kind)
	}
	if len(team) == 0 {
		b.WriteString("없음\n")
	}
	return b.String()
}

func editSchema() map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "required": []string{"workflow", "notes"},
		"properties": map[string]any{
			"workflow": map[string]any{"type": "object", "description": "고친 업무 흐름 전체 (schemaVersion 1)"},
			"notes":    map[string]any{"type": "string", "description": "무엇을 왜 바꿨는지 한국어로 짧게"},
		}}
}

func (s *Service) proposeEdit(ctx context.Context, prov providers.Provider, rec storage.AssistRecord, w domain.Workflow, team []domain.Assignment, conns []ConnectionInfo) (EditResult, error) {
	var current any
	json.Unmarshal(w.Draft, &current)
	pretty, _ := json.MarshalIndent(current, "", "  ")
	var b strings.Builder
	fmt.Fprintf(&b, "# 요청\n%s\n\n# 지금 업무 흐름 (수정 %d)\n```json\n%s\n```\n\n", rec.Request, w.Revision, pretty)
	if rec.ParentID != "" {
		parent, err := s.DB.Assist(ctx, rec.ParentID)
		if err != nil {
			return EditResult{}, err
		}
		fmt.Fprintf(&b, "# 이전 요청과 제안\n이전 요청: %s\n위 흐름은 이전 제안이다. 이번 요청으로 그 제안을 추가 수정한다.\n", parent.Request)
	}
	if rec.ScopeNodeID != "" {
		fmt.Fprintf(&b, "# 수정 범위\nid=%s 업무만 수정한다. 다른 업무의 필드·순서와 흐름 제목은 그대로 유지한다. 업무 추가·삭제는 하지 않는다.\n", rec.ScopeNodeID)
	}
	b.WriteString("# 이 서비스의 담당자 (assignmentId는 여기 있는 id만 쓴다)\n" + teamList(team, conns) + "\n")
	b.WriteString(workflowFormat("위 담당자 id") + workflowRules)
	b.WriteString(`- 요청과 관계없는 업무는 id·내용을 그대로 둔다. 바꾸지 않은 업무의 id를 바꾸지 않는다.
- 기존 condition·join 업무는 유지해도 되지만 새로 만들지 않는다. 그 밖의 기존 필드(completion, limits, routing, meeting)도 필요할 때만 바꾼다.
- 맡길 담당자가 없으면 assignmentId를 비우고 notes에 어떤 담당자가 필요한지 적는다.

# 출력
결과 파일에 {"workflow": 고친 업무 흐름 전체, "notes": 바꾼 내용 요약} 하나만 쓴다.
`)
	schema, _ := json.Marshal(editSchema())
	recorder, onEvent := s.assistRecorder(rec)
	defer recorder.Close()
	raw, err := runJSON(ctx, prov, agentJob{
		OnEvent: onEvent,
		What:    "흐름 수정 AI", Instructions: assistInstructions, Prompt: b.String(), Model: rec.Model,
		Key: "edit", Type: "json", File: "edit.json", Schema: schema, Timeout: 10 * time.Minute,
		Answer: "질문하지 말고 합리적인 가정을 정해 notes에 적어 주세요.",
	}, "필요한 정보는 모두 위에 있으므로 다른 파일을 읽거나 명령을 실행하지 않는다. ", "수정안")
	if err != nil {
		return EditResult{}, err
	}
	var out struct {
		Workflow json.RawMessage `json:"workflow"`
		Notes    Text            `json:"notes"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return EditResult{}, fmt.Errorf("수정안을 읽을 수 없습니다: %v (받은 내용 앞부분: %q)", err, clip(string(raw), 200))
	}
	if len(out.Workflow) == 0 || out.Workflow[0] != '{' {
		return EditResult{}, fmt.Errorf("수정안에 workflow 객체가 없습니다 (받은 내용 앞부분: %q)", clip(string(raw), 200))
	}
	spec, err := domain.ParseWorkflow(out.Workflow)
	if err != nil {
		return EditResult{}, fmt.Errorf("수정안의 업무 흐름을 읽을 수 없습니다: %v", err)
	}
	if rec.ScopeNodeID != "" {
		old, err := domain.ParseWorkflow(w.Draft)
		if err != nil {
			return EditResult{}, err
		}
		if len(old.Nodes) != len(spec.Nodes) || old.Title != spec.Title {
			return EditResult{}, fmt.Errorf("%w: 선택한 업무 밖의 변경이 있어 적용할 수 없습니다. 전체 흐름으로 다시 요청하세요", ErrInvalid)
		}
		for i, n := range old.Nodes {
			if spec.Nodes[i].ID != n.ID || (n.ID != rec.ScopeNodeID && !reflect.DeepEqual(n, spec.Nodes[i])) {
				return EditResult{}, fmt.Errorf("%w: 선택한 업무 밖의 변경이 있습니다: %s", ErrInvalid, n.Title)
			}
		}
	}
	draft, _ := json.Marshal(spec)
	check, err := s.DB.ValidateDraft(ctx, rec.ProjectID, draft)
	if err != nil {
		return EditResult{}, err
	}
	old, _ := domain.ParseWorkflow(rec.BaseWorkflow)
	return EditResult{Workflow: draft, Notes: string(out.Notes), Issues: check.Issues, CanVersion: check.CanVersion, Changes: diffSteps(old, spec)}, nil
}

// diffSteps compares two drafts step by step.
func diffSteps(old, next *domain.WorkflowSpec) AssistChanges {
	ch := AssistChanges{Added: []string{}, Removed: []string{}, Changed: []string{}}
	before := map[string]domain.Node{}
	if old != nil {
		for _, n := range old.Nodes {
			before[n.ID] = n
		}
	}
	seen := map[string]bool{}
	for _, n := range next.Nodes {
		seen[n.ID] = true
		o, ok := before[n.ID]
		switch {
		case !ok:
			ch.Added = append(ch.Added, n.Title)
		default:
			a, _ := json.Marshal(o)
			b, _ := json.Marshal(n)
			if string(a) != string(b) {
				ch.Changed = append(ch.Changed, n.Title)
			}
		}
	}
	if old != nil {
		for _, n := range old.Nodes {
			if !seen[n.ID] {
				ch.Removed = append(ch.Removed, n.Title)
			}
		}
	}
	return ch
}

func (s *Service) drawPipeline(ctx context.Context, prov providers.Provider, rec storage.AssistRecord, p domain.Project, w domain.Workflow, team []domain.Assignment) (DiagramResult, error) {
	spec, err := domain.ParseWorkflow(w.Draft)
	if err != nil {
		return DiagramResult{}, fmt.Errorf("업무 흐름을 읽을 수 없습니다: %v", err)
	}
	names := map[string]string{}
	for _, a := range team {
		who := "사람"
		if a.ActorKind == domain.ActorAI {
			who = "AI"
		}
		names[a.ID] = a.DisplayName + " (" + who + ")"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# 업무\n서비스 %q의 업무 흐름 %q(수정 %d)을 파이프라인 다이어그램 한 장으로 그린다.\n", p.Name, spec.Title, w.Revision)
	if p.Goal != "" {
		fmt.Fprintf(&b, "서비스 목표: %s\n", p.Goal)
	}
	if rec.Request != "" {
		fmt.Fprintf(&b, "사람의 추가 요청: %s\n", rec.Request)
	}
	b.WriteString("\n# 업무 목록 (선행 업무 → 업무 순서로 진행, 선행이 없으면 시작 업무)\n")
	for _, n := range spec.Nodes {
		who := names[n.AssignmentID]
		if who == "" {
			who = "담당자 없음"
		}
		fmt.Fprintf(&b, "- id=%s %q 종류=%s 담당=%s", n.ID, n.Title, n.Kind, who)
		if len(n.DependsOn) > 0 {
			fmt.Fprintf(&b, " 선행=%s", strings.Join(n.DependsOn, ","))
		}
		if len(n.Outputs) > 0 {
			var outs []string
			for _, o := range n.Outputs {
				outs = append(outs, o.Key+":"+string(o.Type))
			}
			fmt.Fprintf(&b, " 결과=%s", strings.Join(outs, ","))
		}
		if len(n.ReworkTargets) > 0 {
			fmt.Fprintf(&b, " 반려시_되돌림=%s", strings.Join(n.ReworkTargets, ","))
		}
		b.WriteString("\n")
	}
	b.WriteString(`
# 그리는 방법
- 진행 순서와 동시에 진행되는 업무(같은 선행을 가진 업무)가 한눈에 보이게 그린다. 흐름에 맞는 유형(process, swimlane, flowchart 등)을 스킬 안내에 따라 고른다.
- 사람이 맡는 업무와 승인은 AI 업무와 구분되게 표시한다. 반려 시 되돌아가는 흐름은 점선 등 보조 표시로만 그린다.
- 업무가 많아 스킬의 복잡도 한도를 넘으면 단계별로 묶어 한 장에 담는다.
`)
	b.WriteString(s.Diagram.Rules())
	path := ""
	recorder, onEvent := s.assistRecorder(rec)
	defer recorder.Close()
	data, _, err := runAgent(ctx, prov, agentJob{
		OnEvent: onEvent,
		What:    "다이어그램 AI", Instructions: assistInstructions, Prompt: b.String(), Model: rec.Model,
		Key: "pipeline", Type: string(domain.OutDiagram), File: "pipeline.html", Timeout: 15 * time.Minute,
		Answer:  "질문하지 말고 스킬의 기본 스타일로 진행하세요.",
		Prepare: s.Diagram.Grant,
		Verify: func(p string) error {
			path = p
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if err := engine.CheckDiagramFormat(data); err != nil {
				return err
			}
			return s.Diagram.Check(ctx, p)
		},
	}, "다른 파일은 만들지 않는다.")
	if err != nil {
		return DiagramResult{}, err
	}
	if data == nil || path == "" {
		return DiagramResult{}, errors.New("다이어그램 AI가 다이어그램 파일을 만들지 않았습니다")
	}
	rel := filepath.Join("projects", p.ID, "diagrams", rec.ID+".html")
	dst := filepath.Join(s.DataDir, rel)
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return DiagramResult{}, err
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return DiagramResult{}, err
	}
	return DiagramResult{Path: filepath.ToSlash(rel), Checked: s.Diagram.Plugin != "" && s.Diagram.Python != ""}, nil
}

// ErrAssistConflict means the workflow changed after the edit was asked.
var ErrAssistConflict = errors.New("업무 흐름이 요청 뒤에 바뀌었습니다. 지금 흐름으로 다시 요청하세요")

// ApplyAssist saves a ready edit as the workflow's new draft, only onto
// the revision it was made from.
func (s *Service) ApplyAssist(ctx context.Context, id string) (domain.Workflow, error) {
	s.applyMu.Lock()
	defer s.applyMu.Unlock()
	rec, err := s.DB.Assist(ctx, id)
	if err != nil {
		return domain.Workflow{}, err
	}
	if rec.Kind != AssistEdit || rec.Status != AssistReady {
		return domain.Workflow{}, ErrNotReady
	}
	var res EditResult
	if err := json.Unmarshal(rec.Result, &res); err != nil {
		return domain.Workflow{}, err
	}
	w, err := s.DB.SaveWorkflowDraft(ctx, rec.ProjectID, rec.WorkflowID, rec.BaseRevision, res.Workflow)
	if errors.Is(err, storage.ErrConflict) {
		return w, ErrAssistConflict
	}
	if err != nil {
		return w, err
	}
	if _, err := s.DB.UpdateAssist(ctx, id, AssistApplied, nil, "", AssistReady); err != nil {
		return w, err
	}
	return w, nil
}

// DiscardAssist drops a job; a result that arrives later is ignored.
func (s *Service) DiscardAssist(ctx context.Context, id string) error {
	ok, err := s.DB.UpdateAssist(ctx, id, AssistDiscarded, nil, "", AssistWorking, AssistReady, AssistFailed)
	if err == nil && !ok {
		return ErrNotReady
	}
	return err
}

// AssistDiagram returns a ready diagram's HTML.
func (s *Service) AssistDiagram(ctx context.Context, id string) (string, error) {
	rec, err := s.DB.Assist(ctx, id)
	if err != nil {
		return "", err
	}
	var res DiagramResult
	json.Unmarshal(rec.Result, &res)
	if rec.Kind != AssistDiagram || res.Path == "" {
		return "", ErrNotReady
	}
	rel := filepath.FromSlash(res.Path)
	if !filepath.IsLocal(rel) {
		return "", errors.New("다이어그램 경로가 올바르지 않습니다")
	}
	data, err := os.ReadFile(filepath.Join(s.DataDir, rel))
	return string(data), err
}

// recoverAssists marks jobs left working by a previous app run as failed.
func (s *Service) recoverAssists(ctx context.Context) error {
	list, err := s.DB.Assists(ctx, "", 1000)
	if err != nil {
		return err
	}
	for _, a := range list {
		if a.Status == AssistWorking {
			if _, err := s.DB.UpdateAssist(ctx, a.ID, AssistFailed, nil, "앱이 종료되어 중단되었습니다. 다시 요청하세요", AssistWorking); err != nil {
				return err
			}
		}
	}
	return nil
}
