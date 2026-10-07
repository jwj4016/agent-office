package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"agent-office/internal/design"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
)

func TestDesignBindings(t *testing.T) {
	a := startApp(t)
	conn := need(a.EnsureTestConnection())
	data, err := os.ReadFile(testenv.RepoRoot() + "/tests/fixtures/design/game-launch.json")
	if err != nil {
		t.Fatal(err)
	}
	proposal := strings.NewReplacer("CONN_AI", conn.ID, "CONN_CODE", conn.ID).Replace(string(data))
	a.testProvider.Scripts["designer"] = []providers.Step{
		{Write: &providers.WriteStep{Key: "design", Content: proposal}},
		{Complete: &providers.CompletedPayload{Status: providers.StatusSucceeded}},
	}
	v := need(a.StartDesign(DesignInput{Goal: "게임을 출시하고 싶어. 코드 리뷰는 내가 할게.", HumanTasks: []string{"코드 리뷰"},
		Mode: "review", ConnectionID: conn.ID, Model: "designer"}))
	deadline := time.Now().Add(5 * time.Second)
	for v.Status == design.StatusDrafting && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
		v = need(a.GetDesign(v.ID))
	}
	if v.Status != design.StatusReady || v.Result == nil || !v.Result.CanApply || len(v.Request.HumanTasks) != 1 {
		t.Fatalf("design = %+v", v)
	}
	r := need(a.ApplyDesign(v.ID, false))
	if r.Applied == nil || !r.Applied.NewProject {
		t.Fatalf("apply = %+v", r)
	}
	if list := need(a.Dashboard(false)); len(list) != 1 || list[0].Project.Name != "작은 웹 게임" {
		t.Fatalf("dashboard = %+v", list)
	}
	if err := a.DiscardDesign(v.ID); err == nil {
		t.Fatal("applied design discarded")
	}
	if _, err := a.ApplyDesign(v.ID, false); err == nil || !strings.Contains(err.Error(), "이미 적용") {
		t.Fatalf("second apply: %v", err)
	}
	if err := a.SetAutoPolicy(r.Applied.ProjectID, []string{conn.ID}); err != nil {
		t.Fatal(err)
	}
	if err := a.SetAutoPolicy("prj-nope", nil); err == nil {
		t.Fatal("auto policy set on unknown project")
	}
	if _, err := a.StartDesign(DesignInput{Goal: " ", Mode: "review", ConnectionID: conn.ID}); err == nil {
		t.Fatal("empty goal accepted")
	}
}
