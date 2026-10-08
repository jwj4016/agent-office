package engine_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
)

// M4 gate: one run that skips an unneeded branch and still joins, holds
// a design meeting, develops backend and frontend in parallel worktrees
// (the backend asks the architect a question on the way), integrates and
// really builds the merged code, takes a code review back to the backend
// for one revision round, and finishes.
func TestM4GateBranchParallelRevision(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not on PATH")
	}
	h, g := gitHarness(t)
	roles := map[string]string{"a-planner": "기획", "a-architect": "설계", "a-backend": "백엔드", "a-frontend": "프론트엔드", "a-qa": "QA"}
	p := testenv.Seed(t, h.db, "게임", "workflows/m4-gate.json", roles, []string{"a-owner"})
	arch := p.Assignments["a-architect"]
	h.prov.Scripts["gate-plan"] = []providers.Step{
		step(t, `{"write": {"key": "spec", "content": "# 작은 웹 게임\n\n- 점수 API와 화면"}}`),
		step(t, `{"write": {"key": "scope", "content": "{\"legal\": {\"needed\": false}}"}}`), done(t),
	}
	h.prov.Scripts["gate-backend"] = []providers.Step{
		ask(t, "q1", "@"+arch+" 점수 API 경로는 /score로 하나요?"),
		file(t, "go.mod", "module game\n\ngo 1.21\n"),
		file(t, "api/score.go", "package api\n\nfunc Score(a, b int) int { return a + b }\n"), done(t),
	}
	h.prov.Scripts["gate-frontend"] = []providers.Step{file(t, "web/page.go", "package web\n\nfunc Title() string { return \"게임\" }\n"), done(t)}
	h.prov.Scripts["gate-backend-v2"] = []providers.Step{
		file(t, "api/score.go", "package api\n\nimport \"errors\"\n\nfunc Score(a, b int) (int, error) {\n\tif a < 0 || b < 0 {\n\t\treturn 0, errors.New(\"negative\")\n\t}\n\treturn a + b, nil\n}\n"), done(t),
	}
	h.prov.Scripts["agree"] = []providers.Step{
		step(t, `{"write": {"key": "opinion", "content": "{\"opinion\": \"REST /score 로 충분합니다\", \"agree\": true}"}}`), done(t),
	}
	h.meetingScenario = "agree"
	h.setModel(p, "a-planner", "gate-plan")
	h.setModel(p, "a-backend", "gate-backend")
	h.setModel(p, "a-frontend", "gate-frontend")

	runID := h.start(p)
	h.useScenario("deliver", "auto") // the planner's script is for the plan only
	h.approve(p.ID, runID, "approve")
	h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	d := h.detail(p.ID, runID)

	// Branch join: the unneeded legal review was skipped, the join went on.
	if d.Step("route").Status != engine.StSucceeded || d.Step("legal").Status != engine.StSkipped || d.Step("merge").Status != engine.StSucceeded {
		t.Fatalf("branch:\n%s", describe(d))
	}
	// Meeting: both developers agreed in one round; the architect decided.
	msgs := messagesOf(t, h, p.ID, runID)
	if got := proposals(msgs); len(got) != 2 || len(d.Step("design").Artifacts) != 1 {
		t.Fatalf("meeting proposals %v, design %+v", got, d.Step("design"))
	}
	// The backend's question went to the architect, not the person.
	if qs := byKind(msgs, "question"); len(qs) != 1 || qs[0].Recipient != arch || len(byKind(msgs, "answer")) != 1 {
		t.Fatalf("questions = %+v", qs)
	}
	// Parallel development in separate worktrees, integrated and built.
	be, fe := d.Step("backend").Workspace, d.Step("frontend").Workspace
	if be == nil || fe == nil || be.Path == fe.Path {
		t.Fatalf("workspaces %+v %+v", be, fe)
	}
	if got := paths(codeChangeOf(t, h, p.ID, d.Step("integrate"))); got["api/score.go"] == "" || got["web/page.go"] == "" || got["go.mod"] == "" {
		t.Fatalf("integrated = %v", got)
	}

	// Revision round: the review sends only the backend back.
	h.useScenario("backend", "gate-backend-v2")
	h.review(p.ID, runID, "review", engine.ReviewChangesRequested, "음수 점수를 거부하세요", "backend")
	h.waitStep(p.ID, runID, "review", engine.StWaitingHuman)
	d = h.detail(p.ID, runID)
	if d.Step("backend").Round != 1 || d.Step("integrate").Round != 1 || d.Step("frontend").Round != 0 {
		t.Fatalf("rounds:\n%s", describe(d))
	}
	if d.Step("backend").Workspace.Start != be.Commit {
		t.Fatal("the revision did not continue from the backend's previous commit")
	}
	if n := len(h.started("frontend")); n != 1 {
		t.Fatalf("frontend ran %d times", n)
	}
	if !strings.Contains(h.started("backend")[len(h.started("backend"))-1].Instructions, "음수 점수를 거부하세요") {
		t.Fatal("review comment did not reach the backend")
	}
	if cc := codeChangeOf(t, h, p.ID, d.Step("integrate")); !strings.Contains(cc.Patch, "errors.New") || paths(cc)["web/page.go"] == "" {
		t.Fatalf("integration after revision = %s", cc.Patch)
	}

	h.review(p.ID, runID, "review", engine.ReviewPass, "확인했습니다")
	d = h.waitRun(p.ID, runID, engine.RunSucceeded)
	if k := kinds(messagesOf(t, h, p.ID, runID)); k["review_request"] != 2 || k["decision"] != 1 || k["handoff"] == 0 {
		t.Fatalf("messages = %v", k)
	}
	if head, _ := g.Head(h.ctx, d.Repo.Path); head != d.Repo.Base {
		t.Fatal("service repository HEAD moved")
	}
	h.waitFor(p.ID, runID, "worktrees removed", func(engine.RunDetail) bool {
		_, err := os.Stat(fe.Path)
		return os.IsNotExist(err)
	})
}
