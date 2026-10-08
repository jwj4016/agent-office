package engine_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
	"agent-office/internal/workspace"
)

func gitHarness(t *testing.T) (*harness, *workspace.Git) {
	t.Helper()
	g := workspace.FindGit()
	if g == nil {
		t.Skip("git is not installed")
	}
	h := newHarness(t, func(c *engine.Config) { c.Git = g })
	h.wait = 90 * time.Second // every git command is a process start
	return h, g
}

func file(t *testing.T, path, content string) providers.Step {
	t.Helper()
	b, _ := json.Marshal(map[string]any{"file": map[string]string{"path": path, "content": content}})
	return step(t, string(b))
}

func codeChangeOf(t *testing.T, h *harness, projectID string, s engine.StepView) engine.CodeChange {
	t.Helper()
	if len(s.Artifacts) == 0 {
		t.Fatalf("%s has no result", s.ID)
	}
	c, err := h.eng.ReadArtifact(h.ctx, projectID, s.Artifacts[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	var cc engine.CodeChange
	if err := json.Unmarshal([]byte(c.Content), &cc); err != nil {
		t.Fatalf("%s: %v\n%s", s.ID, err, c.Content)
	}
	return cc
}

func paths(cc engine.CodeChange) map[string]string {
	out := map[string]string{}
	for _, c := range cc.Changes {
		out[c.Path] = c.Status
	}
	return out
}

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// codeFlow is the service-dev workflow whose backend and frontend change
// real files; integrate builds the merged code.
func codeFlow(t *testing.T, h *harness, build bool) testenv.Project {
	t.Helper()
	draft := testenv.EditFixture(t, "workflows/service-dev.json", func(n map[string]map[string]any) {
		if build {
			n["integrate"]["completion"] = map[string]any{"commands": []any{map[string]any{"executable": "go", "args": []any{"build", "./..."}}}}
		}
	})
	p := testenv.SeedDraft(t, h.db, "게임", draft, testenv.ServiceDevRoles, []string{"a-owner"})
	h.prov.Scripts["backend-code"] = []providers.Step{
		file(t, "go.mod", "module demo\n\ngo 1.21\n"),
		file(t, "api/api.go", "package api\n\nfunc Version() string { return \"v1\" }\n"),
		step(t, `{"write": {"key": "change", "content": "{\"summary\": \"API 추가\"}"}}`), done(t),
	}
	h.prov.Scripts["frontend-code"] = []providers.Step{file(t, "web/web.go", "package web\n\nfunc Page() string { return \"hi\" }\n"), done(t)}
	h.setModel(p, "a-backend", "backend-code")
	h.setModel(p, "a-frontend", "frontend-code")
	return p
}

// T14: two developers work in separate worktrees at the same time; the
// integration step merges both and the real build runs on the result. The
// service's own repository work tree is never touched, and finished
// worktrees are removed while their branches stay.
func TestParallelCodeStepsIntegrateAndBuild(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	h, g := gitHarness(t)
	p := codeFlow(t, h, true)
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	d := h.detail(p.ID, runID)

	be, fe := d.Step("backend").Workspace, d.Step("frontend").Workspace
	if be == nil || fe == nil || be.Kind != "code" || fe.Kind != "code" || be.Path == fe.Path || be.Branch == fe.Branch || be.Commit == "" {
		t.Fatalf("backend/frontend workspaces = %+v / %+v", be, fe)
	}
	if h.started("backend")[0].Workspace != be.Path || h.started("frontend")[0].Workspace != fe.Path {
		t.Fatal("providers were not started in their own worktrees")
	}
	if plan := h.started("plan")[0]; plan.Policy.Sandbox != "read-only" || plan.Workspace == d.Repo.Path {
		t.Fatalf("non-code step workspace = %s %s", plan.Policy.Sandbox, plan.Workspace)
	}

	cc := codeChangeOf(t, h, p.ID, d.Step("integrate"))
	got := paths(cc)
	if got["go.mod"] != "added" || got["api/api.go"] != "added" || got["web/web.go"] != "added" {
		t.Fatalf("integrated changes = %+v", cc.Changes)
	}
	if cc.BaseCommit != d.Repo.Base || !strings.HasPrefix(cc.Branch, "agent-office/") || !strings.Contains(cc.Patch, "+func Page()") {
		t.Fatalf("code change = %+v", cc)
	}
	// The model's own summary is kept next to the facts from Git.
	var raw map[string]any
	c, _ := h.eng.ReadArtifact(h.ctx, p.ID, d.Step("backend").Artifacts[0].ID)
	json.Unmarshal([]byte(c.Content), &raw)
	if raw["summary"] != "API 추가" || raw["commit"] != be.Commit {
		t.Fatalf("backend result = %s", c.Content)
	}

	// The person reviews the integrated code in a read-only checkout,
	// prepared in the background.
	var review *engine.InboxItem
	h.waitFor(p.ID, runID, "review checkout", func(engine.RunDetail) bool {
		items, _ := h.eng.Inbox(h.ctx)
		for i := range items {
			if items[i].StepID == "review" && items[i].Workspace != nil {
				review = &items[i]
			}
		}
		return review != nil
	})
	if review == nil || review.Workspace == nil || review.Workspace.Kind != "view" || review.Workspace.Start != cc.Commit {
		t.Fatalf("review workspace = %+v", review)
	}
	for _, f := range []string{"api/api.go", "web/web.go"} {
		if _, err := os.Stat(filepath.Join(review.Workspace.Path, f)); err != nil {
			t.Fatalf("review checkout lacks %s", f)
		}
	}

	if dirty, _ := g.Dirty(h.ctx, d.Repo.Path); dirty {
		t.Fatal("the service repository work tree changed")
	}
	if head, _ := g.Head(h.ctx, d.Repo.Path); head != d.Repo.Base {
		t.Fatal("the service repository HEAD moved")
	}

	h.review(p.ID, runID, "review", engine.ReviewPass, "통합 확인")
	h.waitRun(p.ID, runID, engine.RunSucceeded)
	h.waitFor(p.ID, runID, "worktrees removed", func(engine.RunDetail) bool {
		_, err1 := os.Stat(be.Path)
		_, err2 := os.Stat(review.Workspace.Path)
		return os.IsNotExist(err1) && os.IsNotExist(err2)
	})
	if branches := gitOut(t, d.Repo.Path, "branch", "--list", "agent-office/*"); !strings.Contains(branches, be.Branch) {
		t.Fatalf("branches = %s", branches)
	}
}

// A reworked code step continues from its previous commit; only the
// affected steps run again and integration picks up the new commit.
func TestCodeReworkContinuesFromPreviousCommit(t *testing.T) {
	h, _ := gitHarness(t)
	p := codeFlow(t, h, false)
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	first := h.detail(p.ID, runID).Step("backend").Workspace

	h.prov.Scripts["backend-v2"] = []providers.Step{file(t, "api/api.go", "package api\n\nfunc Version() string { return \"v2\" }\n"), done(t)}
	h.useScenario("backend", "backend-v2")
	h.review(p.ID, runID, "review", engine.ReviewChangesRequested, "버전을 v2로", "backend")
	h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	d := h.detail(p.ID, runID)

	second := d.Step("backend").Workspace
	if second.Start != first.Commit || second.Branch == first.Branch {
		t.Fatalf("rework started from %s, want %s", second.Start, first.Commit)
	}
	if n := len(h.started("frontend")); n != 1 {
		t.Fatalf("frontend ran %d times", n)
	}
	cc := codeChangeOf(t, h, p.ID, d.Step("integrate"))
	if !strings.Contains(cc.Patch, `return "v2"`) || paths(cc)["web/web.go"] != "added" {
		t.Fatalf("integration after rework = %+v", cc)
	}
	if d.Step("integrate").Round != 1 || d.Step("backend").Round != 1 {
		t.Fatalf("rounds = %d %d", d.Step("integrate").Round, d.Step("backend").Round)
	}
}

// Conflicting parallel changes are left for the integrator, who is told
// which files conflict; unresolved conflicts fail the step.
func TestIntegrationConflictMustBeResolved(t *testing.T) {
	h, _ := gitHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.prov.Scripts["be"] = []providers.Step{file(t, "shared.txt", "backend\n"), done(t)}
	h.prov.Scripts["fe"] = []providers.Step{file(t, "shared.txt", "frontend\n"), done(t)}
	h.setModel(p, "a-backend", "be")
	h.setModel(p, "a-frontend", "fe")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")

	s := h.waitStep(p.ID, runID, "integrate", engine.StFailed)
	if !strings.Contains(s.Error, "충돌") || !strings.Contains(s.Error, "shared.txt") {
		t.Fatalf("integrate error = %q", s.Error)
	}
	if prompt := h.started("integrate")[0].Prompt; !strings.Contains(prompt, "병합 충돌") || !strings.Contains(prompt, "shared.txt") {
		t.Fatalf("integrator was not told about the conflict:\n%s", prompt)
	}

	h.prov.Scripts["resolve"] = []providers.Step{file(t, "shared.txt", "backend+frontend\n"), done(t)}
	h.useScenario("integrate", "resolve")
	if err := h.eng.RetryStep(h.ctx, p.ID, runID, "integrate"); err != nil {
		t.Fatal(err)
	}
	h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	cc := codeChangeOf(t, h, p.ID, h.detail(p.ID, runID).Step("integrate"))
	if !strings.Contains(cc.Patch, "+backend+frontend") || strings.Contains(cc.Patch, "<<<<<<<") {
		t.Fatalf("resolved patch = %s", cc.Patch)
	}
}

// A service working in the user's own repository: runs start from its
// last commit; uncommitted changes and untracked files stay as they were.
func TestUserRepositoryIsLeftAlone(t *testing.T) {
	h, g := gitHarness(t)
	ctx := context.Background()
	repo := filepath.Join(t.TempDir(), "내 저장소")
	if _, err := g.Init(ctx, repo); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("committed\n"), 0o600)
	gitOut(t, repo, "add", "README.md")
	gitOut(t, repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "readme")
	head := gitOut(t, repo, "rev-parse", "HEAD")
	os.WriteFile(filepath.Join(repo, "README.md"), []byte("my unsaved edit\n"), 0o600)
	os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("untracked\n"), 0o600)
	before := gitOut(t, repo, "status", "--porcelain")

	p := codeFlow(t, h, false)
	proj, _ := h.db.Project(h.ctx, p.ID)
	proj.WorkspacePath = repo
	if _, err := h.db.UpdateProject(h.ctx, proj); err != nil {
		t.Fatal(err)
	}
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	d := h.detail(p.ID, runID)
	if d.Repo == nil || d.Repo.Base != head || !strings.Contains(d.Repo.Note, "커밋되지 않은 변경") {
		t.Fatalf("repo = %+v", d.Repo)
	}
	data, _ := os.ReadFile(filepath.Join(repo, "README.md"))
	if string(data) != "my unsaved edit\n" || gitOut(t, repo, "status", "--porcelain") != before || gitOut(t, repo, "rev-parse", "HEAD") != head {
		t.Fatal("the user's work tree changed")
	}
	// The integrated work started from the last commit, not the unsaved edit.
	if cc := codeChangeOf(t, h, p.ID, d.Step("integrate")); paths(cc)["README.md"] != "" {
		t.Fatalf("unsaved edit leaked into the run: %+v", cc.Changes)
	}
}

// Without Git, code steps share one folder, so they run one at a time.
func TestSharedFolderRunsCodeStepsOneAtATime(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	slow := []providers.Step{{Sleep: "400ms"}, writeChange(t), done(t)}
	h.prov.Scripts["slow-code"] = slow
	h.setModel(p, "a-backend", "slow-code")
	h.setModel(p, "a-frontend", "slow-code")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	deadline := time.Now().Add(10 * time.Second)
	for h.detail(p.ID, runID).Step("integrate").Status == engine.StPending && time.Now().Before(deadline) {
		d := h.detail(p.ID, runID)
		if d.Step("backend").Status == engine.StRunning && d.Step("frontend").Status == engine.StRunning {
			t.Fatal("two code steps ran in the same shared folder at once")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if d := h.detail(p.ID, runID); d.Repo == nil || d.Repo.Kind != "shared" || !strings.Contains(d.Repo.Note, "하나씩") {
		t.Fatalf("repo = %+v", d.Repo)
	}
}
