package engine

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-office/internal/domain"
	"agent-office/internal/storage"
	"agent-office/internal/workspace"
)

// Code isolation (spec §10.1). When the service works in a Git
// repository (its own folder, or a repository the app keeps for it),
// every step that produces a code_change gets its own worktree on its
// own branch, started from the run's base commit plus the commits of the
// code steps upstream of it; other steps get a read-only checkout. The
// app commits a code step's work itself and records what changed, so a
// result never depends on what a model says it changed. The user's own
// work tree and uncommitted changes are never touched.
//
// Without Git the run falls back to one shared folder, and code steps of
// a project then run one at a time so two never write the same files.

// Workspace kinds (workspaces.kind).
const (
	WsBase   = "base"
	WsShared = "shared"
	WsCode   = "code"
	WsView   = "view"
)

// Workspace statuses.
const (
	WsActive    = "active"
	WsCommitted = "committed"
	WsKept      = "kept" // left in place because it holds uncommitted work
	WsRemoved   = "removed"
)

// patchLimit bounds the diff stored in a code_change result.
const patchLimit = 200 << 10

// isCodeStep reports whether a step produces code changes.
func isCodeStep(n *domain.Node) bool {
	for _, o := range n.Outputs {
		if o.Type == domain.OutCodeChange {
			return true
		}
	}
	return false
}

// runRepo is where a run's code lives.
type runRepo struct {
	ID     string
	Kind   string // base | shared
	Repo   string // repository root (base)
	Path   string // shared folder, or repository root
	Subdir string // the service's folder inside the repository
	Base   string // base commit
	Note   string
}

// planRunRepo decides, before a run starts, where its code lives.
func (e *Engine) planRunRepo(ctx context.Context, projectID, runID, workspacePath string) (runRepo, error) {
	g := e.cfg.Git
	if workspacePath == "" {
		if g == nil {
			dir := filepath.Join(e.projectDir(projectID), "runs", runID, "work")
			return runRepo{Kind: WsShared, Path: dir, Note: "Git이 설치되어 있지 않아 실행별 공유 폴더를 사용합니다. 코드 업무는 하나씩 실행됩니다"}, os.MkdirAll(dir, 0o700)
		}
		repo := filepath.Join(e.projectDir(projectID), "repo")
		base, err := g.Init(ctx, repo)
		if err != nil {
			return runRepo{}, fmt.Errorf("서비스 저장소를 만들 수 없습니다: %w", err)
		}
		return runRepo{Kind: WsBase, Repo: repo, Path: repo, Base: base}, nil
	}
	shared := runRepo{Kind: WsShared, Path: workspacePath}
	if g == nil {
		shared.Note = "Git이 설치되어 있지 않아 작업 폴더를 그대로 사용합니다. 코드 업무는 하나씩 실행됩니다"
		return shared, nil
	}
	root, err := g.Root(ctx, workspacePath)
	if err != nil {
		shared.Note = "작업 폴더가 Git 저장소가 아니어서 폴더를 그대로 사용합니다. 코드 업무는 하나씩 실행되고, 변경은 분리되지 않습니다"
		return shared, nil
	}
	base, err := g.Head(ctx, root)
	if err != nil {
		shared.Note = "작업 폴더의 Git 저장소에 커밋이 없어 폴더를 그대로 사용합니다. 첫 커밋을 만들면 업무별 작업 공간이 분리됩니다"
		return shared, nil
	}
	rr := runRepo{Kind: WsBase, Repo: root, Path: root, Base: base}
	if rel, err := filepath.Rel(root, workspacePath); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		rr.Subdir = rel
	}
	if dirty, _ := g.Dirty(ctx, root); dirty {
		rr.Note = fmt.Sprintf("작업 폴더에 커밋되지 않은 변경이 있습니다. 실행은 마지막 커밋 %s에서 시작하며, 그 변경은 사용하지도 건드리지도 않습니다", short(base))
	}
	return rr, nil
}

// insertRunRepo records the run's repository in the run-creating change.
func insertRunRepo(ctx context.Context, c *storage.Change, projectID, runID string, rr runRepo) error {
	_, err := c.Tx.ExecContext(ctx, `INSERT INTO workspaces (id, project_id, run_id, path, base_commit, status, created_at, kind, repo, subdir, note)
		VALUES (?, ?, ?, ?, ?, 'ready', ?, ?, ?, ?, ?)`,
		storage.NewID("ws"), projectID, runID, rr.Path, rr.Base, storage.Now(), rr.Kind, rr.Repo, rr.Subdir, rr.Note)
	return err
}

// loadRunRepo returns the run's repository, or ok=false for runs started
// before workspaces were recorded.
func loadRunRepo(ctx context.Context, q querier, runID string) (runRepo, bool) {
	var rr runRepo
	err := q.QueryRowContext(ctx, `SELECT id, kind, repo, path, subdir, base_commit, note FROM workspaces WHERE run_id = ? AND kind IN ('base', 'shared')`, runID).
		Scan(&rr.ID, &rr.Kind, &rr.Repo, &rr.Path, &rr.Subdir, &rr.Base, &rr.Note)
	return rr, err == nil
}

// stepWorkspace is where one attempt works.
type stepWorkspace struct {
	ID        string
	Kind      string // code | view | shared
	Root      string // worktree root (or the shared folder)
	Dir       string // working directory: Root plus the service's subfolder
	Repo      string
	Branch    string
	Base      string // the run's base commit
	Start     string // commit the attempt started from
	Commit    string // commit the app made at the end (code)
	Conflicts []string
	Pending   []string // commits to merge once the conflict is resolved
	Note      string
}

// ReadOnly reports whether the attempt must not change files.
func (w stepWorkspace) ReadOnly() bool { return w.Kind == WsView }

func shortID(id string) string {
	if i := strings.LastIndexByte(id, '-'); i >= 0 {
		id = id[i+1:]
	}
	if len(id) > 8 {
		id = id[:8]
	}
	return id
}

// prepareWorkspace creates the workspace of attempt a of step n and links
// it to the attempt. It runs outside any database transaction because Git
// may take a while.
func (e *Engine) prepareWorkspace(ctx context.Context, st *runState, n *domain.Node, a attemptRow) (stepWorkspace, error) {
	q := e.db.Read()
	rr, ok := loadRunRepo(ctx, q, st.run.ID)
	if !ok {
		dir, err := e.workspaceDir(st)
		return stepWorkspace{Kind: WsShared, Root: dir, Dir: dir}, err
	}
	if rr.Kind == WsShared {
		ws := stepWorkspace{ID: rr.ID, Kind: WsShared, Root: rr.Path, Dir: rr.Path, Note: rr.Note}
		return ws, e.linkWorkspace(ctx, a.ID, rr.ID)
	}
	g := e.cfg.Git
	if g == nil {
		return stepWorkspace{}, errors.New("이 실행은 Git 작업 공간을 쓰는데 Git을 찾을 수 없습니다")
	}
	upstream, err := e.upstreamCommits(ctx, q, st, n, rr.Repo)
	if err != nil {
		return stepWorkspace{}, err
	}
	ws := stepWorkspace{Repo: rr.Repo, Base: rr.Base, Root: filepath.Join(e.cfg.DataDir, "wt", shortID(st.run.ID)+"-"+shortID(a.ID))}
	ws.Dir = filepath.Join(ws.Root, rr.Subdir)
	if err := os.MkdirAll(filepath.Dir(ws.Root), 0o700); err != nil {
		return ws, err
	}
	if isCodeStep(n) {
		ws.Kind = WsCode
		ws.Start = rr.Base
		if prev := previousCommit(ctx, q, st.run.ID, n.ID); prev != "" {
			ws.Start = prev // a rework or retry continues from the last work
		}
		ws.Branch = fmt.Sprintf("agent-office/%s/%s-g%d-a%d", shortID(st.run.ID), n.ID, a.Generation, a.Attempt)
		if err := g.AddWorktree(ctx, rr.Repo, ws.Root, ws.Branch, ws.Start); err != nil {
			return ws, err
		}
		for i, c := range upstream {
			conflicts, err := g.Merge(ctx, ws.Root, c.commit, fmt.Sprintf("Agent Office: %s 변경 합치기", c.title))
			if err != nil {
				return ws, err
			}
			if len(conflicts) > 0 {
				ws.Conflicts = conflicts
				for _, rest := range upstream[i+1:] {
					ws.Pending = append(ws.Pending, rest.commit)
				}
				ws.Note = fmt.Sprintf("%s의 변경을 합치다 충돌이 났습니다", c.title)
				break
			}
		}
	} else {
		ws.Kind = WsView
		ws.Start = rr.Base
		var rest []upstreamCommit
		if len(upstream) > 0 {
			ws.Start, rest = upstream[0].commit, upstream[1:]
		}
		if err := g.AddDetached(ctx, rr.Repo, ws.Root, ws.Start); err != nil {
			return ws, err
		}
		for _, c := range rest {
			conflicts, err := g.Merge(ctx, ws.Root, c.commit, "Agent Office: 확인용으로 합치기")
			if err != nil || len(conflicts) > 0 {
				g.AbortMerge(ctx, ws.Root)
				ws.Note = fmt.Sprintf("%s의 변경은 자동으로 합칠 수 없어 이 작업 공간에 없습니다", c.title)
			}
		}
	}
	ws.ID = storage.NewID("ws")
	conflicts, _ := json.Marshal(nonNil(ws.Conflicts))
	pending, _ := json.Marshal(nonNil(ws.Pending))
	_, err = e.db.Change(ctx, func(c *storage.Change) error {
		if _, err := c.Tx.ExecContext(ctx, `INSERT INTO workspaces (id, project_id, run_id, assignment_id, path, base_commit, branch, status, created_at, kind, repo, subdir, start_commit, pending_merges, conflicts, note)
			VALUES (?, ?, ?, NULLIF(?, ''), ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?)`,
			ws.ID, st.run.ProjectID, st.run.ID, n.AssignmentID, ws.Root, ws.Base, ws.Branch, storage.Now(), ws.Kind, ws.Repo, rr.Subdir, ws.Start,
			string(pending), string(conflicts), ws.Note); err != nil {
			return err
		}
		if _, err := c.Tx.ExecContext(ctx, `UPDATE step_attempts SET workspace_id = ? WHERE id = ?`, ws.ID, a.ID); err != nil {
			return err
		}
		return c.Emit(st.run.ProjectID, st.run.ID, a.ID, "workspace.prepared", map[string]any{
			"kind": ws.Kind, "branch": ws.Branch, "start": ws.Start, "conflicts": nonNil(ws.Conflicts), "note": ws.Note,
		})
	})
	return ws, err
}

func uniq(xs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func (e *Engine) linkWorkspace(ctx context.Context, attemptID, wsID string) error {
	_, err := e.db.Change(ctx, func(c *storage.Change) error {
		_, err := c.Tx.ExecContext(ctx, `UPDATE step_attempts SET workspace_id = ? WHERE id = ?`, wsID, attemptID)
		return err
	})
	return err
}

type upstreamCommit struct{ commit, title string }

// upstreamCommits returns the commits of the code steps upstream of n
// that are not already contained in another one, in graph order.
func (e *Engine) upstreamCommits(ctx context.Context, q querier, st *runState, n *domain.Node, repo string) ([]upstreamCommit, error) {
	anc := st.graph.Ancestors(n.ID)
	var all []upstreamCommit
	for _, id := range st.graph.Order() {
		un := st.graph.Node(id)
		a, ok := st.attempts[id]
		if !anc[id] || !isCodeStep(un) || !ok || a.Status != StSucceeded {
			continue
		}
		var commit string
		q.QueryRowContext(ctx, `SELECT w.commit_sha FROM step_attempts s JOIN workspaces w ON w.id = s.workspace_id WHERE s.id = ?`, a.ID).Scan(&commit)
		if commit != "" {
			all = append(all, upstreamCommit{commit: commit, title: un.Title})
		}
	}
	var out []upstreamCommit
	for i, c := range all {
		contained := false
		for j, other := range all {
			if i != j && c.commit != other.commit && e.cfg.Git.IsAncestor(ctx, repo, c.commit, other.commit) {
				contained = true
				break
			}
			if j < i && c.commit == other.commit {
				contained = true
				break
			}
		}
		if !contained {
			out = append(out, c)
		}
	}
	return out, nil
}

// previousCommit is the last commit an attempt of this step made in this
// run (any generation), so reworks and retries build on earlier work.
func previousCommit(ctx context.Context, q querier, runID, stepID string) string {
	var commit string
	q.QueryRowContext(ctx, `SELECT w.commit_sha FROM workspaces w JOIN step_attempts s ON s.workspace_id = w.id
		WHERE s.run_id = ? AND s.step_id = ? AND w.kind = 'code' AND w.commit_sha <> ''
		ORDER BY s.generation DESC, s.attempt DESC LIMIT 1`, runID, stepID).Scan(&commit)
	return commit
}

// attemptWorkspace loads the workspace linked to an attempt.
func (e *Engine) attemptWorkspace(ctx context.Context, q querier, st *runState, attemptID string) (stepWorkspace, error) {
	var ws stepWorkspace
	var subdir, pending, conflicts string
	err := q.QueryRowContext(ctx, `SELECT w.id, w.kind, w.path, w.subdir, w.repo, w.branch, w.base_commit, w.start_commit, w.commit_sha, w.pending_merges, w.conflicts, w.note
		FROM step_attempts s JOIN workspaces w ON w.id = s.workspace_id WHERE s.id = ?`, attemptID).
		Scan(&ws.ID, &ws.Kind, &ws.Root, &subdir, &ws.Repo, &ws.Branch, &ws.Base, &ws.Start, &ws.Commit, &pending, &conflicts, &ws.Note)
	if errors.Is(err, sql.ErrNoRows) {
		dir, err := e.workspaceDir(st)
		return stepWorkspace{Kind: WsShared, Root: dir, Dir: dir}, err
	}
	if err != nil {
		return ws, err
	}
	ws.Dir = ws.Root
	if ws.Kind != WsShared {
		ws.Dir = filepath.Join(ws.Root, subdir)
	}
	json.Unmarshal([]byte(pending), &ws.Pending)
	json.Unmarshal([]byte(conflicts), &ws.Conflicts)
	return ws, nil
}

// WorkspaceInfo describes an attempt's workspace for the UI.
type WorkspaceInfo struct {
	Kind      string   `json:"kind"`
	Path      string   `json:"path"`
	Branch    string   `json:"branch,omitempty"`
	Start     string   `json:"start,omitempty"`
	Commit    string   `json:"commit,omitempty"`
	Status    string   `json:"status"`
	Conflicts []string `json:"conflicts"`
	Note      string   `json:"note,omitempty"`
}

// RepoInfo describes where a run's code lives.
type RepoInfo struct {
	Kind string `json:"kind"` // base | shared
	Path string `json:"path"`
	Base string `json:"base,omitempty"`
	Note string `json:"note,omitempty"`
}

func workspaceInfo(ctx context.Context, q querier, attemptID string) *WorkspaceInfo {
	var w WorkspaceInfo
	var subdir, conflicts string
	err := q.QueryRowContext(ctx, `SELECT w.kind, w.path, w.subdir, w.branch, w.start_commit, w.commit_sha, w.status, w.conflicts, w.note
		FROM step_attempts s JOIN workspaces w ON w.id = s.workspace_id WHERE s.id = ?`, attemptID).
		Scan(&w.Kind, &w.Path, &subdir, &w.Branch, &w.Start, &w.Commit, &w.Status, &conflicts, &w.Note)
	if err != nil {
		return nil
	}
	if w.Kind != WsShared && subdir != "" {
		w.Path = filepath.Join(w.Path, subdir)
	}
	json.Unmarshal([]byte(conflicts), &w.Conflicts)
	w.Conflicts = nonNil(w.Conflicts)
	return &w
}

func repoInfo(ctx context.Context, q querier, runID string) *RepoInfo {
	rr, ok := loadRunRepo(ctx, q, runID)
	if !ok {
		return nil
	}
	return &RepoInfo{Kind: rr.Kind, Path: rr.Path, Base: rr.Base, Note: rr.Note}
}

// CodeChange is the code_change result the app writes from Git. Fields a
// model wrote into the same file (a summary, the tests it ran) are kept
// next to these, which always come from the repository.
type CodeChange struct {
	BaseCommit     string             `json:"baseCommit"`
	StartCommit    string             `json:"startCommit"`
	Commit         string             `json:"commit"`
	Branch         string             `json:"branch"`
	Changes        []workspace.Change `json:"changes"`
	DiffStat       workspace.Stat     `json:"diffStat"`
	Patch          string             `json:"patch"`
	PatchTruncated bool               `json:"patchTruncated"`
}

// captureCode commits a code step's work and writes its code_change
// outputs into outDir. Unresolved conflicts fail the step; the work stays
// committed on the branch either way.
func (e *Engine) captureCode(ctx context.Context, st *runState, n *domain.Node, attemptID, outDir string) error {
	ws, err := e.attemptWorkspace(ctx, e.db.Read(), st, attemptID)
	if err != nil || ws.Kind != WsCode {
		return err
	}
	g := e.cfg.Git
	if g == nil {
		return errors.New("Git을 찾을 수 없어 코드 변경을 기록하지 못했습니다")
	}
	// Resolving a conflict means editing the files; nobody has to run
	// git add. Staging marks them resolved, so their content decides:
	// leftover markers fail the step and nothing is committed.
	if err := g.StageAll(ctx, ws.Root); err != nil {
		return err
	}
	staged, err := g.Staged(ctx, ws.Root)
	if err != nil {
		return err
	}
	if bad := workspace.ConflictMarkers(ws.Root, append(staged, ws.Conflicts...)); len(bad) > 0 {
		return fmt.Errorf("병합 충돌이 해결되지 않았습니다 (충돌 표시가 남은 파일: %s)", strings.Join(uniq(bad), ", "))
	}
	a, _, err := loadAttempt(ctx, e.db.Read(), attemptID)
	if err != nil {
		return err
	}
	commit, err := g.CommitAll(ctx, ws.Root, fmt.Sprintf("Agent Office: %s (실행 %s, %d-%d)", n.Title, shortID(st.run.ID), a.Generation, a.Attempt))
	if err != nil {
		return err
	}
	var problem error
	for _, c := range ws.Pending {
		if problem != nil {
			break
		}
		conflicts, err := g.Merge(ctx, ws.Root, c, "Agent Office: 남은 변경 합치기")
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			g.AbortMerge(ctx, ws.Root)
			problem = fmt.Errorf("다음 변경을 합치다 다시 충돌이 났습니다 (%s): %s. 다시 시도하면 지금까지의 작업에서 이어갑니다", short(c), strings.Join(conflicts, ", "))
			break
		}
		if commit, err = g.Head(ctx, ws.Root); err != nil {
			return err
		}
	}
	if _, err := e.db.Change(ctx, func(c *storage.Change) error {
		if _, err := c.Tx.ExecContext(ctx, `UPDATE workspaces SET commit_sha = ?, status = 'committed' WHERE id = ?`, commit, ws.ID); err != nil {
			return err
		}
		return c.Emit(st.run.ProjectID, st.run.ID, attemptID, "workspace.committed", map[string]string{"branch": ws.Branch, "commit": commit})
	}); err != nil {
		return err
	}
	if problem != nil {
		return problem
	}
	cc := CodeChange{BaseCommit: ws.Base, StartCommit: ws.Start, Commit: commit, Branch: ws.Branch}
	if cc.Changes, err = g.Changes(ctx, ws.Root, ws.Base, commit); err != nil {
		return err
	}
	if cc.Changes == nil {
		cc.Changes = []workspace.Change{}
	}
	cc.DiffStat, _ = g.Stat(ctx, ws.Root, ws.Base, commit)
	cc.Patch, cc.PatchTruncated, _ = g.Diff(ctx, ws.Root, ws.Base, commit, patchLimit)
	for _, o := range n.Outputs {
		if o.Type != domain.OutCodeChange {
			continue
		}
		path := outputPath(outDir, o)
		doc := map[string]any{}
		if data, err := os.ReadFile(path); err == nil {
			json.Unmarshal(data, &doc) // keep what the model said (summary, tests)
		}
		if doc == nil {
			doc = map[string]any{}
		}
		raw, _ := json.Marshal(cc)
		var facts map[string]any
		json.Unmarshal(raw, &facts)
		for k, v := range facts {
			doc[k] = v
		}
		out, _ := json.MarshalIndent(doc, "", "  ")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, out, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// cleanupWorkspaces removes the worktrees of finished runs. Branches stay
// (they hold the work); a worktree with uncommitted changes is kept.
func (e *Engine) cleanupWorkspaces(ctx context.Context) {
	g := e.cfg.Git
	if g == nil {
		return
	}
	rows, err := e.db.Read().QueryContext(ctx, `SELECT w.id, w.project_id, w.run_id, w.kind, w.repo, w.path FROM workspaces w JOIN runs r ON r.id = w.run_id
		WHERE r.status IN ('succeeded', 'cancelled') AND w.kind IN ('code', 'view') AND w.status IN ('active', 'committed')`)
	if err != nil {
		return
	}
	type wsRow struct{ id, project, run, kind, repo, path string }
	var list []wsRow
	for rows.Next() {
		var w wsRow
		rows.Scan(&w.id, &w.project, &w.run, &w.kind, &w.repo, &w.path)
		list = append(list, w)
	}
	rows.Close()
	for _, w := range list {
		status := WsRemoved
		if _, err := os.Stat(w.path); err == nil {
			// A view never holds work; a code worktree goes only when clean.
			if err := g.RemoveWorktree(ctx, w.repo, w.path, w.kind == WsView); err != nil {
				status = WsKept
			}
		}
		e.db.Change(ctx, func(c *storage.Change) error {
			if _, err := c.Tx.ExecContext(ctx, `UPDATE workspaces SET status = ? WHERE id = ?`, status, w.id); err != nil {
				return err
			}
			return c.Emit(w.project, w.run, "", "workspace."+status, map[string]string{"path": w.path})
		})
	}
}

// codeBusy reports whether a code step of the project is active; in a
// shared folder code steps run one at a time.
func (e *Engine) codeBusy(projectID string) bool {
	for _, a := range e.active {
		if a.projectID == projectID && a.code {
			return true
		}
	}
	return false
}

// writeWorkspace tells the agent where it works and what it may change.
func writeWorkspace(b *strings.Builder, ws stepWorkspace) {
	switch ws.Kind {
	case WsCode:
		b.WriteString("## 작업 공간\n\n")
		fmt.Fprintf(b, "- 위치: %s\n- 브랜치: %s (시작 커밋 %s)\n", ws.Dir, ws.Branch, short(ws.Start))
		b.WriteString("- 이 업무만 쓰는 Git worktree입니다. 다른 업무와 동시에 같은 파일을 쓰지 않습니다.\n")
		b.WriteString("- 작업이 끝나면 앱이 이 브랜치에 커밋하고 변경 목록을 기록합니다. 직접 커밋하거나 브랜치를 바꾸지 마세요.\n")
		if len(ws.Conflicts) > 0 {
			fmt.Fprintf(b, "- **병합 충돌**: %s. 다음 파일의 충돌 표시(<<<<<<<, =======, >>>>>>>)를 없애 두 변경을 모두 살려 해결하세요: %s\n", ws.Note, strings.Join(ws.Conflicts, ", "))
		}
		if len(ws.Pending) > 0 {
			b.WriteString("- 충돌을 해결하면 앱이 나머지 선행 변경도 이어서 합칩니다.\n")
		}
		b.WriteString("\n")
	case WsView:
		b.WriteString("## 작업 공간\n\n")
		fmt.Fprintf(b, "- 위치: %s (커밋 %s, 읽기 전용)\n- 이 업무는 코드를 바꾸지 않습니다. 결과는 아래 결과 경로에만 저장하세요.\n", ws.Dir, short(ws.Start))
		if ws.Note != "" {
			fmt.Fprintf(b, "- 참고: %s\n", ws.Note)
		}
		b.WriteString("\n")
	}
}
