package engine

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-office/internal/storage"
	"agent-office/internal/workspace"
)

// Code delivery (company-requests plan §4.6). A finished run's code lives
// on the branches of its code steps. Delivering it makes that code the
// service's base for later runs and hands it to the person: the app's own
// repository moves its current branch and can be copied into a folder; a
// person's repository only gets a new branch, and its current branch moves
// only when asked, cleanly and fast-forward.

// Delivery methods.
const (
	DeliverBase    = "base"    // app repository: move its branch to the final code
	DeliverFolder  = "folder"  // app repository: base, then copy it into a folder
	DeliverBranch  = "branch"  // person's repository: a result branch
	DeliverAdvance = "advance" // person's repository: result branch, then fast-forward
)

// Delivery events, recorded on the run.
const (
	EvCodeDelivered      = "code.delivered"
	EvCodeDeliveryFailed = "code.delivery_failed"
)

// FinalStep is a code step whose last commit is part of a run's result.
type FinalStep struct {
	StepID string `json:"stepId"`
	Title  string `json:"title"`
	Commit string `json:"commit"`
}

// FinalCode describes the code a succeeded run produced and where it
// stands.
type FinalCode struct {
	RunID string `json:"runId"`
	Repo  string `json:"repo"`
	// AppRepo marks the repository the app keeps for a service without a
	// work folder.
	AppRepo bool   `json:"appRepo"`
	Base    string `json:"base"`
	// Steps are the code steps no other code step built on; several are
	// merged on delivery. None means the run changed no code.
	Steps []FinalStep `json:"steps"`
	// Commit is the final code when it is one step's commit or was merged
	// before; "" until a delivery merges several steps.
	Commit string         `json:"commit,omitempty"`
	Stat   workspace.Stat `json:"stat"`
	// Included reports whether the service's current base code already
	// holds every step's work.
	Included   bool           `json:"included"`
	Deliveries []CodeDelivery `json:"deliveries"`
}

// CodeDelivery is one recorded delivery attempt.
type CodeDelivery struct {
	Method    string `json:"method"`
	Commit    string `json:"commit,omitempty"`
	Branch    string `json:"branch,omitempty"`
	Folder    string `json:"folder,omitempty"`
	Workspace bool   `json:"workspace,omitempty"`
	// Note says what was left undone (an advance that was not possible).
	Note  string `json:"note,omitempty"`
	Error string `json:"error,omitempty"`
	At    string `json:"at"`
	// RequestItemID is the request action the delivery was made for.
	RequestItemID string `json:"requestItemId,omitempty"`
}

// DeliverInput asks for one delivery.
type DeliverInput struct {
	ProjectID string `json:"projectId"`
	RunID     string `json:"runId"`
	Method    string `json:"method"`
	// Folder is the target of a folder delivery: missing or empty.
	Folder string `json:"folder,omitempty"`
	// UseAsWorkspace makes the folder the service's work folder.
	UseAsWorkspace bool `json:"useAsWorkspace,omitempty"`
	// RequestItemID names the request action this delivery carries out.
	RequestItemID string `json:"requestItemId,omitempty"`
}

// FinalCode returns what a delivery of the run would hand over.
func (e *Engine) FinalCode(ctx context.Context, projectID, runID string) (FinalCode, error) {
	fc, _, err := e.finalCode(ctx, projectID, runID)
	return fc, err
}

func (e *Engine) finalCode(ctx context.Context, projectID, runID string) (FinalCode, runRepo, error) {
	fc := FinalCode{RunID: runID, Steps: []FinalStep{}, Deliveries: []CodeDelivery{}}
	if err := e.runInProject(ctx, projectID, runID); err != nil {
		return fc, runRepo{}, err
	}
	q := e.db.Read()
	st, err := loadState(ctx, q, runID)
	if err != nil {
		return fc, runRepo{}, err
	}
	if st.run.Status != RunSucceeded {
		return fc, runRepo{}, errors.New("성공한 실행의 코드만 전달할 수 있습니다")
	}
	rr, ok := loadRunRepo(ctx, q, runID)
	if !ok || rr.Kind != WsBase {
		return fc, rr, errors.New("이 실행은 Git 작업 공간 없이 폴더에서 바로 작업해, 결과가 이미 그 폴더에 있습니다")
	}
	g := e.cfg.Git
	if g == nil {
		return fc, rr, errors.New("Git을 찾을 수 없어 코드를 전달할 수 없습니다")
	}
	fc.Repo, fc.Base = rr.Repo, rr.Base
	fc.AppRepo = sameFolder(rr.Repo, filepath.Join(e.projectDir(projectID), "repo"))

	var all []FinalStep
	for _, id := range st.graph.Order() {
		n := st.graph.Node(id)
		a, ok := st.attempts[id]
		if !isCodeStep(n) || !ok || a.Status != StSucceeded {
			continue
		}
		var commit string
		q.QueryRowContext(ctx, `SELECT w.commit_sha FROM step_attempts s JOIN workspaces w ON w.id = s.workspace_id WHERE s.id = ? AND w.kind = 'code'`, a.ID).Scan(&commit)
		if commit != "" && commit != rr.Base { // a step that changed nothing has no code to hand over
			all = append(all, FinalStep{StepID: id, Title: n.Title, Commit: commit})
		}
	}
	for i, s := range all {
		contained := false
		for j, o := range all {
			if i == j {
				continue
			}
			if (s.Commit == o.Commit && j < i) || (s.Commit != o.Commit && g.IsAncestor(ctx, rr.Repo, s.Commit, o.Commit)) {
				contained = true
				break
			}
		}
		if !contained {
			fc.Steps = append(fc.Steps, s)
		}
	}
	if len(fc.Steps) == 0 {
		return fc, rr, nil
	}
	if len(fc.Steps) == 1 {
		fc.Commit = fc.Steps[0].Commit
	} else if c := g.BranchCommit(ctx, rr.Repo, finalBranch(runID)); c != "" && containsAll(ctx, g, rr.Repo, fc.Steps, c) {
		fc.Commit = c
	}
	if fc.Commit != "" {
		fc.Stat, _ = g.Stat(ctx, rr.Repo, rr.Base, fc.Commit)
	}
	if repo, head := e.serviceBase(ctx, projectID); head != "" {
		fc.Included = containsAll(ctx, g, repo, fc.Steps, head)
	}
	fc.Deliveries, err = e.codeDeliveries(ctx, runID)
	return fc, rr, err
}

// DeliverCode hands over a succeeded run's code (plan §4.6). A request
// that cannot apply is refused as it is; every delivery made, failed
// ones too, is recorded on the run.
func (e *Engine) DeliverCode(ctx context.Context, in DeliverInput) (CodeDelivery, error) {
	e.deliverMu.Lock()
	defer e.deliverMu.Unlock()
	fc, rr, folder, err := e.checkDelivery(ctx, in)
	if err != nil {
		return CodeDelivery{}, err
	}
	d, err := e.deliverCode(ctx, in, fc, rr, folder)
	d.Method, d.At, d.RequestItemID = in.Method, storage.Now(), in.RequestItemID
	if err != nil {
		d.Error = err.Error()
	}
	_, werr := e.db.Change(ctx, func(c *storage.Change) error {
		if err == nil && d.Workspace {
			if _, err := c.Tx.ExecContext(ctx, `UPDATE projects SET workspace_path = ?, updated_at = ? WHERE id = ?`, d.Folder, storage.Now(), in.ProjectID); err != nil {
				return err
			}
			if err := c.Emit(in.ProjectID, "", "", "project.updated", nil); err != nil {
				return err
			}
		}
		kind := EvCodeDelivered
		if err != nil {
			kind = EvCodeDeliveryFailed
		}
		return c.Emit(in.ProjectID, in.RunID, "", kind, d)
	})
	if err != nil {
		return d, err
	}
	return d, werr
}

// checkDelivery refuses a delivery that does not fit the run's
// repository, and checks the target folder.
func (e *Engine) checkDelivery(ctx context.Context, in DeliverInput) (fc FinalCode, rr runRepo, folder string, err error) {
	if fc, rr, err = e.finalCode(ctx, in.ProjectID, in.RunID); err != nil {
		return
	}
	if len(fc.Steps) == 0 {
		err = errors.New("이 실행에는 전달할 코드 결과가 없습니다")
		return
	}
	switch in.Method {
	case DeliverBase, DeliverFolder:
		if !fc.AppRepo {
			err = errors.New("이 서비스는 작업 폴더의 저장소에서 실행했습니다. 결과 브랜치로 전달하세요")
		}
	case DeliverBranch, DeliverAdvance:
		if fc.AppRepo {
			err = errors.New("이 서비스는 앱 저장소에서 실행했습니다. 기준 코드 갱신이나 폴더로 꺼내기로 전달하세요")
		}
	default:
		err = fmt.Errorf("알 수 없는 전달 방식입니다: %q", in.Method)
	}
	if err == nil && in.Method == DeliverFolder {
		folder, err = emptyFolder(in.Folder)
	}
	return
}

func (e *Engine) deliverCode(ctx context.Context, in DeliverInput, fc FinalCode, rr runRepo, folder string) (CodeDelivery, error) {
	var d CodeDelivery
	var err error
	g := e.cfg.Git
	if d.Commit, err = e.mergeFinal(ctx, g, rr.Repo, in.RunID, fc); err != nil {
		return d, err
	}
	switch in.Method {
	case DeliverBase, DeliverFolder:
		if err := advanceAppRepo(ctx, g, rr.Repo, d.Commit, in.RunID); err != nil {
			return d, err
		}
		d.Branch = g.CurrentBranch(ctx, rr.Repo)
		if in.Method == DeliverFolder {
			if err := g.Clone(ctx, rr.Repo, folder); err != nil {
				return d, fmt.Errorf("폴더로 꺼내지 못했습니다: %w", err)
			}
			d.Folder, d.Workspace = folder, in.UseAsWorkspace
		}
	case DeliverBranch, DeliverAdvance:
		if d.Branch, err = resultBranch(ctx, g, rr.Repo, in.RunID, d.Commit); err != nil {
			return d, err
		}
		if in.Method == DeliverAdvance {
			d.Note = advanceUserRepo(ctx, g, rr.Repo, d.Commit)
		}
	}
	return d, nil
}

// mergeFinal returns the final code, merging several steps' commits on
// the run's final branch in a scratch worktree. A conflict leaves no
// branch behind.
func (e *Engine) mergeFinal(ctx context.Context, g *workspace.Git, repo, runID string, fc FinalCode) (string, error) {
	if fc.Commit != "" {
		return fc.Commit, nil
	}
	branch := finalBranch(runID)
	if g.BranchCommit(ctx, repo, branch) != "" {
		if err := g.DeleteBranch(ctx, repo, branch); err != nil { // left from an older, different result
			return "", err
		}
	}
	dir := filepath.Join(e.cfg.DataDir, "wt", shortID(runID)+"-final")
	os.RemoveAll(dir)
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return "", err
	}
	if err := g.AddWorktree(ctx, repo, dir, branch, fc.Steps[0].Commit); err != nil {
		return "", err
	}
	defer g.RemoveWorktree(ctx, repo, dir, true)
	for _, s := range fc.Steps[1:] {
		conflicts, err := g.Merge(ctx, dir, s.Commit, fmt.Sprintf("Agent Office: %s 결과 합치기 (실행 %s)", s.Title, shortID(runID)))
		if err == nil && len(conflicts) == 0 {
			continue
		}
		g.AbortMerge(ctx, dir)
		g.RemoveWorktree(ctx, repo, dir, true)
		g.DeleteBranch(ctx, repo, branch)
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("코드 단계의 결과를 합치다 충돌이 났습니다(%s). 충돌을 해결하는 업무를 만들어 처리하세요", strings.Join(conflicts, ", "))
	}
	return g.Head(ctx, dir)
}

// advanceAppRepo moves the app repository's current branch to commit,
// merging when the base moved on since the run started.
func advanceAppRepo(ctx context.Context, g *workspace.Git, repo, commit, runID string) error {
	head, err := g.Head(ctx, repo)
	if err != nil {
		return err
	}
	if g.IsAncestor(ctx, repo, commit, head) {
		return nil
	}
	if dirty, err := g.Dirty(ctx, repo); err != nil || dirty {
		return fmt.Errorf("앱 저장소에 커밋되지 않은 변경이 있어 기준 코드를 바꾸지 않았습니다 (%s)", repo)
	}
	if g.IsAncestor(ctx, repo, head, commit) {
		return g.FastForward(ctx, repo, commit)
	}
	conflicts, err := g.Merge(ctx, repo, commit, fmt.Sprintf("Agent Office: 실행 %s 결과 반영", shortID(runID)))
	if err != nil || len(conflicts) > 0 {
		g.AbortMerge(ctx, repo)
		if err != nil {
			return err
		}
		return fmt.Errorf("기준 코드와 결과가 충돌합니다(%s). 충돌을 해결하는 업무를 만들어 처리하세요", strings.Join(conflicts, ", "))
	}
	return nil
}

// resultBranch names the result in a person's repository, reusing a
// branch already at commit.
func resultBranch(ctx context.Context, g *workspace.Git, repo, runID, commit string) (string, error) {
	base := "agent-office/result-" + shortID(runID)
	for i := 1; ; i++ {
		name := base
		if i > 1 {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		switch g.BranchCommit(ctx, repo, name) {
		case commit:
			return name, nil
		case "":
			return name, g.CreateBranch(ctx, repo, name, commit)
		}
	}
}

// advanceUserRepo fast-forwards a person's current branch to commit when
// that is safe, and otherwise says why it was left alone.
func advanceUserRepo(ctx context.Context, g *workspace.Git, repo, commit string) string {
	branch := g.CurrentBranch(ctx, repo)
	if branch == "" {
		return "작업 폴더가 특정 브랜치에 있지 않아 현재 브랜치에는 반영하지 않았습니다"
	}
	head, err := g.Head(ctx, repo)
	if err != nil {
		return "작업 폴더의 현재 커밋을 읽지 못해 반영하지 않았습니다"
	}
	if g.IsAncestor(ctx, repo, commit, head) {
		return ""
	}
	if dirty, err := g.Dirty(ctx, repo); err != nil || dirty {
		return fmt.Sprintf("작업 폴더에 커밋되지 않은 변경이 있어 %s 브랜치에는 반영하지 않았습니다", branch)
	}
	if !g.IsAncestor(ctx, repo, head, commit) {
		return fmt.Sprintf("%s 브랜치가 실행 뒤에 바뀌어 빨리 감기로 반영할 수 없습니다. 결과 브랜치를 직접 합치세요", branch)
	}
	if err := g.FastForward(ctx, repo, commit); err != nil {
		return fmt.Sprintf("%s 브랜치에 반영하지 못했습니다: %v", branch, err)
	}
	return ""
}

// serviceBase returns the repository the service's next run would use and
// the commit it would start from ("" when there is none).
func (e *Engine) serviceBase(ctx context.Context, projectID string) (repo, head string) {
	g := e.cfg.Git
	var path string
	e.db.Read().QueryRowContext(ctx, `SELECT workspace_path FROM projects WHERE id = ?`, projectID).Scan(&path)
	repo = filepath.Join(e.projectDir(projectID), "repo")
	if path != "" {
		root, err := g.Root(ctx, path)
		if err != nil {
			return "", ""
		}
		repo = root
	}
	head, _ = g.Head(ctx, repo)
	return repo, head
}

func (e *Engine) codeDeliveries(ctx context.Context, runID string) ([]CodeDelivery, error) {
	rows, err := e.db.Read().QueryContext(ctx, `SELECT payload FROM execution_events WHERE run_id = ? AND kind IN (?, ?) ORDER BY sequence`,
		runID, EvCodeDelivered, EvCodeDeliveryFailed)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CodeDelivery{}
	for rows.Next() {
		var raw string
		var d CodeDelivery
		if rows.Scan(&raw) == nil && json.Unmarshal([]byte(raw), &d) == nil {
			out = append(out, d)
		}
	}
	return out, rows.Err()
}

func finalBranch(runID string) string { return "agent-office/" + shortID(runID) + "/final" }

func containsAll(ctx context.Context, g *workspace.Git, repo string, steps []FinalStep, commit string) bool {
	for _, s := range steps {
		if !g.IsAncestor(ctx, repo, s.Commit, commit) {
			return false
		}
	}
	return true
}

// emptyFolder accepts an absolute folder that is missing or empty.
func emptyFolder(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" || !filepath.IsAbs(p) {
		return "", errors.New("꺼낼 폴더를 절대 경로로 골라 주세요")
	}
	p = filepath.Clean(p)
	entries, err := os.ReadDir(p)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return "", fmt.Errorf("폴더를 읽을 수 없습니다: %w", err)
	}
	if len(entries) > 0 {
		return "", errors.New("비어 있지 않은 폴더에는 꺼내지 않습니다. 새 폴더나 빈 폴더를 골라 주세요")
	}
	return p, nil
}

func sameFolder(a, b string) bool {
	ra, err1 := filepath.EvalSymlinks(a)
	rb, err2 := filepath.EvalSymlinks(b)
	if err1 != nil || err2 != nil {
		ra, rb = filepath.Clean(a), filepath.Clean(b)
	}
	return strings.EqualFold(ra, rb)
}
