package engine_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-office/internal/domain"
	"agent-office/internal/engine"
	"agent-office/internal/storage"
	"agent-office/internal/testenv"
)

// docSetup is a service whose marketing, development and finance
// departments each have an AI member, a work item and a saved plan.
type docSetup struct {
	p                testenv.Project
	mkt, dev, fin    domain.Department
	plan, devW, finW domain.WorkItem
	workspace        string
}

func newDocSetup(t *testing.T, h *harness) docSetup {
	t.Helper()
	ctx := h.ctx
	p := testenv.SeedDraft(t, h.db, "캠페인", []byte(`{"schemaVersion":1,"title":"빈 흐름","nodes":[]}`),
		map[string]string{"a-mkt": "마케터", "a-dev": "개발자", "a-fin": "재무 담당"}, nil)
	s := docSetup{p: p, workspace: t.TempDir()}
	proj, _ := h.db.Project(ctx, p.ID)
	proj.WorkspacePath = s.workspace
	if _, err := h.db.UpdateProject(ctx, proj); err != nil {
		t.Fatal(err)
	}
	dept := func(name, asg string) domain.Department {
		d, err := h.db.SaveDepartment(ctx, domain.Department{ProjectID: p.ID, Name: name, MemberIDs: []string{asg}, LeadAssignmentID: asg})
		if err != nil {
			t.Fatal(err)
		}
		return d
	}
	s.mkt, s.dev, s.fin = dept("마케팅팀", p.Assignments["a-mkt"]), dept("개발팀", p.Assignments["a-dev"]), dept("재무팀", p.Assignments["a-fin"])
	work := func(d domain.Department, asg, title string) domain.WorkItem {
		w, err := h.db.CreateWorkItem(ctx, domain.WorkItem{ProjectID: p.ID, DepartmentID: d.ID, Title: title})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.db.SaveWorkPlan(ctx, domain.WorkPlan{ProjectID: p.ID, WorkItemID: w.ID, AssignmentID: asg}); err != nil {
			t.Fatal(err)
		}
		return w
	}
	s.plan = work(s.mkt, p.Assignments["a-mkt"], "캠페인 기획안 작성")
	s.devW = work(s.dev, p.Assignments["a-dev"], "페이지 구현 검토")
	s.finW = work(s.fin, p.Assignments["a-fin"], "예산 검토")
	return s
}

func (s docSetup) write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(s.workspace, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func (s docSetup) runAndWait(t *testing.T, h *harness, w domain.WorkItem) (string, string) {
	t.Helper()
	runID, err := h.eng.StartWork(h.ctx, s.p.ID, w.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	h.waitRun(s.p.ID, runID, engine.RunSucceeded)
	reqs := h.started(domain.WorkStepID)
	last := reqs[len(reqs)-1]
	return runID, last.Instructions
}

// Stage 4 acceptance: while the campaign is still in progress, its plan
// draft (a run result) is shared; development and finance use the same
// frozen copy, and a second version does not change what an earlier run
// was given.
func TestSharedDraftIsUsedByOtherDepartments(t *testing.T) {
	h := newHarness(t)
	s := newDocSetup(t, h)
	ctx := h.ctx
	mktRun, _ := s.runAndWait(t, h, s.plan)
	art := h.detail(s.p.ID, mktRun).Step(domain.WorkStepID).Artifacts[0]
	v1, err := h.eng.ShareArtifact(ctx, s.p.ID, art.ID, storage.NewDocumentVersion{Title: "캠페인 기획안", WorkItemID: s.plan.ID, Categories: []string{"기획"}})
	if err != nil {
		t.Fatal(err)
	}
	if v1.Number != 1 || v1.SourceKind != "artifact" || v1.SourceArtifactID != art.ID {
		t.Fatalf("v1 = %+v", v1)
	}
	if w, _ := h.db.WorkItem(ctx, s.p.ID, s.plan.ID); domain.WorkClosed(w.Status) {
		t.Fatal("sharing closed the work")
	}
	content, err := h.eng.ReadDocumentVersion(ctx, s.p.ID, v1.ID)
	if err != nil || !content.HashOK || content.Content == "" {
		t.Fatalf("read v1: %+v %v", content, err)
	}
	for _, w := range []domain.WorkItem{s.devW, s.finW} {
		if err := h.db.LinkDocument(ctx, s.p.ID, w.ID, v1.ID); err != nil {
			t.Fatal(err)
		}
	}
	devRun, instructions := s.runAndWait(t, h, s.devW)
	if !strings.Contains(instructions, "## 참고 공유 자료") || !strings.Contains(instructions, "캠페인 기획안 1판 (초안)") || !strings.Contains(instructions, content.Content) {
		t.Fatalf("development was not given the draft:\n%s", instructions)
	}
	finRun, _ := s.runAndWait(t, h, s.finW)
	for _, run := range []string{devRun, finRun} {
		pins, _ := h.db.RunDocuments(ctx, s.p.ID, run)
		if len(pins) != 1 || pins[0].ID != v1.ID {
			t.Fatalf("run %s pinned %+v", run, pins)
		}
	}

	// Marketing revises the plan from its work folder.
	s.write(t, "plan.md", "# 캠페인 기획안\n\n2판: 예산 줄임")
	v2, err := h.eng.ShareWorkspaceFile(ctx, s.p.ID, "plan.md", storage.NewDocumentVersion{DocumentID: v1.DocumentID})
	if err != nil {
		t.Fatal(err)
	}
	if v2.Number != 2 || v2.SourcePath != "plan.md" {
		t.Fatalf("v2 = %+v", v2)
	}
	// The earlier runs keep version 1; the users are asked to check.
	if pins, _ := h.db.RunDocuments(ctx, s.p.ID, devRun); len(pins) != 1 || pins[0].ID != v1.ID {
		t.Fatalf("dev run now pins %+v", pins)
	}
	checks, _ := h.db.ImpactChecks(ctx, s.p.ID, "", "open")
	if len(checks) != 2 {
		t.Fatalf("impact checks = %+v", checks)
	}
	// Changing the work folder file later never touches the shared copy.
	s.write(t, "plan.md", "# 다른 내용")
	if again, _ := h.eng.ReadDocumentVersion(ctx, s.p.ID, v2.ID); again.Content != "# 캠페인 기획안\n\n2판: 예산 줄임" || !again.HashOK {
		t.Fatalf("v2 copy changed: %q", again.Content)
	}
}

// A run's draft is shared and the same work's next run continues from it.
func TestNextRunContinuesFromSharedDraft(t *testing.T) {
	h := newHarness(t)
	s := newDocSetup(t, h)
	first, _ := s.runAndWait(t, h, s.plan)
	art := h.detail(s.p.ID, first).Step(domain.WorkStepID).Artifacts[0]
	v1, err := h.eng.ShareArtifact(h.ctx, s.p.ID, art.ID, storage.NewDocumentVersion{Title: "기획안 초안", WorkItemID: s.plan.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := h.db.LinkDocument(h.ctx, s.p.ID, s.plan.ID, v1.ID); err != nil {
		t.Fatal(err)
	}
	_, instructions := s.runAndWait(t, h, s.plan)
	if !strings.Contains(instructions, "기획안 초안 1판") {
		t.Fatalf("second run did not get the draft:\n%s", instructions)
	}
	// Sharing the same result again is refused: nothing changed.
	if _, err := h.eng.ShareArtifact(h.ctx, s.p.ID, art.ID, storage.NewDocumentVersion{DocumentID: v1.DocumentID}); !errors.Is(err, storage.ErrInvalid) {
		t.Fatalf("same content shared twice: %v", err)
	}
}

func TestWorkspaceSharingStaysInsideTheFolder(t *testing.T) {
	h := newHarness(t)
	s := newDocSetup(t, h)
	nv := storage.NewDocumentVersion{Title: "자료", OwnerDepartmentID: s.mkt.ID}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	os.WriteFile(outside, []byte("secret"), 0o600)
	os.Mkdir(filepath.Join(s.workspace, "dir"), 0o700)
	for _, path := range []string{"../secret.txt", outside, "missing.md", "dir", ""} {
		if _, err := h.eng.ShareWorkspaceFile(h.ctx, s.p.ID, path, nv); !errors.Is(err, storage.ErrInvalid) {
			t.Fatalf("%q: %v", path, err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(s.workspace, "link.txt")); err == nil {
		if _, err := h.eng.ShareWorkspaceFile(h.ctx, s.p.ID, "link.txt", nv); !errors.Is(err, storage.ErrInvalid) {
			t.Fatalf("symlink: %v", err)
		}
	} else {
		t.Logf("symlink check skipped: %v", err)
	}
	if docs, _ := h.db.Documents(h.ctx, s.p.ID, storage.DocumentFilter{IncludeWithdrawn: true}); len(docs) != 0 {
		t.Fatalf("documents = %+v", docs)
	}
}
