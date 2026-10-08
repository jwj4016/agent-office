package engine_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
	"agent-office/internal/workspace"
)

// TestRealM4Gate runs the M4 gate workflow with real Codex and Claude
// (uses the account's quota; off by default):
//
//	AGENT_OFFICE_REAL_GATE=<control dir> AGENT_OFFICE_CODEX_IT=<codex executable> \
//	  go test ./internal/engine/ -run TestRealM4Gate -v -timeout 120m
//
// Nothing a person decides is automated. Every open item (approval,
// review, tool permission, question) is written to <control>/pending/<id>.json;
// the person answers by creating <control>/decisions/<id>.txt whose first
// line is the decision:
//
//	approval:       approve | reject <target> <reason…>
//	review:         pass <comment…> | changes <target> <comment…>
//	tool_approval:  accept | decline
//	question:       the answer text
func TestRealM4Gate(t *testing.T) {
	ctrl := os.Getenv("AGENT_OFFICE_REAL_GATE")
	codex := os.Getenv("AGENT_OFFICE_CODEX_IT")
	if ctrl == "" || codex == "" {
		t.Skip("set AGENT_OFFICE_REAL_GATE and AGENT_OFFICE_CODEX_IT to run the real gate")
	}
	g := workspace.FindGit()
	node, _ := exec.LookPath("node")
	script, _ := filepath.Abs(filepath.Join(testenv.RepoRoot(), "runners", "claude", "dist", "main.js"))
	if g == nil || node == "" {
		t.Skip("git and node are required")
	}
	ctx := context.Background()
	for _, d := range []string{"data", "pending", "decisions"} {
		os.MkdirAll(filepath.Join(ctrl, d), 0o700)
	}
	db, err := storage.Open(ctx, filepath.Join(ctrl, "data", "agent-office.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// The service repository: a Go module the developers extend.
	repo := filepath.Join(ctrl, "점수 게임")
	if _, err := g.Init(ctx, repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(repo, "go.mod")); os.IsNotExist(err) {
		os.WriteFile(filepath.Join(repo, "go.mod"), []byte("module game\n\ngo 1.21\n"), 0o600)
		os.WriteFile(filepath.Join(repo, "README.md"), []byte("# 점수 게임\n\nAgent Office M4 게이트용 저장소\n"), 0o600)
		if _, err := g.CommitAll(ctx, repo, "Go 모듈 시작"); err != nil {
			t.Fatal(err)
		}
	}

	verified := json.RawMessage(`{"call":true}`)
	codexConn, _ := db.SaveConnection(ctx, domain.ProviderConnection{Name: "내 Codex", Provider: "codex", ExecutablePath: codex, VerifiedCapabilities: verified})
	claudeConn, _ := db.SaveConnection(ctx, domain.ProviderConnection{Name: "내 Claude", Provider: "claude", VerifiedCapabilities: verified})
	draft := testenv.EditFixture(t, "workflows/m4-gate.json", func(n map[string]map[string]any) {
		n["plan"]["instructions"] = "작은 Go 예제 '점수 게임'의 요구사항을 spec(Markdown)에 짧게 정리한다: 맞힌 수(hits)와 놓친 수(misses)로 점수를 계산하는 api 패키지와, 점수를 HTML 조각으로 보여 주는 web 패키지. scope(JSON)에는 {\"legal\": {\"needed\": <true|false>}}로 법률 검토 필요 여부를 적는다. 개인정보·결제가 없는 학습용 예제다."
		n["design"]["instructions"] = "저장소는 Go 모듈 game(go.mod 있음). 백엔드는 api 패키지에 func Score(hits, misses int) int, 프론트엔드는 web 패키지에 func Render(score int) string을 만든다. 파일 위치, 함수 동작(점수 규칙), 각자의 테스트 방법을 contract에 정한다. 두 개발자가 서로의 폴더와 go.mod를 건드리지 않게 한다."
		n["backend"]["instructions"] = "API 계약대로 api/score.go와 api/score_test.go만 작성한다. web 폴더와 go.mod는 건드리지 않는다."
		n["backend"]["completion"] = map[string]any{"commands": []any{map[string]any{"executable": "go", "args": []any{"test", "./api/..."}}}}
		n["frontend"]["instructions"] = "API 계약대로 web/render.go와 web/render_test.go만 작성한다. api 폴더와 go.mod는 건드리지 않는다."
		n["frontend"]["completion"] = map[string]any{"commands": []any{map[string]any{"executable": "go", "args": []any{"test", "./web/..."}}}}
		n["integrate"]["instructions"] = "백엔드·프론트엔드 변경이 합쳐진 작업 공간을 확인한다. cmd/game/main.go를 만들어 api.Score와 web.Render를 연결해 예시 점수를 출력하게 한다."
		n["integrate"]["completion"] = map[string]any{"commands": []any{
			map[string]any{"executable": "go", "args": []any{"build", "./..."}},
			map[string]any{"executable": "go", "args": []any{"vet", "./..."}},
			map[string]any{"executable": "go", "args": []any{"test", "./..."}},
		}}
		n["qa"]["instructions"] = "통합된 코드와 리뷰 결과를 읽고 계약대로 동작하는지 검토해 verification(JSON: summary, findings, tests)으로 보고한다. 코드는 고치지 않는다."
		n["deliver"]["instructions"] = "검증 결과를 바탕으로 무엇을 만들었는지 사용자에게 전달할 요약을 쓴다."
	})
	p := testenv.SeedDraft(t, db, "점수 게임", draft, map[string]string{"a-planner": "기획", "a-architect": "설계", "a-backend": "백엔드", "a-frontend": "프론트엔드", "a-qa": "QA"}, []string{"a-owner"})
	proj, _ := db.Project(ctx, p.ID)
	proj.WorkspacePath = repo
	proj.Goal = "작은 Go 예제 '점수 게임'을 만든다"
	db.UpdateProject(ctx, proj)
	use := map[string]domain.ProviderConnection{"a-planner": claudeConn, "a-architect": claudeConn, "a-frontend": claudeConn, "a-backend": codexConn, "a-qa": codexConn}
	list, _ := db.Assignments(ctx, p.ID)
	for fixture, conn := range use {
		for _, a := range list {
			if a.ID == p.Assignments[fixture] {
				a.ConnectionID, a.Model = conn.ID, ""
				if _, err := db.SaveAssignment(ctx, a); err != nil {
					t.Fatal(err)
				}
			}
		}
	}

	eng := engine.New(engine.Config{
		DB: db, DataDir: filepath.Join(ctrl, "data"), Git: g, Logf: t.Logf,
		Providers: func(c domain.ProviderConnection) (providers.Provider, error) {
			if c.Provider == "codex" {
				return &providers.Codex{Executable: c.ExecutablePath, Version: "m4-gate"}, nil
			}
			return &providers.ClaudeBridge{Node: node, Script: script}, nil
		},
	})
	ectx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() { eng.Run(ectx); close(done) }()
	defer func() { stop(); <-done }()

	w, _ := db.Workflow(ctx, p.ID, p.WorkflowID)
	v, res, err := db.ConfirmVersion(ctx, p.ID, w.ID, w.Revision)
	if err != nil {
		t.Fatalf("confirm: %v %v", err, res.Issues)
	}
	runID, err := eng.StartRun(ctx, p.ID, v.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("RUN %s project %s repo %s", runID, p.ID, repo)

	seen := map[string]string{}
	decided := map[string]bool{}
	deadline := time.Now().Add(110 * time.Minute)
	for time.Now().Before(deadline) {
		d, err := eng.RunDetail(ctx, p.ID, runID)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range d.Steps {
			state := fmt.Sprintf("%s g%d a%d r%d", s.Status, s.Generation, s.Attempt, s.Round)
			if s.Error != "" {
				state += " (" + s.Error + ")"
			}
			if seen[s.ID] != state {
				seen[s.ID] = state
				t.Logf("STEP %-10s %s", s.ID, state)
			}
		}
		if d.Status == engine.RunSucceeded || d.Status == engine.RunCancelled {
			t.Logf("RUN %s", d.Status)
			return
		}
		items, _ := eng.Inbox(ctx)
		sort.Slice(items, func(i, j int) bool { return items[i].Since < items[j].Since })
		for _, it := range items {
			key := it.Kind + "-" + firstOf(it.ApprovalID, it.MessageID, it.AttemptID)
			if decided[key] {
				continue
			}
			pending := filepath.Join(ctrl, "pending", key+".json")
			if _, err := os.Stat(pending); os.IsNotExist(err) {
				b, _ := json.MarshalIndent(it, "", "  ")
				os.WriteFile(pending, b, 0o600)
				t.Logf("PENDING %s %s: %s", key, it.StepTitle, clipText(it.Detail, 300))
			}
			raw, err := os.ReadFile(filepath.Join(ctrl, "decisions", key+".txt"))
			if err != nil {
				continue
			}
			decided[key] = true
			text := strings.TrimSpace(string(raw))
			verb, rest, _ := strings.Cut(text, " ")
			var out engine.Outcome
			switch it.Kind {
			case engine.InboxApproval:
				if verb == "approve" {
					out, err = eng.DecideApproval(ctx, p.ID, it.ApprovalID, it.Generation, engine.ApprovalApproved, "", nil)
				} else {
					target, reason, _ := strings.Cut(rest, " ")
					out, err = eng.DecideApproval(ctx, p.ID, it.ApprovalID, it.Generation, engine.ApprovalRejected, reason, []string{target})
				}
			case engine.InboxReview:
				if verb == "pass" {
					out, err = eng.SubmitReview(ctx, p.ID, it.AttemptID, it.Generation, engine.ReviewPass, rest, nil, nil)
				} else {
					target, comment, _ := strings.Cut(rest, " ")
					out, err = eng.SubmitReview(ctx, p.ID, it.AttemptID, it.Generation, engine.ReviewChangesRequested, comment, []string{target}, nil)
				}
			case engine.InboxToolApproval:
				out, err = eng.DecideToolApproval(ctx, p.ID, it.ApprovalID, verb == "accept")
			case engine.InboxQuestion:
				out, err = eng.AnswerQuestion(ctx, p.ID, it.MessageID, text)
			default:
				err = fmt.Errorf("no handler for %s", it.Kind)
			}
			t.Logf("DECIDED %s %q -> %+v err=%v", key, clipText(text, 120), out, err)
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatal("the gate run did not finish in time")
}

func firstOf(xs ...string) string {
	for _, x := range xs {
		if x != "" {
			return x
		}
	}
	return ""
}

func clipText(s string, n int) string {
	r := []rune(strings.ReplaceAll(s, "\n", " "))
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return string(r)
}
