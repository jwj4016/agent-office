package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"agent-office/internal/storage"
)

// Every setting key the UI uses must be allowed by the Go side, or the UI
// silently fails to remember it (the frontend tests use a fake API).
func TestFrontendSettingKeysAreAllowed(t *testing.T) {
	key := regexp.MustCompile(`'(ui\.[A-Za-z]+)'`)
	seen := 0
	filepath.WalkDir(filepath.Join("frontend", "src"), func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.Contains(path, ".test.") || !(strings.HasSuffix(path, ".ts") || strings.HasSuffix(path, ".tsx")) {
			return nil
		}
		data, _ := os.ReadFile(path)
		for _, m := range key.FindAllStringSubmatch(string(data), -1) {
			seen++
			if !storage.SettingKeys[m[1]] {
				t.Errorf("%s uses setting %q, which SetSetting rejects", path, m[1])
			}
		}
		return nil
	})
	if seen < 3 {
		t.Fatalf("found only %d setting keys; did the frontend move?", seen)
	}
}
