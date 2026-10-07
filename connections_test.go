package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"agent-office/internal/connect"
	"agent-office/internal/domain"
)

// fakeClaudeAPI answers every Messages call with the given output fields.
func fakeClaudeAPI(t *testing.T, fields map[string]string) *httptest.Server {
	t.Helper()
	text, _ := json.Marshal(fields)
	body, _ := json.Marshal(map[string]any{
		"id": "msg", "type": "message", "role": "assistant", "model": "claude-opus-5-5",
		"content": []any{map[string]any{"type": "text", "text": string(text)}}, "stop_reason": "end_turn",
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 3},
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "sk-ant-secret-123" {
			w.WriteHeader(401)
			io.WriteString(w, `{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestConnectionLifecycle(t *testing.T) {
	a := startApp(t)
	srv := fakeClaudeAPI(t, map[string]string{"message": "ok", "reply": "pong"})
	c := need(a.SaveConnection(ConnectionInput{Name: "Claude API", Provider: connect.KindClaudeAPI, Settings: connect.Config{BaseURL: srv.URL}}))
	if c.Usable || c.HasKey {
		t.Fatalf("new connection = %+v", c)
	}
	// No key yet: the auth step fails and no call is attempted.
	r := need(a.TestConnectionCall(c.ID, ""))
	if r.Ready || r.Steps[len(r.Steps)-1].Status != connect.Skipped {
		t.Fatalf("call without key: %+v", r)
	}

	c = need(a.SetConnectionKey(c.ID, "  sk-ant-secret-123  "))
	if !c.HasKey || !strings.HasPrefix(c.SecretRef, "secret://") {
		t.Fatalf("after key: %+v", c)
	}
	if free := need(a.CheckConnection(c.ID)); !free.Passed() || free.Ready {
		t.Fatalf("free check: %+v", free)
	}
	r = need(a.TestConnectionCall(c.ID, ""))
	if !r.Ready {
		t.Fatalf("call: %+v", r)
	}
	views := need(a.ListConnections())
	raw, _ := json.Marshal(views)
	if strings.Contains(string(raw), "sk-ant-secret-123") {
		t.Fatal("API key leaked into the connection list")
	}
	var dbRaw string
	a.db.Read().QueryRow(`SELECT secret_ref || config || verified_capabilities FROM provider_connections WHERE id = ?`, c.ID).Scan(&dbRaw)
	if strings.Contains(dbRaw, "sk-ant-secret-123") {
		t.Fatal("API key stored in the database")
	}
	if !views[0].Usable {
		t.Fatalf("verified connection not usable: %+v", views[0])
	}

	// An AI assignment on this connection can now run; editing the
	// connection clears the verification again.
	p := need(a.CreateProject(ProjectInput{Name: "x", Mode: "review"}))
	role := need(a.SaveRole(domain.Role{Name: "기획"}))
	asg := need(a.SaveAssignment(domain.Assignment{ProjectID: p.ID, RoleID: role.ID, ActorKind: domain.ActorAI, DisplayName: "기획 AI", ConnectionID: c.ID}))
	_ = asg
	c = need(a.SaveConnection(ConnectionInput{ID: c.ID, Name: "Claude API 2", Provider: connect.KindClaudeAPI, Settings: connect.Config{BaseURL: srv.URL}}))
	if c.Usable || !c.HasKey {
		t.Fatalf("after edit: %+v", c)
	}
	c = need(a.ClearConnectionKey(c.ID))
	if c.HasKey || c.SecretRef != "" {
		t.Fatalf("after clear: %+v", c)
	}
}

func TestConnectionCallWrongKeyFails(t *testing.T) {
	a := startApp(t)
	srv := fakeClaudeAPI(t, map[string]string{"message": "ok", "reply": "pong"})
	c := need(a.SaveConnection(ConnectionInput{Name: "Claude API", Provider: connect.KindClaudeAPI, Settings: connect.Config{BaseURL: srv.URL}}))
	need(a.SetConnectionKey(c.ID, "sk-wrong"))
	r := need(a.TestConnectionCall(c.ID, ""))
	last := r.Steps[len(r.Steps)-1]
	if r.Ready || last.ID != "call" || last.Status != connect.Failed {
		t.Fatalf("wrong key: %+v", r)
	}
	if strings.Contains(last.Detail, "sk-wrong") {
		t.Fatal("key echoed in error detail")
	}
}
