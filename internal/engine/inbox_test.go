package engine_test

import (
	"strings"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/testenv"
)

// T07 at the engine level: two services run side by side; inbox items,
// dashboards and artifacts stay with their own project.
func TestInboxAndDashboardAcrossServices(t *testing.T) {
	h := newHarness(t)
	game := testenv.ServiceDev(t, h.db, "게임")
	estate := testenv.ServiceDev(t, h.db, "부동산")
	gameRun, estateRun := h.start(game), h.start(estate)
	h.waitStep(game.ID, gameRun, "approve", engine.StWaitingApproval)
	h.waitStep(estate.ID, estateRun, "approve", engine.StWaitingApproval)

	items, err := h.eng.Inbox(h.ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("inbox = %+v", items)
	}
	byProject := map[string]engine.InboxItem{}
	for _, it := range items {
		byProject[it.ProjectName] = it
		if it.Kind != engine.InboxApproval || it.ApprovalID == "" || len(it.ReworkTargets) != 1 {
			t.Fatalf("item = %+v", it)
		}
	}
	if byProject["게임"].RunID != gameRun || byProject["부동산"].RunID != estateRun {
		t.Fatal("inbox items attributed to the wrong service")
	}

	dash, err := h.eng.Dashboard(h.ctx, false)
	if err != nil || len(dash) != 2 {
		t.Fatalf("dashboard = %+v, %v", dash, err)
	}
	for _, s := range dash {
		if s.Inbox != 1 || s.WaitingRuns != 1 || s.LastRun == nil || s.LastRun.Title != "서비스 개발" {
			t.Fatalf("summary = %+v", s)
		}
	}

	// An artifact is readable only through its own project.
	plan := h.detail(game.ID, gameRun).Step("plan").Artifacts[0]
	c, err := h.eng.ReadArtifact(h.ctx, game.ID, plan.ID)
	if err != nil || !c.HashOK || c.Content == "" {
		t.Fatalf("read = %+v, %v", c, err)
	}
	if _, err := h.eng.ReadArtifact(h.ctx, estate.ID, plan.ID); err == nil {
		t.Fatal("read another service's artifact")
	}
	// Each run pinned only its own project's plan.
	gameIn, estateIn := h.detail(game.ID, gameRun).Step("approve").Inputs, h.detail(estate.ID, estateRun).Step("approve").Inputs
	estatePlan := h.detail(estate.ID, estateRun).Step("plan").Artifacts[0]
	if string(gameIn) == string(estateIn) || !containsStr(string(estateIn), estatePlan.ID) {
		t.Fatal("inputs crossed services")
	}
}

func containsStr(s, sub string) bool { return strings.Contains(s, sub) }
