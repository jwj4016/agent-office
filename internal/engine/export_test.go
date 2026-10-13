package engine_test

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-office/internal/engine"
	"agent-office/internal/export"
	"agent-office/internal/secrets"
	"agent-office/internal/testenv"
)

// T20: a finished service exports with its records and results but
// without keys, key references, sessions or executable paths; a stored
// key that leaked into content is masked.
func TestExportExcludesKeys(t *testing.T) {
	dataDir := t.TempDir()
	h := newHarness(t, func(c *engine.Config) { c.DataDir = dataDir })
	p := testenv.ServiceDev(t, h.db, "게임")
	runID := h.start(p)
	h.approve(p.ID, runID, "approve")
	h.review(p.ID, runID, "review", engine.ReviewPass, "좋습니다")
	run := h.waitRun(p.ID, runID, engine.RunSucceeded)

	const key = "sk-ant-api03-THISISATESTKEY0123456789"
	store := secrets.NewMemory()
	store.Set("secret://abc123", key)
	h.db.Write(h.ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec(`UPDATE provider_connections SET secret_ref = 'secret://abc123', executable_path = '/opt/secret/codex' WHERE id = ?`, p.Connection.ID); err != nil {
			return err
		}
		// Someone pasted the key into the service instructions.
		if _, err := tx.Exec(`UPDATE projects SET instructions = ? WHERE id = ?`, "키는 "+key+" 입니다. 다른 키 sk-proj-abcdefghijklmnopqrstuvwx", p.ID); err != nil {
			return err
		}
		_, err := tx.Exec(`UPDATE step_attempts SET provider_session = 'thread-123' WHERE project_id = ?`, p.ID)
		return err
	})
	// One result file went missing on disk.
	lost := run.Step("deliver").Artifacts[0].Path
	os.Remove(filepath.Join(dataDir, filepath.FromSlash(lost)))

	var buf bytes.Buffer
	res, err := export.Project(h.ctx, export.Options{DB: h.db, DataDir: dataDir, Secrets: store, AppVersion: "test"}, p.ID, &buf)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	var all strings.Builder
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
		all.Write(b)
	}
	for _, banned := range []string{key, "sk-proj-abcdefghijklmnopqrstuvwx", "secret://abc123", "/opt/secret/codex", "thread-123"} {
		if strings.Contains(all.String(), banned) {
			i := strings.Index(all.String(), banned)
			t.Fatalf("export contains %q: …%s…", banned, all.String()[max(0, i-200):i+20])
		}
	}
	var conns []map[string]any
	json.Unmarshal([]byte(files["data/provider_connections.json"]), &conns)
	if len(conns) != 1 || len(conns[0]) != 3 || conns[0]["name"] == nil || conns[0]["provider"] != "test" {
		t.Fatalf("connections = %v (only id, name and provider)", conns)
	}
	if !strings.Contains(files["data/projects.json"], export.Mask) || res.Masked < 2 {
		t.Fatalf("key not masked (masked=%d): %s", res.Masked, files["data/projects.json"])
	}
	for _, name := range []string{"manifest.json", "README.txt", "data/roles.json", "data/assignments.json", "data/workflow_versions.json",
		"data/runs.json", "data/step_attempts.json", "data/messages.json", "data/approvals.json", "data/execution_events.json", "data/provider_connections.json"} {
		if _, ok := files[name]; !ok {
			t.Fatalf("missing %s", name)
		}
	}
	// Result files come along, except the one that was lost.
	var want int
	for _, s := range run.Steps {
		want += len(s.Artifacts)
	}
	if res.Artifacts != want-1 || len(res.Missing) != 1 || res.Missing[0] != lost {
		t.Fatalf("artifacts = %d of %d, missing %v", res.Artifacts, want, res.Missing)
	}
	plan := run.Step("plan").Artifacts[0]
	if _, ok := files["files/"+plan.Path]; !ok {
		t.Fatalf("plan result not exported (%s)", plan.Path)
	}
	var manifest map[string]any
	json.Unmarshal([]byte(files["manifest.json"]), &manifest)
	if manifest["format"] != export.Format || manifest["projectName"] != "게임" {
		t.Fatalf("manifest = %v", manifest)
	}
	// Another project's rows never leak in.
	other := testenv.ServiceDev(t, h.db, "부동산")
	buf.Reset()
	export.Project(h.ctx, export.Options{DB: h.db, DataDir: dataDir}, other.ID, &buf)
	if bytes.Contains(buf.Bytes(), []byte(p.ID)) {
		t.Fatal("other project's export mentions the first project")
	}
}
