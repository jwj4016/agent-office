//go:build !windows

package main

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// revealInFolder opens the system file manager at a file: selected in
// Finder on macOS, its folder elsewhere (Linux file managers differ).
func revealInFolder(path string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-R", path).Start()
	}
	return exec.Command("xdg-open", filepath.Dir(path)).Start()
}
