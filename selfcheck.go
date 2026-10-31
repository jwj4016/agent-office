package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"agent-office/internal/connect"
	"agent-office/internal/domain"
	"agent-office/internal/engine"
	osproc "agent-office/internal/proc"
	"agent-office/internal/providers"
	"agent-office/internal/secrets"
	"agent-office/internal/storage"
	"agent-office/internal/workspace"
)

// The self-check runs the installed app's own binary without a window:
// storage in a Korean, space-containing folder, the bundled Claude bridge,
// a whole flow with the test provider, cancel and a restart. It never
// touches the person's data folder unless one is given explicitly, and
// never calls a paid API (spec §15 "실제 패키지 안에서 … 시험", T21).

const (
	CheckOK      = "ok"
	CheckWarn    = "warn" // works, but something the person may need is missing
	CheckFailed  = "failed"
	CheckSkipped = "skipped"
)

type CheckItem struct {
	ID     string `json:"id"`
	Label  string `json:"label"`
	Status string `json:"status"`
	Detail string `json:"detail"`
	Ms     int64  `json:"ms"`
}

type CheckReport struct {
	Version string      `json:"version"`
	OS      string      `json:"os"`
	Arch    string      `json:"arch"`
	Exe     string      `json:"exe"`
	DataDir string      `json:"dataDir"`
	OK      bool        `json:"ok"`
	Checks  []CheckItem `json:"checks"`
}

type checkOptions struct {
	DataDir string // empty: a new temporary folder, removed afterwards
	Keep    bool
	Wait    time.Duration
}

// selfCheckMain handles `agent-office --self-check [-out file] [-data-dir dir] [-keep]`.
// The report goes to -out as JSON (Windows GUI builds have no console)
// and to stdout as text. The exit code is 1 when a check failed.
func selfCheckMain(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("self-check", flag.ContinueOnError)
	fs.SetOutput(stdout)
	out := fs.String("out", "", "JSON 결과 파일")
	dir := fs.String("data-dir", "", "시험할 데이터 폴더 (없으면 임시 폴더)")
	keep := fs.Bool("keep", false, "임시 폴더를 지우지 않음")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rep := runSelfCheck(context.Background(), checkOptions{DataDir: *dir, Keep: *keep, Wait: 3 * time.Minute})
	for _, c := range rep.Checks {
		fmt.Fprintf(stdout, "[%s] %s (%dms) %s\n", c.Status, c.Label, c.Ms, c.Detail)
	}
	fmt.Fprintf(stdout, "Agent Office %s %s/%s ok=%v\n", rep.Version, rep.OS, rep.Arch, rep.OK)
	if *out != "" {
		b, _ := json.MarshalIndent(rep, "", "  ")
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			fmt.Fprintln(stdout, "결과 파일을 쓰지 못했습니다:", err)
			return 1
		}
	}
	if !rep.OK {
		return 1
	}
	return 0
}

func runSelfCheck(ctx context.Context, o checkOptions) (rep CheckReport) {
	rep = CheckReport{Version: appVersion, OS: runtime.GOOS, Arch: runtime.GOARCH}
	rep.Exe, _ = os.Executable()
	add := func(id, label string, f func() (string, string)) {
		start := time.Now()
		status, detail := f()
		rep.Checks = append(rep.Checks, CheckItem{ID: id, Label: label, Status: status, Detail: detail, Ms: time.Since(start).Milliseconds()})
	}
	defer func() {
		rep.OK = true
		for _, c := range rep.Checks {
			if c.Status == CheckFailed {
				rep.OK = false
			}
		}
	}()

	dir := o.DataDir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "에이전트 오피스 진단 ")
		if err != nil {
			add("data", "데이터 폴더", func() (string, string) { return CheckFailed, err.Error() })
			return rep
		}
		dir = tmp
		if !o.Keep {
			defer os.RemoveAll(tmp)
		}
	}
	rep.DataDir = dir

	dataOK := false
	add("data", "데이터 폴더·SQLite (저장 → 다시 열기)", func() (string, string) {
		if err := checkStorage(ctx, dir); err != nil {
			return CheckFailed, err.Error()
		}
		dataOK = true
		return CheckOK, dir
	})
	add("secrets", "키 저장소", func() (string, string) {
		if secrets.Open().Persistent() {
			return CheckOK, "OS 키 저장소 사용"
		}
		return CheckWarn, "OS 키 저장소를 쓸 수 없어 API 키는 앱을 끄면 지워집니다"
	})
	add("bridge", "Claude 실행 프로그램 (포함된 bridge)", func() (string, string) { return checkBridge(ctx) })
	add("codex", "Codex CLI", func() (string, string) {
		if p, ok := connect.FindExecutable("codex"); ok {
			return CheckOK, p
		}
		return CheckWarn, "codex를 찾지 못했습니다 (Codex 연결을 쓸 때만 필요)"
	})
	add("git", "Git", func() (string, string) {
		if g := workspace.FindGit(); g != nil {
			return CheckOK, g.Exe
		}
		return CheckWarn, "git이 없어 코드 업무를 하나씩 공유 폴더에서 실행합니다"
	})
	add("diagram", "다이어그램 스킬 (포함된 diagram-design)", func() (string, string) { return checkDiagramSkill(ctx) })
	if !dataOK {
		return rep
	}

	// The flow checks run the real App against the folder, the same way
	// the window does, but without Wails.
	restore := setEnv("AGENT_OFFICE_DATA_DIR", dir)
	defer restore()
	a, err := startHeadless(ctx)
	if err != nil {
		add("flow", "흐름 실행 (테스트 AI)", func() (string, string) { return CheckFailed, err.Error() })
		return rep
	}
	var project domain.Project
	var runID string
	add("flow", "흐름 실행 (테스트 AI, 한글·공백 작업 폴더)", func() (string, string) {
		var detail string
		project, runID, detail, err = checkFlow(a, dir, o.Wait)
		if err != nil {
			return CheckFailed, err.Error()
		}
		return CheckOK, detail
	})
	add("cancel", "실행 취소", func() (string, string) {
		if project.ID == "" {
			return CheckSkipped, "흐름 실행 실패"
		}
		if err := checkCancel(a, project.ID, o.Wait); err != nil {
			return CheckFailed, err.Error()
		}
		return CheckOK, "실행이 취소됨으로 끝남"
	})
	a.shutdown(ctx)
	add("restart", "앱 재시작 후 기록 복원", func() (string, string) {
		if runID == "" {
			return CheckSkipped, "흐름 실행 실패"
		}
		b, err := startHeadless(ctx)
		if err != nil {
			return CheckFailed, err.Error()
		}
		defer b.shutdown(ctx)
		return checkRestored(b, project.ID, runID)
	})
	return rep
}

func setEnv(key, value string) func() {
	old, had := os.LookupEnv(key)
	os.Setenv(key, value)
	return func() {
		if had {
			os.Setenv(key, old)
		} else {
			os.Unsetenv(key)
		}
	}
}

// checkStorage saves a role, closes the database and reads it back. It
// only works in an empty folder so a person's real data is never mixed
// with check data.
func checkStorage(ctx context.Context, dir string) error {
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s: 비어 있지 않은 폴더입니다. 시험용 빈 폴더를 지정하세요", dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, "agent-office.db")
	db, err := storage.Open(ctx, path)
	if err != nil {
		return err
	}
	saved, err := db.SaveRole(ctx, domain.Role{Name: "진단 역할", Instructions: "한글 지침 " + time.Now().Format(time.RFC3339Nano)})
	db.Close()
	if err != nil {
		return err
	}
	db, err = storage.Open(ctx, path)
	if err != nil {
		return fmt.Errorf("다시 열기: %w", err)
	}
	defer db.Close()
	got, err := db.Role(ctx, saved.ID)
	if err != nil {
		return fmt.Errorf("다시 연 데이터에서 읽기: %w", err)
	}
	if got.Instructions != saved.Instructions {
		return errors.New("다시 연 데이터의 내용이 저장한 값과 다릅니다")
	}
	return nil
}

// bridgeProbe is what `node main.js --probe` prints.
type bridgeProbe struct {
	OK            bool   `json:"ok"`
	Node          string `json:"node"`
	SDKVersion    string `json:"sdkVersion"`
	BinaryVersion string `json:"binaryVersion"`
	Error         string `json:"error"`
}

func checkBridge(ctx context.Context) (string, string) {
	script, ok := connect.FindBridge()
	if !ok {
		return CheckFailed, "포함된 Claude 실행 프로그램(claude-runner)을 찾지 못했습니다"
	}
	node, ok := connect.FindExecutable("node")
	if !ok {
		return CheckWarn, script + " — Node.js가 없어 시험하지 못했습니다 (Claude 연결에 Node.js 18 이상 필요)"
	}
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, node, script, "--probe")
	osproc.Hide(cmd)
	out, err := cmd.Output()
	var p bridgeProbe
	if jerr := json.Unmarshal(out, &p); jerr != nil {
		msg := strings.TrimSpace(string(out))
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			msg += strings.TrimSpace(string(ee.Stderr))
		}
		return CheckFailed, fmt.Sprintf("%s: %v %s", script, err, truncate(msg, 300))
	}
	if !p.OK {
		return CheckFailed, p.Error
	}
	return CheckOK, fmt.Sprintf("SDK %s, Claude %s, node %s (%s)", p.SDKVersion, p.BinaryVersion, p.Node, script)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func startHeadless(ctx context.Context) (*App, error) {
	a := NewApp()
	a.emit = func(context.Context, string, ...interface{}) {}
	a.startup(ctx)
	if st := a.SystemStatus(); st.Error != "" {
		return nil, errors.New(st.Error)
	}
	return a, nil
}

// setupExampleService builds a service the way a person does in the UI:
// roles, assignments (AI on the given connection, the owner as a person)
// and the example workflow with every step assigned, then confirms it.
func setupExampleService(a *App, name, workspacePath, connectionID, model string) (domain.Project, string, error) {
	p, err := a.CreateProject(ProjectInput{Name: name, Goal: name + "를 만든다", Mode: "review", WorkspacePath: workspacePath})
	if err != nil {
		return p, "", err
	}
	roles, err := a.ListRoles()
	if err != nil {
		return p, "", err
	}
	roleIDs := map[string]string{}
	for _, r := range roles {
		roleIDs[r.Name] = r.ID
	}
	for _, n := range []string{"기획", "설계", "백엔드", "프론트엔드", "QA", "리뷰어"} {
		if roleIDs[n] == "" {
			r, err := a.SaveRole(domain.Role{Name: n, Instructions: n + " 지침"})
			if err != nil {
				return p, "", err
			}
			roleIDs[n] = r.ID
		}
	}
	assign := map[string]string{}
	for key, role := range map[string]string{"a-planner": "기획", "a-architect": "설계", "a-backend": "백엔드", "a-frontend": "프론트엔드", "a-qa": "QA"} {
		as, err := a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: roleIDs[role], ActorKind: domain.ActorAI,
			DisplayName: role + " AI", ConnectionID: connectionID, Model: model})
		if err != nil {
			return p, "", err
		}
		assign[key] = as.ID
	}
	owner, err := a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: roleIDs["리뷰어"], ActorKind: domain.ActorHuman, DisplayName: "나"})
	if err != nil {
		return p, "", err
	}
	assign["a-owner"] = owner.ID

	w, err := a.CreateWorkflow(p.ID, "service-dev", "")
	if err != nil {
		return p, "", err
	}
	// The template leaves assignees empty; fill them like the editor does.
	byStep := map[string]string{"plan": "a-planner", "approve": "a-owner", "design": "a-architect", "backend": "a-backend",
		"frontend": "a-frontend", "integrate": "a-architect", "review": "a-owner", "qa": "a-qa", "deliver": "a-planner"}
	var spec domain.WorkflowSpec
	if err := json.Unmarshal(w.Draft, &spec); err != nil {
		return p, "", err
	}
	for i := range spec.Nodes {
		spec.Nodes[i].AssignmentID = assign[byStep[spec.Nodes[i].ID]]
	}
	draft, _ := json.Marshal(spec)
	if w, err = a.SaveWorkflowDraft(p.ID, w.ID, w.Revision, string(draft)); err != nil {
		return p, "", err
	}
	res, err := a.ConfirmVersion(p.ID, w.ID, w.Revision)
	if err != nil {
		return p, "", err
	}
	if res.Version == nil || !res.Validation.CanRun {
		return p, "", fmt.Errorf("흐름 확정 실패: %+v", res.Validation.Issues)
	}
	return p, res.Version.ID, nil
}

func waitFor(wait time.Duration, what string, f func() (bool, error)) error {
	deadline := time.Now().Add(wait)
	for {
		done, err := f()
		if err != nil || done {
			return err
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("%s: %s 안에 끝나지 않았습니다", what, wait)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func waitInboxItem(a *App, projectID, kind string, wait time.Duration) (engine.InboxItem, error) {
	var found engine.InboxItem
	err := waitFor(wait, kind+" 대기", func() (bool, error) {
		items, err := a.Inbox()
		for _, it := range items {
			if it.ProjectID == projectID && it.Kind == kind {
				found = it
				return true, nil
			}
		}
		return false, err
	})
	return found, err
}

func waitRunStatus(a *App, projectID, runID, status string, wait time.Duration) error {
	return waitFor(wait, "실행 "+status, func() (bool, error) {
		r, err := a.GetRun(projectID, runID)
		if err == nil && (r.Status == engine.RunFailed || r.Status == engine.RunCancelled) && r.Status != status {
			return false, fmt.Errorf("실행이 %s로 끝났습니다", r.Status)
		}
		return r.Status == status, err
	})
}

// checkFlow runs the example flow in a Korean, space-containing work
// folder: AI steps, the owner's approval and review, then reads a result.
func checkFlow(a *App, dir string, wait time.Duration) (domain.Project, string, string, error) {
	work := filepath.Join(dir, "작업 폴더 (진단)")
	if err := os.MkdirAll(work, 0o700); err != nil {
		return domain.Project{}, "", "", err
	}
	conn, err := a.EnsureTestConnection()
	if err != nil {
		return domain.Project{}, "", "", err
	}
	p, version, err := setupExampleService(a, "진단 서비스", work, conn.ID, providers.AutoScenario)
	if err != nil {
		return p, "", "", err
	}
	start, err := a.StartRun(p.ID, version)
	if err != nil {
		return p, "", "", err
	}
	if len(start.Issues) > 0 {
		return p, "", "", fmt.Errorf("실행을 시작할 수 없습니다: %+v", start.Issues)
	}
	ap, err := waitInboxItem(a, p.ID, engine.InboxApproval, wait)
	if err != nil {
		return p, start.RunID, "", err
	}
	if _, err := a.DecideApproval(p.ID, ap.ApprovalID, ap.Generation, engine.ApprovalApproved, "진행", nil); err != nil {
		return p, start.RunID, "", err
	}
	rv, err := waitInboxItem(a, p.ID, engine.InboxReview, wait)
	if err != nil {
		return p, start.RunID, "", err
	}
	if _, err := a.SubmitReview(p.ID, rv.AttemptID, rv.Generation, engine.ReviewPass, "통과", nil, nil); err != nil {
		return p, start.RunID, "", err
	}
	if err := waitRunStatus(a, p.ID, start.RunID, engine.RunSucceeded, wait); err != nil {
		return p, start.RunID, "", err
	}
	run, err := a.GetRun(p.ID, start.RunID)
	if err != nil {
		return p, start.RunID, "", err
	}
	files := 0
	for _, s := range run.Steps {
		for _, art := range s.Artifacts {
			c, err := a.ReadArtifact(p.ID, art.ID)
			if err != nil {
				return p, start.RunID, "", err
			}
			if !c.HashOK {
				return p, start.RunID, "", fmt.Errorf("결과 %s의 내용이 기록과 다릅니다", art.ID)
			}
			files++
		}
	}
	if files == 0 {
		return p, start.RunID, "", errors.New("결과 파일이 없습니다")
	}
	return p, start.RunID, fmt.Sprintf("업무 %d개 완료, 결과 %d개 확인 (%s)", len(run.Steps), files, work), nil
}

func checkCancel(a *App, projectID string, wait time.Duration) error {
	runs, err := a.ListRuns(projectID)
	if err != nil || len(runs) == 0 {
		return fmt.Errorf("이전 실행을 찾지 못했습니다: %v", err)
	}
	start, err := a.StartRun(projectID, runs[0].VersionID)
	if err != nil {
		return err
	}
	if err := a.CancelRun(projectID, start.RunID); err != nil {
		return err
	}
	return waitRunStatus(a, projectID, start.RunID, engine.RunCancelled, wait)
}

func checkRestored(a *App, projectID, runID string) (string, string) {
	run, err := a.GetRun(projectID, runID)
	if err != nil {
		return CheckFailed, err.Error()
	}
	if run.Status != engine.RunSucceeded {
		return CheckFailed, "다시 연 실행 상태: " + run.Status
	}
	for _, s := range run.Steps {
		for _, art := range s.Artifacts {
			if c, err := a.ReadArtifact(projectID, art.ID); err != nil || !c.HashOK {
				return CheckFailed, fmt.Sprintf("결과 %s를 다시 읽지 못했습니다: %v", art.ID, err)
			}
		}
	}
	return CheckOK, "실행·결과가 그대로 남아 있음"
}

// checkDiagramSkill reports the bundled diagram skill and, when Python is
// there, runs its self-check on one of the skill's own examples. A
// missing Python is only a warning: diagrams are then checked for format.
func checkDiagramSkill(ctx context.Context) (string, string) {
	d := findDiagramSkill()
	if d.Plugin == "" {
		return CheckFailed, "diagram-design 스킬을 찾지 못했습니다 (앱 패키지에 포함되어야 합니다)"
	}
	if d.Python == "" {
		return CheckWarn, d.Plugin + " (Python 3.8 이상이 없어 다이어그램 자동 검사는 건너뜁니다)"
	}
	sample := filepath.Join(d.SkillDir(), "assets", "example-architecture.html")
	if err := d.Check(ctx, sample); err != nil {
		return CheckFailed, fmt.Sprintf("%v (%s)", err, d.Python)
	}
	return CheckOK, fmt.Sprintf("%s, 검사: %s", d.Plugin, d.Python)
}
