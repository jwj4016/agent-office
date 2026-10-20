package connect

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindDiagramPluginInCheckout(t *testing.T) {
	t.Setenv("AGENT_OFFICE_DIAGRAM_PLUGIN", "")
	p, ok := FindDiagramPlugin()
	if !ok {
		t.Fatal("third_party/diagram-design not found from the checkout")
	}
	if !fileExists(filepath.Join(p, ".claude-plugin", "plugin.json")) || !fileExists(filepath.Join(p, DiagramSkillRel, "scripts", "self_check.py")) {
		t.Fatalf("not a plugin folder: %s", p)
	}
}

func TestFindDiagramPluginOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AGENT_OFFICE_DIAGRAM_PLUGIN", dir)
	if p, ok := FindDiagramPlugin(); ok || p != dir {
		t.Fatalf("empty override = %q, %v", p, ok)
	}
	skill := filepath.Join(dir, DiagramSkillRel)
	os.MkdirAll(skill, 0o700)
	os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("x"), 0o600)
	if p, ok := FindDiagramPlugin(); !ok || p != dir {
		t.Fatalf("override = %q, %v", p, ok)
	}
}

func TestFindPythonSkipsStubsAndOldVersions(t *testing.T) {
	versions := map[string][2]int{"/usr/bin/python3": {3, 9}, "/opt/py/python": {3, 12}, "/old/python3": {2, 7}}
	version := func(p string) (int, int, bool) {
		v, ok := versions[p]
		return v[0], v[1], ok
	}
	at := func(paths map[string]string) func(string) (string, bool) {
		return func(name string) (string, bool) { p, ok := paths[name]; return p, ok }
	}
	for _, tc := range []struct {
		name, goos string
		paths      map[string]string
		devTools   bool
		want       string
	}{
		{"mac stub without developer tools", "darwin", map[string]string{"python3": "/usr/bin/python3"}, false, ""},
		{"mac system python with developer tools", "darwin", map[string]string{"python3": "/usr/bin/python3"}, true, "/usr/bin/python3"},
		{"falls back to python", "darwin", map[string]string{"python3": "/usr/bin/python3", "python": "/opt/py/python"}, false, "/opt/py/python"},
		{"python 2 is not enough", "linux", map[string]string{"python3": "/old/python3"}, true, ""},
		{"windows store alias", "windows", map[string]string{"python3": `C:\Users\me\AppData\Local\Microsoft\WindowsApps\python3.exe`}, true, ""},
		{"none installed", "linux", map[string]string{}, true, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := findPython(tc.goos, at(tc.paths), func() bool { return tc.devTools }, version); got != tc.want {
				t.Fatalf("findPython = %q, want %q", got, tc.want)
			}
		})
	}
}
