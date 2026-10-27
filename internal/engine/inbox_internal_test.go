package engine

import (
	"encoding/json"
	"path/filepath"
	"testing"
)

// A tool approval is read for the person: the tool, the file or command,
// and which input result the file is.
func TestParseToolRequest(t *testing.T) {
	for _, c := range []struct {
		action, detail string
		want           ToolRequest
	}{
		{"Read", `Read {"file_path":"/d/projects/p/artifacts/r/a/prd_doc-x.md"}`, ToolRequest{Name: "Read", Path: "/d/projects/p/artifacts/r/a/prd_doc-x.md"}},
		{"Bash", `Bash {"command":"npm test","description":"테스트"}`, ToolRequest{Name: "Bash", Command: "npm test"}},
		{"WebFetch", `WebFetch {"url":"https://example.com","prompt":"요약"}`, ToolRequest{Name: "WebFetch", Command: "https://example.com"}},
		{"command", "go test ./...", ToolRequest{Name: "command", Command: "go test ./..."}},
		{"file_change", "파일을 고칩니다", ToolRequest{Name: "file_change"}},
		{"Read", "Claude가 파일을 읽으려 합니다", ToolRequest{Name: "Read"}}, // a title, not JSON
		{"Read", `Read {"file_path":`, ToolRequest{Name: "Read"}},  // cut at 500 characters
	} {
		if got := parseToolRequest(c.action, c.detail); got != c.want {
			t.Errorf("%s %q = %+v, want %+v", c.action, c.detail, got, c.want)
		}
	}

	dataDir := filepath.Join(t.TempDir(), "data")
	inputs, _ := json.Marshal([]ManifestEntry{
		{Name: "PRD", ArtifactID: "art-1", Path: "projects/p/artifacts/r/a/prd_doc-x.md"},
		{Name: "출처 목록", Path: ""}, // optional input that was not made
	})
	if got := inputNamed(dataDir, inputs, filepath.Join(dataDir, "projects", "p", "artifacts", "r", "a", "prd_doc-x.md")); got != "PRD" {
		t.Fatalf("input name = %q", got)
	}
	if got := inputNamed(dataDir, inputs, "/elsewhere/prd_doc-x.md"); got != "" {
		t.Fatalf("a file outside the inputs was named %q", got)
	}
}
