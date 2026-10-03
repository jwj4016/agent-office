package main

import (
	"context"
	"strings"
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

// Review finding 4: a second app on the same data dir reports the lock
// and never starts an engine that could disturb the first one.
func TestSecondAppInstanceRefused(t *testing.T) {
	dir := t.TempDir()
	first := startTemp(t, dir)
	defer first.shutdown(context.Background())

	t.Setenv("AGENT_OFFICE_DATA_DIR", dir)
	second := NewApp()
	second.emit = func(context.Context, string, ...interface{}) {}
	second.startup(context.Background())
	defer second.shutdown(context.Background())
	if st := second.SystemStatus(); !strings.Contains(st.Error, "사용 중") {
		t.Fatalf("second instance status = %+v", st)
	}
	if second.eng != nil {
		t.Fatal("second instance started an engine")
	}
	if _, err := second.Dashboard(false); err == nil {
		t.Fatal("second instance served data")
	}
}
