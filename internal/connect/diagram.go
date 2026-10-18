package connect

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// DiagramSkillRel is the skill folder inside the diagram-design plugin.
var DiagramSkillRel = filepath.Join("skills", "diagram-design")

// FindDiagramPlugin locates the bundled diagram-design plugin folder (the
// one holding .claude-plugin/plugin.json): an override, next to the app,
// or (development) third_party/diagram-design in the checkout.
func FindDiagramPlugin() (string, bool) {
	marker := filepath.Join(DiagramSkillRel, "SKILL.md")
	if p := os.Getenv("AGENT_OFFICE_DIAGRAM_PLUGIN"); p != "" {
		return p, fileExists(filepath.Join(p, marker))
	}
	found, ok := findBundled(filepath.Join("diagram-design", marker), filepath.Join("third_party", "diagram-design", marker))
	if !ok {
		return "", false
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(found))), true
}

// FindPython returns a Python 3.8+ interpreter for the diagram skill's
// self-check, or "" when there is none. Stubs that would open an install
// dialog instead of running are skipped.
func FindPython() string {
	return findPython(runtime.GOOS, FindExecutable, func() bool {
		return exec.Command("/usr/bin/xcode-select", "-p").Run() == nil
	}, pythonVersion)
}

func findPython(goos string, find func(string) (string, bool), devToolsInstalled func() bool, version func(string) (int, int, bool)) string {
	for _, name := range []string{"python3", "python"} {
		p, ok := find(name)
		if !ok {
			continue
		}
		// macOS: /usr/bin/python3 without the Command Line Tools only
		// offers to install them.
		if goos == "darwin" && p == "/usr/bin/python3" && !devToolsInstalled() {
			continue
		}
		// Windows: the App Execution Alias opens the Microsoft Store.
		if goos == "windows" && strings.Contains(strings.ToLower(p), `\microsoft\windowsapps\`) {
			continue
		}
		if major, minor, ok := version(p); ok && (major > 3 || major == 3 && minor >= 8) {
			return p
		}
	}
	return ""
}

var pythonVersionRe = regexp.MustCompile(`Python (\d+)\.(\d+)`)

func pythonVersion(exe string) (int, int, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := runQuiet(ctx, exe, "--version")
	if err != nil {
		return 0, 0, false
	}
	m := pythonVersionRe.FindStringSubmatch(out)
	if m == nil {
		return 0, 0, false
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	return major, minor, true
}
