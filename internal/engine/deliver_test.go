package engine_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
)

// finishRun drives the code flow to its end.
func finishRun(t *testing.T, h *harness, p testenv.Project) string {
	t.Helper()
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	h.review(p.ID, runID, "review", engine.ReviewPass, "확인")
	h.waitRun(p.ID, runID, engine.RunSucceeded)
	return runID
}

func finalCode(t *testing.T, h *harness, projectID, runID string) engine.FinalCode {
	t.Helper()
	fc, err := h.eng.FinalCode(h.ctx, projectID, runID)
	if err != nil {
		t.Fatal(err)
	}
	return fc
}

// A service without a work folder: delivering moves the app repository to
// the run's code, copies it into the person's folder and makes that the
// work folder, so the next run starts from the delivered code.
func TestDeliverCodeToFolder(t *testing.T) {
	h, g := gitHarness(t)
	p := codeFlow(t, h, false)
	runID := finishRun(t, h, p)
	d := h.detail(p.ID, runID)

	fc := finalCode(t, h, p.ID, runID)
	integrate := d.Step("integrate").Workspace
	if !fc.AppRepo || len(fc.Steps) != 1 || fc.Steps[0].StepID != "integrate" || fc.Commit != integrate.Commit || fc.Included || fc.Stat.Files != 3 {
		t.Fatalf("final code = %+v", fc)
	}
	if _, err := h.eng.DeliverCode(h.ctx, engine.DeliverInput{ProjectID: p.ID, RunID: runID, Method: engine.DeliverBranch}); err == nil {
		t.Fatal("a result branch was made in the app repository")
	}

	folder := filepath.Join(t.TempDir(), "꺼낸 코드")
	got, err := h.eng.DeliverCode(h.ctx, engine.DeliverInput{ProjectID: p.ID, RunID: runID, Method: engine.DeliverFolder, Folder: folder, UseAsWorkspace: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Commit != fc.Commit || got.Folder != folder || !got.Workspace || got.Error != "" {
		t.Fatalf("delivery = %+v", got)
	}
	if head, _ := g.Head(h.ctx, d.Repo.Path); head != fc.Commit {
		t.Fatalf("app repository HEAD = %s, want %s", head, fc.Commit)
	}
	if dirty, _ := g.Dirty(h.ctx, d.Repo.Path); dirty {
		t.Fatal("the app repository was left with changes")
	}
	for _, f := range []string{"go.mod", "api/api.go", "web/web.go"} {
		if _, err := os.Stat(filepath.Join(folder, f)); err != nil {
			t.Fatalf("folder lacks %s", f)
		}
	}
	if remotes := gitOut(t, folder, "remote"); remotes != "" {
		t.Fatalf("the copy still points back: %q", remotes)
	}
	proj, _ := h.db.Project(h.ctx, p.ID)
	if proj.WorkspacePath != folder {
		t.Fatalf("work folder = %q", proj.WorkspacePath)
	}
	if fc := finalCode(t, h, p.ID, runID); !fc.Included || len(fc.Deliveries) != 1 || fc.Deliveries[0].Method != engine.DeliverFolder {
		t.Fatalf("after delivery = %+v", fc)
	}

	// A folder with something in it is refused before anything happens.
	if _, err := h.eng.DeliverCode(h.ctx, engine.DeliverInput{ProjectID: p.ID, RunID: runID, Method: engine.DeliverFolder, Folder: folder}); err == nil || !strings.Contains(err.Error(), "비어 있지 않은") {
		t.Fatalf("non-empty folder: %v", err)
	}
	if fc := finalCode(t, h, p.ID, runID); len(fc.Deliveries) != 1 {
		t.Fatalf("refusal recorded as a delivery: %+v", fc.Deliveries)
	}

	next := h.start(p)
	h.approve(p.ID, next, "approve")
	h.waitStep(p.ID, next, "review", engine.StWaitingHuman)
	if r := h.detail(p.ID, next).Repo; r == nil || !strings.EqualFold(r.Path, folder) && r.Path != mustEval(t, folder) || r.Base != fc.Commit {
		t.Fatalf("next run repo = %+v, want %s at %s", r, folder, fc.Commit)
	}
}

func mustEval(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// divergentFlow leaves backend and frontend as the last code steps: the
// integration step only writes a note.
func divergentFlow(t *testing.T, h *harness, be, fe providers.Step) testenv.Project {
	t.Helper()
	draft := testenv.EditFixture(t, "workflows/service-dev.json", func(n map[string]map[string]any) {
		n["integrate"]["outputs"] = []any{map[string]any{"key": "integrated", "type": "markdown", "required": true}}
	})
	p := testenv.SeedDraft(t, h.db, "게임", draft, testenv.ServiceDevRoles, []string{"a-owner"})
	h.prov.Scripts["be"] = []providers.Step{be, done(t)}
	h.prov.Scripts["fe"] = []providers.Step{fe, done(t)}
	h.setModel(p, "a-backend", "be")
	h.setModel(p, "a-frontend", "fe")
	return p
}

// Several last code steps are merged on delivery; a conflict stops the
// delivery and leaves no half-made branch.
func TestDeliverMergesLastCodeSteps(t *testing.T) {
	h, g := gitHarness(t)
	p := divergentFlow(t, h, file(t, "api/api.go", "package api\n"), file(t, "web/web.go", "package web\n"))
	runID := finishRun(t, h, p)
	fc := finalCode(t, h, p.ID, runID)
	if len(fc.Steps) != 2 || fc.Commit != "" {
		t.Fatalf("final code = %+v", fc)
	}
	got, err := h.eng.DeliverCode(h.ctx, engine.DeliverInput{ProjectID: p.ID, RunID: runID, Method: engine.DeliverBase})
	if err != nil {
		t.Fatal(err)
	}
	repo := h.detail(p.ID, runID).Repo.Path
	for _, f := range []string{"api/api.go", "web/web.go"} {
		if _, err := os.Stat(filepath.Join(repo, f)); err != nil {
			t.Fatalf("app repository lacks %s", f)
		}
	}
	if fc := finalCode(t, h, p.ID, runID); fc.Commit != got.Commit || !fc.Included {
		t.Fatalf("after delivery = %+v, delivered %s", fc, got.Commit)
	}

	h2, _ := gitHarness(t)
	q := divergentFlow(t, h2, file(t, "shared.txt", "backend\n"), file(t, "shared.txt", "frontend\n"))
	runID = finishRun(t, h2, q)
	_, err = h2.eng.DeliverCode(h2.ctx, engine.DeliverInput{ProjectID: q.ID, RunID: runID, Method: engine.DeliverBase})
	if err == nil || !strings.Contains(err.Error(), "충돌") || !strings.Contains(err.Error(), "shared.txt") {
		t.Fatalf("conflict: %v", err)
	}
	repo = h2.detail(q.ID, runID).Repo.Path
	if b := gitOut(t, repo, "branch", "--list", "*final*"); b != "" {
		t.Fatalf("left branch %q", b)
	}
	if head, _ := g.Head(h2.ctx, repo); head != h2.detail(q.ID, runID).Repo.Base {
		t.Fatal("the base moved despite the conflict")
	}
}

// A service in the person's repository gets a result branch; its current
// branch moves only when clean and fast-forward.
func TestDeliverToUserRepository(t *testing.T) {
	h, g := gitHarness(t)
	ctx := context.Background()
	repo := filepath.Join(t.TempDir(), "내 저장소")
	if _, err := g.Init(ctx, repo); err != nil {
		t.Fatal(err)
	}
	p := codeFlow(t, h, false)
	proj, _ := h.db.Project(h.ctx, p.ID)
	proj.WorkspacePath = repo
	if _, err := h.db.UpdateProject(h.ctx, proj); err != nil {
		t.Fatal(err)
	}
	runID := finishRun(t, h, p)
	fc := finalCode(t, h, p.ID, runID)
	if fc.AppRepo || fc.Commit == "" {
		t.Fatalf("final code = %+v", fc)
	}
	if _, err := h.eng.DeliverCode(h.ctx, engine.DeliverInput{ProjectID: p.ID, RunID: runID, Method: engine.DeliverFolder, Folder: filepath.Join(t.TempDir(), "x")}); err == nil {
		t.Fatal("copied out of the person's repository")
	}

	os.WriteFile(filepath.Join(repo, "notes.txt"), []byte("mine\n"), 0o600)
	head := gitOut(t, repo, "rev-parse", "HEAD")
	got, err := h.eng.DeliverCode(h.ctx, engine.DeliverInput{ProjectID: p.ID, RunID: runID, Method: engine.DeliverAdvance})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Branch, "agent-office/result-") || gitOut(t, repo, "rev-parse", got.Branch) != fc.Commit || !strings.Contains(got.Note, "커밋되지 않은 변경") {
		t.Fatalf("delivery = %+v", got)
	}
	if gitOut(t, repo, "rev-parse", "HEAD") != head || gitOut(t, repo, "status", "--porcelain") != "?? notes.txt" {
		t.Fatal("the person's work tree changed")
	}

	os.Remove(filepath.Join(repo, "notes.txt"))
	again, err := h.eng.DeliverCode(h.ctx, engine.DeliverInput{ProjectID: p.ID, RunID: runID, Method: engine.DeliverAdvance})
	if err != nil {
		t.Fatal(err)
	}
	if again.Branch != got.Branch || again.Note != "" || gitOut(t, repo, "rev-parse", "HEAD") != fc.Commit {
		t.Fatalf("second delivery = %+v", again)
	}
	if _, err := os.Stat(filepath.Join(repo, "api", "api.go")); err != nil {
		t.Fatal("the fast-forward did not update the files")
	}
	if fc := finalCode(t, h, p.ID, runID); !fc.Included {
		t.Fatalf("after advance = %+v", fc)
	}
}
