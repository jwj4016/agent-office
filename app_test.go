package main

import (
	"context"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestPingReturnsAndEmits(t *testing.T) {
	var gotName string
	var gotData []interface{}
	a := &App{ctx: context.Background(), emit: func(_ context.Context, name string, data ...interface{}) {
		gotName, gotData = name, data
	}}

	first := a.Ping("안녕")
	second := a.Ping("again")

	if first.Message != "안녕" || first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("unexpected results: %+v %+v", first, second)
	}
	if gotName != EventPing || len(gotData) != 1 || gotData[0].(PingResult) != second {
		t.Fatalf("event not emitted correctly: %q %+v", gotName, gotData)
	}
}

// startTemp starts an App against a temp data dir and a mock keychain.
func startTemp(t *testing.T, dir string) *App {
	t.Helper()
	keyring.MockInit()
	t.Setenv("AGENT_OFFICE_DATA_DIR", dir)
	a := NewApp()
	a.startup(context.Background())
	if st := a.SystemStatus(); st.Error != "" {
		t.Fatalf("startup failed: %s", st.Error)
	}
	return a
}

func TestSettingsSurviveRestart(t *testing.T) {
	dir := t.TempDir()

	a := startTemp(t, dir)
	if err := a.SetSetting("ui.reducedMotion", "true"); err != nil {
		t.Fatal(err)
	}
	if err := a.SetSetting("not.allowed", "x"); err == nil {
		t.Fatal("unknown key accepted")
	}
	a.shutdown(context.Background())

	b := startTemp(t, dir)
	defer b.shutdown(context.Background())
	got, err := b.GetSettings()
	if err != nil || got["ui.reducedMotion"] != "true" {
		t.Fatalf("settings after restart = %v, %v", got, err)
	}
	st := b.SystemStatus()
	if st.SchemaVersion < 1 || st.DataDir != dir || !st.SecretsPersistent {
		t.Fatalf("unexpected status: %+v", st)
	}
}
