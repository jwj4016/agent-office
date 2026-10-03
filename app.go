package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/secrets"
	"agent-office/internal/storage"
	"agent-office/internal/templates"

	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const appVersion = "0.1.0-dev"

// Wails event names.
const (
	EventPing    = "system:ping"
	EventStored  = "events"
	EventDeltas  = "deltas"
	deltaFlushMs = 100
)

var errNotReady = errors.New("저장소가 준비되지 않았습니다")

// App is the only struct bound to the frontend. Its methods are the whole
// UI-facing API: each validates its inputs and checks project scope, and
// none accepts a shell command or an arbitrary file path.
type App struct {
	ctx     context.Context
	emit    func(ctx context.Context, name string, data ...interface{})
	seq     atomic.Int64
	dataDir string
	db      *storage.DB
	eng     *engine.Engine
	secrets secrets.Store
	initErr error

	testProvider *providers.TestProvider
	stopEngine   context.CancelFunc
	engineDone   chan struct{}

	deltaMu sync.Mutex
	deltas  map[string]*Delta
}

func NewApp() *App {
	return &App{emit: runtime.EventsEmit, deltas: map[string]*Delta{}}
}

// dataDir returns AGENT_OFFICE_DATA_DIR when set (tests, side-by-side
// installs), otherwise the per-user config directory.
func dataDir() (string, error) {
	if d := os.Getenv("AGENT_OFFICE_DATA_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "AgentOffice"), nil
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.secrets = secrets.Open()
	dir, err := dataDir()
	if err == nil {
		a.dataDir = dir
		a.db, err = storage.Open(ctx, filepath.Join(dir, "agent-office.db"))
	}
	if err != nil {
		a.initErr = err
		log.Printf("storage init failed: %v", err)
		return
	}
	a.testProvider = &providers.TestProvider{Scripts: map[string][]providers.Step{}}
	a.eng = engine.New(engine.Config{
		DB: a.db, DataDir: dir, Providers: a.providerFor,
		Ephemeral: a.addDelta,
	})
	a.db.Subscribe(func(evs []storage.EventRecord) { a.emit(a.ctx, EventStored, evs) })
	ectx, cancel := context.WithCancel(context.Background())
	a.stopEngine, a.engineDone = cancel, make(chan struct{})
	go func() {
		if err := a.eng.Run(ectx); err != nil {
			log.Printf("engine stopped: %v", err)
		}
		close(a.engineDone)
	}()
	go a.flushDeltas(ectx)
}

func (a *App) onSecondInstance(options.SecondInstanceData) {
	if a.ctx != nil {
		runtime.WindowUnminimise(a.ctx)
		runtime.Show(a.ctx)
	}
}

func (a *App) shutdown(ctx context.Context) {
	if a.stopEngine != nil {
		a.stopEngine()
		<-a.engineDone
	}
	if a.db != nil {
		a.db.Close()
	}
}

func (a *App) providerFor(c domain.ProviderConnection) (providers.Provider, error) {
	switch c.Provider {
	case "test":
		return a.testProvider, nil
	case "codex":
		return &providers.Codex{Executable: c.ExecutablePath, Version: appVersion}, nil
	case "claude":
		var cfg struct {
			Node   string `json:"node"`
			Script string `json:"script"`
		}
		json.Unmarshal(c.Config, &cfg)
		return &providers.ClaudeBridge{Node: cfg.Node, Script: cfg.Script}, nil
	}
	return nil, fmt.Errorf("지원하지 않는 연결 종류 %q", c.Provider)
}

// Delta is streamed provider text, batched for the UI.
type Delta struct {
	ProjectID     string `json:"projectId"`
	RunID         string `json:"runId"`
	StepAttemptID string `json:"stepAttemptId"`
	Text          string `json:"text"`
}

func (a *App) addDelta(projectID string, ev providers.Event) {
	var p providers.TextPayload
	json.Unmarshal(ev.Payload, &p)
	a.deltaMu.Lock()
	defer a.deltaMu.Unlock()
	d := a.deltas[ev.StepAttemptID]
	if d == nil {
		d = &Delta{ProjectID: projectID, RunID: ev.RunID, StepAttemptID: ev.StepAttemptID}
		a.deltas[ev.StepAttemptID] = d
	}
	d.Text += p.Text
}

// flushDeltas sends streamed text about every 100ms (spec §11).
func (a *App) flushDeltas(ctx context.Context) {
	t := time.NewTicker(deltaFlushMs * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		a.deltaMu.Lock()
		var batch []Delta
		for _, d := range a.deltas {
			batch = append(batch, *d)
		}
		a.deltas = map[string]*Delta{}
		a.deltaMu.Unlock()
		if len(batch) > 0 {
			a.emit(a.ctx, EventDeltas, batch)
		}
	}
}

// uiErr turns internal errors into messages the person can act on.
func uiErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, storage.ErrNotInProject):
		return errors.New("이 서비스에서 찾을 수 없는 항목입니다")
	case errors.Is(err, storage.ErrConflict):
		return errors.New("다른 곳에서 먼저 수정되었습니다. 새로 불러온 뒤 다시 시도하세요")
	case errors.Is(err, storage.ErrArchived):
		return errors.New("보관된 서비스는 수정할 수 없습니다")
	case errors.Is(err, storage.ErrRoleCycle):
		return errors.New("상위 역할을 이렇게 지정하면 순환이 생깁니다")
	case errors.Is(err, storage.ErrRoleInUse):
		return errors.New("담당자가 있는 역할은 삭제할 수 없습니다")
	case errors.Is(err, engine.ErrStale):
		return errors.New("이미 처리되었거나 더 이상 유효하지 않은 요청입니다. 화면을 새로 고치세요")
	}
	return err
}

func (a *App) ready() error {
	if a.db == nil || a.eng == nil {
		if a.initErr != nil {
			return fmt.Errorf("%w: %v", errNotReady, a.initErr)
		}
		return errNotReady
	}
	return nil
}

// ---- system ----

// PingResult is returned by Ping and also sent as the EventPing payload.
type PingResult struct {
	Sequence int64  `json:"sequence"`
	Message  string `json:"message"`
	At       string `json:"at"`
}

// Ping echoes the message back and emits the same result as an event.
func (a *App) Ping(message string) PingResult {
	res := PingResult{Sequence: a.seq.Add(1), Message: message, At: time.Now().UTC().Format(time.RFC3339Nano)}
	if a.ctx != nil {
		a.emit(a.ctx, EventPing, res)
	}
	return res
}

// SystemStatus describes where data lives and how secrets are kept.
type SystemStatus struct {
	DataDir           string `json:"dataDir"`
	SchemaVersion     int    `json:"schemaVersion"`
	SecretsPersistent bool   `json:"secretsPersistent"`
	Version           string `json:"version"`
	Error             string `json:"error"`
}

func (a *App) SystemStatus() SystemStatus {
	st := SystemStatus{DataDir: a.dataDir, Version: appVersion}
	if a.secrets != nil {
		st.SecretsPersistent = a.secrets.Persistent()
	}
	if a.initErr != nil || a.db == nil {
		if a.initErr != nil {
			st.Error = a.initErr.Error()
		}
		return st
	}
	st.SchemaVersion, _ = a.db.SchemaVersion(a.ctx)
	return st
}

func (a *App) GetSettings() (map[string]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.db.Settings(a.ctx)
}

func (a *App) SetSetting(key, value string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return a.db.SetSetting(a.ctx, key, value)
}

// EventsAfter returns stored events for catch-up after a reload.
func (a *App) EventsAfter(projectID string, after int64) ([]storage.EventRecord, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	evs, err := a.db.EventsAfter(a.ctx, projectID, after, 500)
	if evs == nil {
		evs = []storage.EventRecord{}
	}
	return evs, uiErr(err)
}

// ---- services ----

func (a *App) Dashboard(includeArchived bool) ([]engine.ProjectSummary, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.eng.Dashboard(a.ctx, includeArchived)
}

type ProjectInput struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Goal         string `json:"goal"`
	Instructions string `json:"instructions"`
	Mode         string `json:"mode"`
}

func (a *App) CreateProject(in ProjectInput) (domain.Project, error) {
	if err := a.ready(); err != nil {
		return domain.Project{}, err
	}
	p, err := a.db.CreateProject(a.ctx, domain.Project{Name: in.Name, Goal: in.Goal, Instructions: in.Instructions, Mode: domain.ProjectMode(in.Mode)})
	return p, uiErr(err)
}

func (a *App) UpdateProject(in ProjectInput) (domain.Project, error) {
	if err := a.ready(); err != nil {
		return domain.Project{}, err
	}
	cur, err := a.db.Project(a.ctx, in.ID)
	if err != nil {
		return cur, uiErr(err)
	}
	cur.Name, cur.Goal, cur.Instructions, cur.Mode = in.Name, in.Goal, in.Instructions, domain.ProjectMode(in.Mode)
	p, err := a.db.UpdateProject(a.ctx, cur)
	return p, uiErr(err)
}

func (a *App) SetProjectArchived(projectID string, archived bool) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.SetProjectArchived(a.ctx, projectID, archived))
}

// ---- organization ----

func (a *App) GetOrganization() (domain.Organization, error) {
	if err := a.ready(); err != nil {
		return domain.Organization{}, err
	}
	return a.db.Organization(a.ctx)
}

func (a *App) UpdateOrganization(name, instructions string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.UpdateOrganization(a.ctx, name, instructions))
}

func (a *App) ListRoles() ([]domain.Role, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	roles, err := a.db.Roles(a.ctx)
	if roles == nil {
		roles = []domain.Role{}
	}
	return roles, err
}

func (a *App) SaveRole(r domain.Role) (domain.Role, error) {
	if err := a.ready(); err != nil {
		return r, err
	}
	saved, err := a.db.SaveRole(a.ctx, r)
	return saved, uiErr(err)
}

func (a *App) DeleteRole(id string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.db.DeleteRole(a.ctx, id))
}

func (a *App) ListAssignments(projectID string) ([]domain.Assignment, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Assignments(a.ctx, projectID)
	if list == nil {
		list = []domain.Assignment{}
	}
	return list, uiErr(err)
}

func (a *App) SaveAssignment(as domain.Assignment) (domain.Assignment, error) {
	if err := a.ready(); err != nil {
		return as, err
	}
	saved, err := a.db.SaveAssignment(a.ctx, as)
	return saved, uiErr(err)
}

// InstructionPreview shows the merged instructions an assignment gets.
type InstructionPreview struct {
	Layers   []domain.InstructionLayer `json:"layers"`
	Composed string                    `json:"composed"`
}

func (a *App) InstructionPreview(projectID, assignmentID string) (InstructionPreview, error) {
	if err := a.ready(); err != nil {
		return InstructionPreview{}, err
	}
	layers, err := a.db.InstructionLayers(a.ctx, projectID, assignmentID)
	if err != nil {
		return InstructionPreview{}, uiErr(err)
	}
	return InstructionPreview{Layers: layers, Composed: domain.ComposeInstructions(layers)}, nil
}

// ConnectionView adds readiness to a stored connection.
type ConnectionView struct {
	domain.ProviderConnection
	Usable bool   `json:"usable"`
	Note   string `json:"note"`
}

func (a *App) ListConnections() ([]ConnectionView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Connections(a.ctx)
	out := []ConnectionView{}
	for _, c := range list {
		v := ConnectionView{ProviderConnection: c}
		switch {
		case c.Provider == "test":
			v.Usable, v.Note = true, "테스트용 가짜 응답입니다. 실제 AI 연결이 아닙니다."
		case c.Verified():
			v.Usable, v.Note = true, "연결 확인됨"
		default:
			v.Note = "연결 확인 전이라 실행할 수 없습니다 (연결 시험은 M2에서 제공)"
		}
		out = append(out, v)
	}
	return out, err
}

// EnsureTestConnection creates the built-in test connection if missing.
func (a *App) EnsureTestConnection() (domain.ProviderConnection, error) {
	if err := a.ready(); err != nil {
		return domain.ProviderConnection{}, err
	}
	list, err := a.db.Connections(a.ctx)
	if err != nil {
		return domain.ProviderConnection{}, err
	}
	for _, c := range list {
		if c.Provider == "test" {
			return c, nil
		}
	}
	return a.db.SaveConnection(a.ctx, domain.ProviderConnection{Name: "테스트 AI (가짜 응답)", Provider: "test"})
}

// ---- workflows ----

func (a *App) WorkflowTemplates() []templates.Template { return templates.List() }

func (a *App) ListWorkflows(projectID string) ([]domain.Workflow, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	list, err := a.db.Workflows(a.ctx, projectID)
	if list == nil {
		list = []domain.Workflow{}
	}
	return list, uiErr(err)
}

func (a *App) GetWorkflow(projectID, workflowID string) (domain.Workflow, error) {
	if err := a.ready(); err != nil {
		return domain.Workflow{}, err
	}
	w, err := a.db.Workflow(a.ctx, projectID, workflowID)
	return w, uiErr(err)
}

// CreateWorkflow starts a workflow from a built-in template.
func (a *App) CreateWorkflow(projectID, templateID, title string) (domain.Workflow, error) {
	if err := a.ready(); err != nil {
		return domain.Workflow{}, err
	}
	raw, ok := templates.Get(templateID)
	if !ok {
		return domain.Workflow{}, fmt.Errorf("템플릿 %q가 없습니다", templateID)
	}
	var draft map[string]any
	json.Unmarshal(raw, &draft)
	if title != "" {
		draft["title"] = title
	}
	data, _ := json.Marshal(draft)
	w, err := a.db.CreateWorkflow(a.ctx, projectID, data)
	return w, uiErr(err)
}

func (a *App) SaveWorkflowDraft(projectID, workflowID string, baseRevision int, draft string) (domain.Workflow, error) {
	if err := a.ready(); err != nil {
		return domain.Workflow{}, err
	}
	w, err := a.db.SaveWorkflowDraft(a.ctx, projectID, workflowID, baseRevision, json.RawMessage(draft))
	return w, uiErr(err)
}

func (a *App) ValidateWorkflow(projectID, workflowID string) (storage.ValidationResult, error) {
	if err := a.ready(); err != nil {
		return storage.ValidationResult{}, err
	}
	r, err := a.db.ValidateWorkflow(a.ctx, projectID, workflowID)
	if r.Issues == nil {
		r.Issues = []domain.Issue{}
	}
	return r, uiErr(err)
}

// ConfirmResult is a new version, or the issues that block one.
type ConfirmResult struct {
	Version    *domain.WorkflowVersion  `json:"version"`
	Validation storage.ValidationResult `json:"validation"`
}

func (a *App) ConfirmVersion(projectID, workflowID string, revision int) (ConfirmResult, error) {
	if err := a.ready(); err != nil {
		return ConfirmResult{}, err
	}
	v, res, err := a.db.ConfirmVersion(a.ctx, projectID, workflowID, revision)
	if res.Issues == nil {
		res.Issues = []domain.Issue{}
	}
	var nv *storage.ErrNotVersionable
	if errors.As(err, &nv) {
		return ConfirmResult{Validation: res}, nil
	}
	if err != nil {
		return ConfirmResult{}, uiErr(err)
	}
	return ConfirmResult{Version: &v, Validation: res}, nil
}

// ---- runs ----

// StartResult is a new run id, or the issues that block starting (T04).
type StartResult struct {
	RunID  string         `json:"runId"`
	Issues []domain.Issue `json:"issues"`
}

func (a *App) StartRun(projectID, versionID string) (StartResult, error) {
	if err := a.ready(); err != nil {
		return StartResult{}, err
	}
	id, err := a.eng.StartRun(a.ctx, projectID, versionID)
	var nr *engine.NotRunnableError
	if errors.As(err, &nr) {
		return StartResult{Issues: nr.Issues}, nil
	}
	return StartResult{RunID: id, Issues: []domain.Issue{}}, uiErr(err)
}

func (a *App) ListRuns(projectID string) ([]engine.RunDetail, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	r, err := a.eng.Runs(a.ctx, projectID)
	return r, uiErr(err)
}

func (a *App) GetRun(projectID, runID string) (engine.RunDetail, error) {
	if err := a.ready(); err != nil {
		return engine.RunDetail{}, err
	}
	d, err := a.eng.RunDetail(a.ctx, projectID, runID)
	return d, uiErr(err)
}

func (a *App) PauseRun(projectID, runID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.eng.PauseRun(a.ctx, projectID, runID))
}

func (a *App) ResumeRun(projectID, runID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.eng.ResumeRun(a.ctx, projectID, runID))
}

func (a *App) CancelRun(projectID, runID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.eng.CancelRun(a.ctx, projectID, runID))
}

func (a *App) RetryStep(projectID, runID, stepID string) error {
	if err := a.ready(); err != nil {
		return err
	}
	return uiErr(a.eng.RetryStep(a.ctx, projectID, runID, stepID))
}

func (a *App) ReadArtifact(projectID, artifactID string) (engine.ArtifactContent, error) {
	if err := a.ready(); err != nil {
		return engine.ArtifactContent{}, err
	}
	c, err := a.eng.ReadArtifact(a.ctx, projectID, artifactID)
	return c, uiErr(err)
}

// ---- my tasks ----

func (a *App) Inbox() ([]engine.InboxItem, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.eng.Inbox(a.ctx)
}

// ActionResult is the outcome of a human action. Problem is set when a
// submission did not meet the completion criteria; the step stays open.
type ActionResult struct {
	Outcome engine.Outcome `json:"outcome"`
	Problem string         `json:"problem"`
}

func actionResult(out engine.Outcome, err error) (ActionResult, error) {
	var verr *engine.VerificationError
	if errors.As(err, &verr) {
		return ActionResult{Problem: verr.Problem}, nil
	}
	return ActionResult{Outcome: out}, uiErr(err)
}

func (a *App) SubmitHumanResult(projectID, attemptID string, generation int, outputs map[string]string) (ActionResult, error) {
	if err := a.ready(); err != nil {
		return ActionResult{}, err
	}
	return actionResult(a.eng.SubmitHumanResult(a.ctx, projectID, attemptID, generation, outputs))
}

// SubmitReview records a review; outputs holds the review step's other
// outputs (the review report itself is written by the app).
func (a *App) SubmitReview(projectID, attemptID string, generation int, decision, comment string, targets []string, outputs map[string]string) (ActionResult, error) {
	if err := a.ready(); err != nil {
		return ActionResult{}, err
	}
	return actionResult(a.eng.SubmitReview(a.ctx, projectID, attemptID, generation, decision, comment, targets, outputs))
}

func (a *App) DecideApproval(projectID, approvalID string, generation int, decision, reason string, targets []string) (ActionResult, error) {
	if err := a.ready(); err != nil {
		return ActionResult{}, err
	}
	return actionResult(a.eng.DecideApproval(a.ctx, projectID, approvalID, generation, decision, reason, targets))
}

func (a *App) DecideToolApproval(projectID, approvalID string, accept bool) (ActionResult, error) {
	if err := a.ready(); err != nil {
		return ActionResult{}, err
	}
	return actionResult(a.eng.DecideToolApproval(a.ctx, projectID, approvalID, accept))
}

func (a *App) AnswerQuestion(projectID, messageID, answer string) (ActionResult, error) {
	if err := a.ready(); err != nil {
		return ActionResult{}, err
	}
	return actionResult(a.eng.AnswerQuestion(a.ctx, projectID, messageID, answer))
}
