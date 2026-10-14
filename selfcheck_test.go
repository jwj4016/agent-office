package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// The self-check runs the whole app in a Korean, space-containing data
// folder and reports each part separately (T21).
func TestSelfCheckRunsFlowCancelAndRestart(t *testing.T) {
	keyring.MockInit()
	dir := filepath.Join(t.TempDir(), "에이전트 오피스 데이터")
	rep := runSelfCheck(context.Background(), checkOptions{DataDir: dir, Wait: e2eWait})
	got := map[string]CheckItem{}
	for _, c := range rep.Checks {
		got[c.ID] = c
	}
	failed := false
	for _, c := range rep.Checks {
		failed = failed || c.Status == CheckFailed
	}
	if rep.OK == failed {
		t.Fatalf("ok=%v does not match the checks: %+v", rep.OK, rep.Checks)
	}
	for _, id := range []string{"data", "flow", "cancel", "restart"} {
		if got[id].Status != CheckOK {
			t.Fatalf("%s: %+v", id, got[id])
		}
	}
	// Bridge, Codex and Git depend on this machine; they must be reported, not crash.
	for _, id := range []string{"secrets", "bridge", "codex", "git"} {
		if got[id].Status == "" {
			t.Fatalf("%s not reported", id)
		}
	}
	if !strings.Contains(got["flow"].Detail, "작업 폴더 (진단)") {
		t.Fatalf("flow did not use the Korean work folder: %s", got["flow"].Detail)
	}
	if os.Getenv("AGENT_OFFICE_DATA_DIR") != "" {
		t.Fatal("self-check leaked AGENT_OFFICE_DATA_DIR")
	}
	if _, err := os.Stat(filepath.Join(dir, "agent-office.db")); err != nil {
		t.Fatal("an explicit data folder must be kept")
	}
}

func TestSelfCheckMainWritesReportAndRemovesTempFolder(t *testing.T) {
	keyring.MockInit()
	t.Setenv("TMPDIR", t.TempDir()) // os.MkdirTemp honours it on Unix
	out := filepath.Join(t.TempDir(), "결과 보고.json")
	var buf bytes.Buffer
	code := selfCheckMain([]string{"-out", out}, &buf)
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var rep CheckReport
	if err := json.Unmarshal(b, &rep); err != nil {
		t.Fatal(err)
	}
	if (code == 0) != rep.OK {
		t.Fatalf("exit %d but ok=%v:\n%s", code, rep.OK, buf.String())
	}
	if !strings.Contains(rep.DataDir, "에이전트 오피스 진단 ") {
		t.Fatalf("temporary folder should have a Korean name with spaces: %q", rep.DataDir)
	}
	if _, err := os.Stat(rep.DataDir); !os.IsNotExist(err) {
		t.Fatalf("temporary data folder left behind: %v", err)
	}
	if !strings.Contains(buf.String(), "[ok] 흐름 실행") {
		t.Fatalf("text report:\n%s", buf.String())
	}
}

func TestSelfCheckReportsMissingDataFolder(t *testing.T) {
	file := filepath.Join(t.TempDir(), "파일")
	os.WriteFile(file, nil, 0o600)
	rep := runSelfCheck(context.Background(), checkOptions{DataDir: filepath.Join(file, "하위"), Wait: time.Second})
	if rep.OK || rep.Checks[0].ID != "data" || rep.Checks[0].Status != CheckFailed {
		t.Fatalf("%+v", rep)
	}
}

func TestSelfCheckRefusesFolderWithData(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "agent-office.db"), []byte("내 데이터"), 0o600)
	rep := runSelfCheck(context.Background(), checkOptions{DataDir: dir, Wait: time.Second})
	if rep.OK || rep.Checks[0].Status != CheckFailed || !strings.Contains(rep.Checks[0].Detail, "빈 폴더") {
		t.Fatalf("%+v", rep.Checks[0])
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "agent-office.db")); string(b) != "내 데이터" {
		t.Fatal("existing data was touched")
	}
	for _, c := range rep.Checks {
		if c.ID == "flow" {
			t.Fatal("flow ran in a folder with existing data")
		}
	}
}
