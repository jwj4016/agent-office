// Package workspace manages Git worktrees, files and artifact path validation.
package workspace

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Git runs git commands for the engine. Every command gets the arguments
// as separate values (never through a shell), no terminal prompts, and
// the app's own author identity so the user's git config is never
// changed.
type Git struct {
	Exe string
	// Timeout bounds one command (default 2m).
	Timeout time.Duration
}

// FindGit returns git from PATH, or nil when it is not installed.
func FindGit() *Git {
	exe, err := exec.LookPath("git")
	if err != nil {
		return nil
	}
	return &Git{Exe: exe}
}

// Author is the identity of commits the app makes.
const (
	AuthorName  = "Agent Office"
	AuthorEmail = "agent-office@localhost"
)

// ErrNoCommit means the repository has no commit yet (unborn HEAD).
var ErrNoCommit = errors.New("저장소에 커밋이 없습니다")

func (g *Git) env() []string {
	keep := []string{"PATH", "HOME", "USERPROFILE", "SystemRoot", "ComSpec", "PATHEXT", "TEMP", "TMP", "TMPDIR", "LANG", "LC_ALL", "XDG_CONFIG_HOME", "APPDATA", "LOCALAPPDATA", "HOMEDRIVE", "HOMEPATH"}
	var env []string
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return append(env,
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME="+AuthorName, "GIT_AUTHOR_EMAIL="+AuthorEmail,
		"GIT_COMMITTER_NAME="+AuthorName, "GIT_COMMITTER_EMAIL="+AuthorEmail,
	)
}

// run executes git in dir and returns trimmed stdout.
func (g *Git) run(ctx context.Context, dir string, args ...string) (string, error) {
	timeout := g.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	full := append([]string{"-c", "core.quotepath=false", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + os.DevNull}, args...)
	cmd := exec.CommandContext(ctx, g.Exe, full...)
	cmd.Dir = dir
	cmd.Env = g.env()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = strings.TrimSpace(out.String())
		}
		return out.String(), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, msg)
	}
	return strings.TrimRight(out.String(), "\r\n"), nil
}

// Root returns the top level of the work tree that contains dir.
func (g *Git) Root(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.FromSlash(out)), nil
}

// Head returns the commit HEAD points at.
func (g *Git) Head(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, "rev-parse", "--verify", "--quiet", "HEAD^{commit}")
	if err != nil || out == "" {
		return "", ErrNoCommit
	}
	return out, nil
}

// Dirty reports whether the work tree has changes not in HEAD
// (including untracked files that are not ignored).
func (g *Git) Dirty(ctx context.Context, dir string) (bool, error) {
	out, err := g.run(ctx, dir, "status", "--porcelain")
	return out != "", err
}

// Init creates a repository with one empty commit, for services that
// have no workspace folder of their own.
func (g *Git) Init(ctx context.Context, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	if head, err := g.Head(ctx, dir); err == nil {
		if root, rerr := g.Root(ctx, dir); rerr == nil && sameDir(root, dir) {
			return head, nil
		}
	}
	if _, err := g.run(ctx, dir, "init", "--quiet"); err != nil {
		return "", err
	}
	if _, err := g.run(ctx, dir, "commit", "--quiet", "--allow-empty", "-m", "Agent Office: 시작"); err != nil {
		return "", err
	}
	return g.Head(ctx, dir)
}

func sameDir(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return strings.EqualFold(filepath.Clean(ra), filepath.Clean(rb)) || filepath.Clean(ra) == filepath.Clean(rb)
}

// AddWorktree checks out a new branch at start in path.
func (g *Git) AddWorktree(ctx context.Context, repo, path, branch, start string) error {
	_, err := g.run(ctx, repo, "worktree", "add", "--quiet", "-b", branch, path, start)
	return err
}

// AddDetached checks out commit in path without a branch (a view).
func (g *Git) AddDetached(ctx context.Context, repo, path, commit string) error {
	_, err := g.run(ctx, repo, "worktree", "add", "--quiet", "--detach", path, commit)
	return err
}

// RemoveWorktree removes a worktree. Without force git refuses when the
// worktree has uncommitted changes, which keeps unsaved work.
func (g *Git) RemoveWorktree(ctx context.Context, repo, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	_, err := g.run(ctx, repo, append(args, path)...)
	if err == nil {
		g.run(ctx, repo, "worktree", "prune")
	}
	return err
}

// IsAncestor reports whether a is an ancestor of (or equal to) b.
func (g *Git) IsAncestor(ctx context.Context, dir, a, b string) bool {
	_, err := g.run(ctx, dir, "merge-base", "--is-ancestor", a, b)
	return err == nil
}

// Merge merges commit into the work tree's HEAD. On conflict the merge is
// left in progress and the conflicted paths are returned, so whoever
// works there can resolve them.
func (g *Git) Merge(ctx context.Context, dir, commit, message string) ([]string, error) {
	if g.IsAncestor(ctx, dir, commit, "HEAD") {
		return nil, nil
	}
	_, err := g.run(ctx, dir, "merge", "--no-ff", "--no-edit", "-m", message, commit)
	if err == nil {
		return nil, nil
	}
	conflicts, cerr := g.Unmerged(ctx, dir)
	if cerr == nil && len(conflicts) > 0 {
		return conflicts, nil
	}
	return nil, err
}

// AbortMerge abandons an in-progress merge.
func (g *Git) AbortMerge(ctx context.Context, dir string) error {
	_, err := g.run(ctx, dir, "merge", "--abort")
	return err
}

// Unmerged lists paths with unresolved merge conflicts.
func (g *Git) Unmerged(ctx context.Context, dir string) ([]string, error) {
	out, err := g.run(ctx, dir, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, err
	}
	return lines(out), nil
}

// MergeInProgress reports whether a merge waits to be concluded.
func (g *Git) MergeInProgress(ctx context.Context, dir string) bool {
	_, err := g.run(ctx, dir, "rev-parse", "--verify", "--quiet", "MERGE_HEAD")
	return err == nil
}

// StageAll stages every change (honouring .gitignore). Staging a file
// that had a merge conflict marks it resolved, so callers check its
// content for conflict markers.
func (g *Git) StageAll(ctx context.Context, dir string) error {
	_, err := g.run(ctx, dir, "add", "--all")
	return err
}

// Staged lists paths whose staged content differs from HEAD, without
// deleted ones.
func (g *Git) Staged(ctx context.Context, dir string) ([]string, error) {
	out, err := g.run(ctx, dir, "diff", "--cached", "--name-only", "--diff-filter=d")
	if err != nil {
		return nil, err
	}
	return lines(out), nil
}

// CommitAll stages every change (honouring .gitignore) and commits it,
// concluding a merge in progress. With nothing to commit it returns the
// current HEAD.
func (g *Git) CommitAll(ctx context.Context, dir, message string) (string, error) {
	if err := g.StageAll(ctx, dir); err != nil {
		return "", err
	}
	staged, _ := g.run(ctx, dir, "diff", "--cached", "--name-only")
	if staged != "" || g.MergeInProgress(ctx, dir) {
		if _, err := g.run(ctx, dir, "commit", "--quiet", "--no-verify", "-m", message); err != nil {
			return "", err
		}
	}
	return g.Head(ctx, dir)
}

// Change is one changed path between two commits.
type Change struct {
	Path   string `json:"path"`
	Status string `json:"status"` // added | modified | deleted | renamed | copied | typechange
	From   string `json:"from,omitempty"`
}

var statusNames = map[byte]string{'A': "added", 'M': "modified", 'D': "deleted", 'R': "renamed", 'C': "copied", 'T': "typechange"}

// Changes lists the paths changed from from to to.
func (g *Git) Changes(ctx context.Context, dir, from, to string) ([]Change, error) {
	out, err := g.run(ctx, dir, "diff", "--name-status", "-M", from, to)
	if err != nil {
		return nil, err
	}
	var list []Change
	for _, l := range lines(out) {
		f := strings.Split(l, "\t")
		if len(f) < 2 || f[0] == "" {
			continue
		}
		c := Change{Status: statusNames[f[0][0]], Path: f[len(f)-1]}
		if c.Status == "" {
			c.Status = "modified"
		}
		if len(f) == 3 {
			c.From = f[1]
		}
		list = append(list, c)
	}
	return list, nil
}

// Stat counts changed files and lines from from to to.
type Stat struct {
	Files      int `json:"files"`
	Insertions int `json:"insertions"`
	Deletions  int `json:"deletions"`
}

func (g *Git) Stat(ctx context.Context, dir, from, to string) (Stat, error) {
	out, err := g.run(ctx, dir, "diff", "--numstat", "-M", from, to)
	if err != nil {
		return Stat{}, err
	}
	var s Stat
	for _, l := range lines(out) {
		f := strings.SplitN(l, "\t", 3)
		if len(f) < 3 {
			continue
		}
		s.Files++
		a, _ := strconv.Atoi(f[0]) // "-" for binary files counts as 0
		d, _ := strconv.Atoi(f[1])
		s.Insertions += a
		s.Deletions += d
	}
	return s, nil
}

// Diff returns the unified diff from from to to, cut at limit bytes.
func (g *Git) Diff(ctx context.Context, dir, from, to string, limit int) (string, bool, error) {
	out, err := g.run(ctx, dir, "diff", "-M", "--no-color", from, to)
	if err != nil {
		return "", false, err
	}
	if len(out) > limit {
		return out[:limit], true, nil
	}
	return out, false, nil
}

// ConflictMarkers lists files under dir (among paths) that still contain
// conflict markers.
func ConflictMarkers(dir string, paths []string) []string {
	var bad []string
	for _, p := range paths {
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(p)))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64<<10), 4<<20)
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "<<<<<<< ") || strings.HasPrefix(line, ">>>>>>> ") {
				bad = append(bad, p)
				break
			}
		}
		f.Close()
	}
	return bad
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}
