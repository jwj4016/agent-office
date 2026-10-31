package engine_test

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"agent-office/internal/connect"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
)

var diagramWorkflow = []byte(`{
  "schemaVersion": 1, "title": "설계와 그림",
  "nodes": [
    {"id": "plan", "title": "기획", "kind": "task", "assignmentId": "a-planner", "dependsOn": [], "outputs": [{"key": "spec", "type": "markdown"}]},
    {"id": "arch", "title": "아키텍처 그림", "kind": "task", "assignmentId": "a-architect", "dependsOn": ["plan"],
     "inputs": [{"name": "기획", "fromStep": "plan", "outputKey": "spec"}], "outputs": [{"key": "arch", "type": "diagram"}]}
  ]}`)

func bundledDiagram(t *testing.T, python bool) engine.DiagramSkill {
	t.Helper()
	d := engine.DiagramSkill{Plugin: filepath.Join(testenv.RepoRoot(), "third_party", "diagram-design")}
	if python {
		if d.Python = connect.FindPython(); d.Python == "" {
			t.Skip("Python 3.8+ is not installed")
		}
	}
	return d
}

func diagramHarness(t *testing.T, d engine.DiagramSkill) (*harness, testenv.Project) {
	h := newHarness(t, func(c *engine.Config) { c.Diagram = d })
	p := testenv.SeedDraft(t, h.db, "그림", diagramWorkflow, map[string]string{"a-planner": "기획", "a-architect": "아키텍트"}, nil)
	return h, p
}

// writeDiagram makes the arch step write content as its diagram.
func writeDiagram(h *harness, name, content string) {
	h.prov.Scripts[name] = []providers.Step{
		{Write: &providers.WriteStep{Key: "arch", Content: content}},
		{Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded}},
	}
	h.useScenario("arch", name)
}

// A diagram step is given the bundled skill (as a Claude plugin, readable
// files and prompt rules) and its result passes the skill's self-check.
// Other steps get none of it.
func TestDiagramStepGetsSkillAndPassesSelfCheck(t *testing.T) {
	d := bundledDiagram(t, true)
	h, p := diagramHarness(t, d)
	runID := h.start(p)
	h.waitRun(p.ID, runID, engine.RunSucceeded)

	arch := h.started("arch")[0]
	if len(arch.Plugins) != 1 || arch.Plugins[0] != d.Plugin || !slices.Contains(arch.ReadableDirs, d.Plugin) {
		t.Fatalf("diagram step plugins=%v readable=%v", arch.Plugins, arch.ReadableDirs)
	}
	// Its inputs (the plan's result) are readable without asking too.
	for _, in := range arch.InputManifest {
		if !slices.Contains(arch.ReadableDirs, filepath.Dir(in.Path)) {
			t.Fatalf("input %s not readable: %v", in.Path, arch.ReadableDirs)
		}
	}
	for _, want := range []string{filepath.Join(d.SkillDir(), "SKILL.md"), "self_check.py", "다이어그램 작성 규칙", "사람에게 묻지 마세요", "self_check.py만 쓰세요"} {
		if !strings.Contains(arch.Prompt, want) {
			t.Fatalf("prompt lacks %q:\n%s", want, arch.Prompt)
		}
	}
	if len(arch.CheckCommand) != 2 || arch.CheckCommand[0] != d.Python || filepath.Base(arch.CheckCommand[1]) != "self_check.py" {
		t.Fatalf("check command = %v", arch.CheckCommand)
	}
	if !strings.HasSuffix(arch.OutputSpec[0].Path, ".html") {
		t.Fatalf("diagram output path = %s", arch.OutputSpec[0].Path)
	}
	if plan := h.started("plan")[0]; len(plan.Plugins) != 0 || len(plan.ReadableDirs) != 0 || strings.Contains(plan.Prompt, "다이어그램 작성 규칙") {
		t.Fatalf("non-diagram step got the skill: %v %v", plan.Plugins, plan.ReadableDirs)
	}
	art := h.detail(p.ID, runID).Step("arch").Artifacts[0]
	if art.Type != "diagram" || !strings.HasSuffix(art.Path, ".html") {
		t.Fatalf("artifact = %+v", art)
	}
}

// A diagram that breaks the skill's rules (here a remote script and an
// inaccessible SVG) does not complete the step.
func TestDiagramFailingSelfCheckFailsStep(t *testing.T) {
	h, p := diagramHarness(t, bundledDiagram(t, true))
	writeDiagram(h, "diagram-remote", `<html><body><svg viewBox="0 0 10 10"><rect width="5" height="5"/></svg><script src="https://cdn.example/x.js"></script></body></html>`)
	runID := h.start(p)
	s := h.waitStep(p.ID, runID, "arch", engine.StFailed)
	if !strings.Contains(s.Error, "self_check.py") || !strings.Contains(s.Error, "remote reference") {
		t.Fatalf("error = %q", s.Error)
	}
}

// Without Python the self-check is skipped, not failed; the result must
// still be HTML with an inline SVG.
func TestDiagramWithoutPythonChecksFormatOnly(t *testing.T) {
	h, p := diagramHarness(t, bundledDiagram(t, false))
	writeDiagram(h, "diagram-unchecked", `<html><body><svg viewBox="0 0 10 10"><rect width="5" height="5"/></svg></body></html>`)
	runID := h.start(p)
	h.waitStep(p.ID, runID, "arch", engine.StSucceeded)
	if a := h.started("arch")[0]; strings.Contains(a.Prompt, "self_check.py") || len(a.CheckCommand) != 0 {
		t.Fatal("prompt or permissions promise a self-check that cannot run")
	}

	h2, p2 := diagramHarness(t, bundledDiagram(t, false))
	writeDiagram(h2, "diagram-no-svg", "# 그림 대신 글\n")
	runID = h2.start(p2)
	if s := h2.waitStep(p2.ID, runID, "arch", engine.StFailed); !strings.Contains(s.Error, "<svg>") {
		t.Fatalf("error = %q", s.Error)
	}
}

// When the skill is missing from the app, the step still runs with the
// format rules and says so; nothing is loaded.
func TestDiagramWithoutBundledSkill(t *testing.T) {
	h, p := diagramHarness(t, engine.DiagramSkill{})
	runID := h.start(p)
	h.waitStep(p.ID, runID, "arch", engine.StSucceeded)
	arch := h.started("arch")[0]
	if len(arch.Plugins) != 0 || !strings.Contains(arch.Prompt, "diagram-design 스킬을 찾지 못했습니다") {
		t.Fatalf("plugins=%v prompt:\n%s", arch.Plugins, arch.Prompt)
	}
}
