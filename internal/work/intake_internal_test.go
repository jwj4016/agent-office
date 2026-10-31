package work

import (
	"os"
	"path/filepath"
	"testing"
)

// A desk's folder suggestion is kept only inside the person's home folder.
func TestSuggestedFolder(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip(err)
	}
	for in, want := range map[string]string{
		"~/dev/workspace/villanote":                   filepath.Join(home, "dev", "workspace", "villanote"),
		filepath.Join(home, "dev", "x") + "/../villa": filepath.Join(home, "dev", "villa"),
		"":                     "",
		"~":                    "",
		"dev/villa":            "",
		"/etc/villa":           "",
		"~/../other/villanote": "",
	} {
		if got := suggestedFolder(in); got != want {
			t.Errorf("suggestedFolder(%q) = %q, want %q", in, got, want)
		}
	}
}
