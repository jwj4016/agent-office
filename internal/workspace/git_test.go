package workspace

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newGit(t *testing.T) *Git {
	t.Helper()
	g := FindGit()
	if g == nil {
		t.Skip("git is not installed")
	}
	return g
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// Two worktrees branch from the same base, each commits, and a third
// merges both — the parallel development shape (T14). Paths use Korean
// and spaces (T21).
func TestWorktreesMergeAndConflict(t *testing.T) {
	g := newGit(t)
	ctx := context.Background()
	root := filepath.Join(t.TempDir(), "작업 폴더")
	repo := filepath.Join(root, "repo")
	base, err := g.Init(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := g.Init(ctx, repo); err != nil || again != base {
		t.Fatalf("Init is not idempotent: %s %v", again, err)
	}
	be, fe, in := filepath.Join(root, "백엔드 wt"), filepath.Join(root, "fe"), filepath.Join(root, "integrate")
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(g.AddWorktree(ctx, repo, be, "ao/be", base))
	must(g.AddWorktree(ctx, repo, fe, "ao/fe", base))
	write(t, be, "api/서버.go", "package api\n")
	write(t, be, "shared.txt", "backend\n")
	write(t, fe, "web/app.js", "console.log(1)\n")
	beCommit, err := g.CommitAll(ctx, be, "백엔드")
	must(err)
	feCommit, err := g.CommitAll(ctx, fe, "프론트엔드")
	must(err)
	if beCommit == base || feCommit == base {
		t.Fatal("nothing committed")
	}
	if same, _ := g.CommitAll(ctx, fe, "no change"); same != feCommit {
		t.Fatal("empty commit created")
	}

	must(g.AddWorktree(ctx, repo, in, "ao/integrate", base))
	for _, c := range []string{beCommit, feCommit} {
		if conflicts, err := g.Merge(ctx, in, c, "merge"); err != nil || len(conflicts) > 0 {
			t.Fatalf("merge %s: %v %v", c, conflicts, err)
		}
	}
	head, err := g.CommitAll(ctx, in, "통합")
	must(err)
	changes, err := g.Changes(ctx, in, base, head)
	must(err)
	got := map[string]string{}
	for _, c := range changes {
		got[c.Path] = c.Status
	}
	if got["api/서버.go"] != "added" || got["web/app.js"] != "added" || len(got) != 3 {
		t.Fatalf("changes = %+v", changes)
	}
	if st, _ := g.Stat(ctx, in, base, head); st.Files != 3 || st.Insertions != 3 {
		t.Fatalf("stat = %+v", st)
	}
	if patch, cut, _ := g.Diff(ctx, in, base, head, 1<<20); cut || !strings.Contains(patch, "+package api") {
		t.Fatalf("patch = %q", patch)
	}

	// A conflicting change is left for someone to resolve.
	write(t, fe, "shared.txt", "frontend\n")
	fe2, err := g.CommitAll(ctx, fe, "conflict")
	must(err)
	conflicts, err := g.Merge(ctx, in, fe2, "merge")
	if err != nil || len(conflicts) != 1 || conflicts[0] != "shared.txt" {
		t.Fatalf("conflicts = %v %v", conflicts, err)
	}
	if bad := ConflictMarkers(in, []string{"shared.txt"}); len(bad) != 1 {
		t.Fatal("conflict markers not found")
	}
	write(t, in, "shared.txt", "backend and frontend\n")
	resolved, err := g.CommitAll(ctx, in, "resolve")
	must(err)
	if g.MergeInProgress(ctx, in) || !g.IsAncestor(ctx, in, fe2, resolved) {
		t.Fatal("merge not concluded")
	}

	// Removing refuses to drop uncommitted work; a clean one goes.
	write(t, be, "unsaved.txt", "x")
	if err := g.RemoveWorktree(ctx, repo, be, false); err == nil {
		t.Fatal("removed a worktree with uncommitted work")
	}
	if err := g.RemoveWorktree(ctx, repo, fe, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(fe); !os.IsNotExist(err) {
		t.Fatal("worktree folder still there")
	}
	// The user's repo work tree was never touched.
	if dirty, _ := g.Dirty(ctx, repo); dirty {
		t.Fatal("repository work tree changed")
	}
}

func TestHeadOfEmptyRepo(t *testing.T) {
	g := newGit(t)
	dir := t.TempDir()
	if _, err := g.run(context.Background(), dir, "init", "--quiet"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Head(context.Background(), dir); err != ErrNoCommit {
		t.Fatalf("err = %v", err)
	}
}
