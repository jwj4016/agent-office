package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-office/internal/domain"
)

// Review finding 3: a symlinked parent directory must not let an output
// (or an artifact read) resolve to a file outside the data dir.
func TestVerifyRejectsSymlinkedParent(t *testing.T) {
	data, outside := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(outside, "spec.md"), []byte("# 외부 파일"), 0o600)
	attempt := filepath.Join(data, "projects", "p", "runs", "r", "attempts", "a")
	os.MkdirAll(attempt, 0o700)
	if err := os.Symlink(outside, filepath.Join(attempt, "out")); err != nil {
		t.Skip("symlinks unavailable:", err)
	}
	e := New(Config{DataDir: data})
	o := domain.Output{Key: "spec", Type: domain.OutMarkdown}
	if _, err := e.verifyOne(o, outputPath(attempt, o)); err == nil || !strings.Contains(err.Error(), "경로") {
		t.Fatalf("symlinked parent accepted: %v", err)
	}

	// A plain file inside the data dir is still accepted, even when the
	// data dir itself is reached through a symlink (e.g. /tmp on macOS).
	plain := filepath.Join(data, "projects", "p", "runs", "r", "attempts", "b")
	os.MkdirAll(filepath.Join(plain, "out"), 0o700)
	os.WriteFile(outputPath(plain, o), []byte("# 정상"), 0o600)
	link := filepath.Join(t.TempDir(), "data-link")
	os.Symlink(data, link)
	viaLink := New(Config{DataDir: link})
	if _, err := viaLink.verifyOne(o, outputPath(strings.Replace(plain, data, link, 1), o)); err != nil {
		t.Fatalf("normal output rejected: %v", err)
	}
	if _, err := e.verifyOne(o, outputPath(plain, o)); err != nil {
		t.Fatalf("normal output rejected: %v", err)
	}
}
