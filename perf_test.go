package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/engine"

	"github.com/zalando/go-keyring"
)

// The measurement scenario (spec §11): two services, 20 assignees, a
// 50-step flow, and two AI steps active once the app starts the runs.
func TestPerfSeedBuildsTheScenarioAndStartsTwoAISteps(t *testing.T) {
	keyring.MockInit()
	dir := filepath.Join(t.TempDir(), "성능 데이터")
	if err := seedPerf(context.Background(), dir); err != nil {
		t.Fatal(err)
	}
	if err := seedPerf(context.Background(), dir); err == nil {
		t.Fatal("seeding a folder that has data must be refused")
	}

	logFile := filepath.Join(t.TempDir(), "perf.jsonl")
	t.Setenv(perfEnv, logFile)
	t.Setenv("AGENT_OFFICE_DATA_DIR", dir)
	a, err := startHeadless(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer a.shutdown(context.Background())
	if !a.SystemStatus().Perf {
		t.Fatal("status should say the app runs for measurement")
	}

	projects := need(a.Dashboard(false))
	if len(projects) != 2 {
		t.Fatalf("services: %d", len(projects))
	}
	people, maxSteps := 0, 0
	for _, p := range projects {
		people += len(need(a.ListAssignments(p.Project.ID)))
		for _, w := range need(a.ListWorkflows(p.Project.ID)) {
			var spec domain.WorkflowSpec
			json.Unmarshal(w.Draft, &spec)
			maxSteps = max(maxSteps, len(spec.Nodes))
		}
	}
	if people != 20 || maxSteps != 50 {
		t.Fatalf("assignees %d, largest flow %d steps", people, maxSteps)
	}

	// Two runs start and the app-wide limit keeps two AI steps working.
	err = waitFor(e2eWait, "AI 2개 작업", func() (bool, error) {
		active := 0
		for _, p := range projects {
			runs, err := a.ListRuns(p.Project.ID)
			if err != nil {
				return false, err
			}
			for _, r := range runs {
				for _, s := range r.Steps {
					if s.Status == engine.StRunning {
						active++
					}
				}
			}
		}
		return active == 2, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	a.PerfMark("ui-ready", `{"sinceNavigationMs":12}`)
	a.PerfMark(strings.Repeat("x", 65), "") // ignored: name too long
	b, _ := os.ReadFile(logFile)
	log := string(b)
	for _, want := range []string{`"name":"startup"`, `"started":2`, `"name":"ui-ready"`, `"sinceNavigationMs":12`} {
		if !strings.Contains(log, want) {
			t.Fatalf("perf log lacks %s:\n%s", want, log)
		}
	}
	if strings.Contains(log, "xxxx") {
		t.Fatal("oversized mark was written")
	}
}

func TestPerfMarkIsOffByDefault(t *testing.T) {
	t.Setenv(perfEnv, "")
	a := &App{}
	a.PerfMark("ui-ready", "{}") // must not panic or write anywhere
	if a.SystemStatus().Perf {
		t.Fatal("perf on without the variable")
	}
}
