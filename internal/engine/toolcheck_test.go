package engine_test

import (
	"encoding/json"
	"strings"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/providers"
	"agent-office/internal/testenv"
)

func bashRequest(t *testing.T, id, command string) providers.Step {
	t.Helper()
	input, _ := json.Marshal(map[string]string{"command": command})
	b, _ := json.Marshal(map[string]any{"request": "approval_request", "payload": map[string]any{
		"requestId": id, "action": "Bash", "detail": "Bash " + string(input), "input": json.RawMessage(input), "options": []string{"accept", "decline"}}})
	return step(t, string(b))
}

// A safe command is allowed by the app at once and recorded as such; a
// risky one waits for the person with the reason; "always allow" keeps a
// rule, approves what else waits under it, and lets the next one through.
func TestToolRequestsAreCheckedBeforeAsking(t *testing.T) {
	h := newHarness(t)
	p := testenv.ServiceDev(t, h.db, "게임")
	h.prov.Scripts["checked"] = []providers.Step{
		bashRequest(t, "r1", "npm test && git status"),
		bashRequest(t, "r2", "npm install"),
		bashRequest(t, "r3", "npm install --save-dev vitest"),
		bashRequest(t, "r4", "sudo rm -rf /var/log"),
		step(t, `{"write": {"key": "spec", "content": "# 기획서"}}`),
		done(t),
	}
	h.useScenario("plan", "checked")
	runID := h.start(p)

	// r1 went through on its own; r2 waits with its reason.
	s := h.waitStep(p.ID, runID, "plan", engine.StWaitingApproval)
	var decision, reason string
	h.db.Read().QueryRow(`SELECT COALESCE(decision, ''), reason FROM approvals WHERE run_id = ? AND request_key = 'r1'`, runID).Scan(&decision, &reason)
	if decision != engine.ApprovalApproved || !strings.Contains(reason, "자동 허용") || !strings.Contains(reason, "테스트 실행") {
		t.Fatalf("safe command: %q %q", decision, reason)
	}
	items, _ := h.eng.Inbox(h.ctx)
	var install *engine.InboxItem
	for i := range items {
		if items[i].Kind == engine.InboxToolApproval && items[i].AttemptID == s.AttemptID {
			install = &items[i]
		}
	}
	if install == nil || install.Tool == nil || len(install.Tool.Risks) != 1 || !strings.Contains(install.Tool.Risks[0], "패키지를 인터넷에서 내려받아") ||
		len(install.Tool.Rules) != 1 || install.Tool.Rules[0] != "npm install" {
		t.Fatalf("waiting request = %+v", install)
	}

	// Always allow npm install here: r2 goes on, r3 needs no one, r4 asks.
	if keys, err := h.eng.RememberToolRules(h.ctx, p.ID, install.ApprovalID); err != nil || len(keys) != 1 {
		t.Fatalf("remember: %v %v", keys, err)
	}
	if _, err := h.eng.DecideToolApproval(h.ctx, p.ID, install.ApprovalID, true); err != nil {
		t.Fatal(err)
	}
	h.waitFor(p.ID, runID, "sudo waits", func(engine.RunDetail) bool {
		var n int
		h.db.Read().QueryRow(`SELECT COUNT(*) FROM approvals WHERE run_id = ? AND request_key = 'r4' AND decision IS NULL`, runID).Scan(&n)
		return n == 1
	})
	h.db.Read().QueryRow(`SELECT COALESCE(decision, ''), reason FROM approvals WHERE run_id = ? AND request_key = 'r3'`, runID).Scan(&decision, &reason)
	if decision != engine.ApprovalApproved || !strings.Contains(reason, "서비스 규칙: npm install") {
		t.Fatalf("rule-covered command: %q %q", decision, reason)
	}
	items, _ = h.eng.Inbox(h.ctx)
	for _, it := range items {
		if it.Kind == engine.InboxToolApproval && it.Tool != nil && strings.Contains(strings.Join(it.Tool.Risks, " "), "관리자 권한") {
			if len(it.Tool.Rules) != 0 {
				t.Fatalf("an administrator command offered a rule: %+v", it.Tool)
			}
			if _, err := h.eng.RememberToolRules(h.ctx, p.ID, it.ApprovalID); err == nil {
				t.Fatal("an administrator command was made a rule")
			}
			if _, err := h.eng.DecideToolApproval(h.ctx, p.ID, it.ApprovalID, false); err != nil {
				t.Fatal(err)
			}
		}
	}
	h.waitStep(p.ID, runID, "plan", engine.StSucceeded)
}
