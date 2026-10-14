package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/providers"
)

// Performance measurement (spec §11): `--perf-seed` fills an empty data
// folder with the measurement scenario — two services, 20 assignees, a
// 50-step flow — and with AGENT_OFFICE_PERF=<file> the app starts those
// runs on launch and appends timings (startup, first screen, screen lag)
// to the file as JSON lines. CPU and memory are sampled from outside by
// scripts/perf-*.sh. Without the variable none of this runs.

// processStart is as close to process start as Go code gets.
var processStart = time.Now()

const (
	perfEnv        = "AGENT_OFFICE_PERF"
	perfRunsEnv    = "AGENT_OFFICE_PERF_RUNS"
	perfRunsFile   = "perf-runs.json"
	perfNamePrefix = "성능 시험 "
)

var perfMu sync.Mutex

func perfEnabled() bool { return os.Getenv(perfEnv) != "" }

// perfLog appends one measurement line to the perf file.
func perfLog(name string, data any) {
	path := os.Getenv(perfEnv)
	if path == "" {
		return
	}
	line, err := json.Marshal(map[string]any{
		"at": time.Now().Format(time.RFC3339Nano), "name": name,
		"ms": time.Since(processStart).Milliseconds(), "data": data,
	})
	if err != nil {
		return
	}
	perfMu.Lock()
	defer perfMu.Unlock()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("perf log: %v", err)
		return
	}
	defer f.Close()
	f.Write(append(line, '\n'))
}

// PerfMark records a screen measurement (first screen, lag summary).
// It does nothing unless the app was started for measurement.
func (a *App) PerfMark(name, data string) {
	if !perfEnabled() || len(name) > 64 || len(data) > 4096 {
		return
	}
	var v any = data
	if json.Valid([]byte(data)) {
		v = json.RawMessage(data)
	}
	perfLog(name, v)
}

type perfRun struct {
	ProjectID string `json:"projectId"`
	VersionID string `json:"versionId"`
}

// startPerfRuns starts the seeded runs; only versions the seed recorded
// in the data folder are started.
func (a *App) startPerfRuns() {
	b, err := os.ReadFile(filepath.Join(a.dataDir, perfRunsFile))
	if err != nil {
		perfLog("runs", map[string]any{"error": "perf-runs.json 없음: --perf-seed로 만든 데이터 폴더가 아닙니다"})
		return
	}
	var runs []perfRun
	json.Unmarshal(b, &runs)
	started := 0
	for _, r := range runs {
		res, err := a.StartRun(r.ProjectID, r.VersionID)
		if err != nil || len(res.Issues) > 0 {
			perfLog("runs", map[string]any{"error": fmt.Sprint(err, res.Issues)})
			continue
		}
		started++
	}
	perfLog("runs", map[string]any{"started": started})
}

// perfSeedMain handles `agent-office --perf-seed -data-dir <empty folder>`.
func perfSeedMain(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("perf-seed", flag.ContinueOnError)
	fs.SetOutput(stdout)
	dir := fs.String("data-dir", "", "측정용 빈 데이터 폴더")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := seedPerf(context.Background(), *dir); err != nil {
		fmt.Fprintln(stdout, "측정 데이터를 만들지 못했습니다:", err)
		return 1
	}
	fmt.Fprintf(stdout, "측정 데이터: %s\n실행: %s=<기록 파일> AGENT_OFFICE_DATA_DIR=%q agent-office\n", *dir, perfEnv, *dir)
	return 0
}

func seedPerf(ctx context.Context, dir string) error {
	if dir == "" {
		return errors.New("-data-dir이 필요합니다")
	}
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s: 비어 있지 않은 폴더입니다", dir)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	restore := setEnv("AGENT_OFFICE_DATA_DIR", dir)
	defer restore()
	a, err := startHeadless(ctx)
	if err != nil {
		return err
	}
	defer a.shutdown(ctx)
	conn, err := a.EnsureTestConnection()
	if err != nil {
		return err
	}

	big, bigVersion, err := seedBigService(a, conn.ID)
	if err != nil {
		return err
	}
	// The second service: the example flow plus extra people, 10 in all.
	small, smallVersion, err := setupExampleService(a, perfNamePrefix+"B", "", conn.ID, providers.AutoSlowScenario)
	if err != nil {
		return err
	}
	if err := addAssignees(a, small.ID, conn.ID, 4); err != nil {
		return err
	}
	runs, _ := json.Marshal([]perfRun{{big.ID, bigVersion}, {small.ID, smallVersion}})
	if err := os.WriteFile(filepath.Join(dir, perfRunsFile), runs, 0o600); err != nil {
		return err
	}
	// Open on the big service's office, the busiest screen.
	return a.SetSetting("ui.lastProjectId", big.ID)
}

// addAssignees adds n AI assignees using an existing role.
func addAssignees(a *App, projectID, connectionID string, n int) error {
	roles, err := a.ListRoles()
	if err != nil || len(roles) == 0 {
		return fmt.Errorf("역할 없음: %v", err)
	}
	for i := 1; i <= n; i++ {
		if _, err := a.SaveAssignment(domain.Assignment{ProjectID: projectID, RoleID: roles[i%len(roles)].ID, ActorKind: domain.ActorAI,
			DisplayName: fmt.Sprintf("지원 AI %d", i), ConnectionID: connectionID, Model: providers.AutoSlowScenario}); err != nil {
			return err
		}
	}
	return nil
}

// seedBigService builds the 50-step flow: a plan, six lanes of eight
// steps each, then the owner's review. Nine AI assignees share the work.
func seedBigService(a *App, connectionID string) (domain.Project, string, error) {
	p, err := a.CreateProject(ProjectInput{Name: perfNamePrefix + "A", Goal: "성능 측정용 50단계 흐름", Mode: "review"})
	if err != nil {
		return p, "", err
	}
	role, err := a.SaveRole(domain.Role{Name: "측정 담당", Instructions: "측정용 역할"})
	if err != nil {
		return p, "", err
	}
	var ai []string
	for i := 1; i <= 9; i++ {
		as, err := a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI,
			DisplayName: fmt.Sprintf("AI %d", i), ConnectionID: connectionID, Model: providers.AutoSlowScenario})
		if err != nil {
			return p, "", err
		}
		ai = append(ai, as.ID)
	}
	owner, err := a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorHuman, DisplayName: "나"})
	if err != nil {
		return p, "", err
	}

	md := []domain.Output{{Key: "note", Type: "markdown"}}
	nodes := []domain.Node{{ID: "plan", Title: "기획", Kind: domain.KindTask, AssignmentID: ai[0], DependsOn: []string{}, Outputs: md}}
	var lastOfLane []string
	for lane := 1; lane <= 6; lane++ {
		prev := "plan"
		for step := 1; step <= 8; step++ {
			id := fmt.Sprintf("l%d-s%d", lane, step)
			nodes = append(nodes, domain.Node{
				ID: id, Title: fmt.Sprintf("작업 %d-%d", lane, step), Kind: domain.KindTask,
				AssignmentID: ai[(lane*8+step)%len(ai)], DependsOn: []string{prev},
				Inputs:  []domain.Input{{Name: "이전 결과", FromStep: prev, OutputKey: "note"}},
				Outputs: md,
			})
			prev = id
		}
		lastOfLane = append(lastOfLane, prev)
	}
	var inputs []domain.Input
	for _, s := range lastOfLane {
		inputs = append(inputs, domain.Input{Name: s, FromStep: s, OutputKey: "note"})
	}
	nodes = append(nodes, domain.Node{ID: "review", Title: "내 검토", Kind: domain.KindReview, AssignmentID: owner.ID,
		DependsOn: lastOfLane, Inputs: inputs, Outputs: []domain.Output{{Key: "review", Type: "report"}}})

	w, err := a.CreateWorkflow(p.ID, "blank", "50단계 흐름")
	if err != nil {
		return p, "", err
	}
	var spec domain.WorkflowSpec
	if err := json.Unmarshal(w.Draft, &spec); err != nil {
		return p, "", err
	}
	spec.Title = "50단계 흐름"
	spec.Nodes = nodes
	draft, _ := json.Marshal(spec)
	if w, err = a.SaveWorkflowDraft(p.ID, w.ID, w.Revision, string(draft)); err != nil {
		return p, "", err
	}
	res, err := a.ConfirmVersion(p.ID, w.ID, w.Revision)
	if err != nil {
		return p, "", err
	}
	if res.Version == nil || !res.Validation.CanRun {
		return p, "", fmt.Errorf("50단계 흐름 확정 실패: %s", issues(res.Validation.Issues))
	}
	return p, res.Version.ID, nil
}

func issues(list []domain.Issue) string {
	var parts []string
	for _, i := range list {
		b, _ := json.Marshal(i)
		parts = append(parts, string(b))
	}
	return strings.Join(parts, "; ")
}
