//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// revealInFolder opens Explorer with the file selected. Explorer wants
// the path quoted after "/select,", which Go's own argument quoting does
// not produce, so the command line is written out.
func revealInFolder(path string) error {
	cmd := exec.Command("explorer")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `explorer /select,"` + path + `"`}
	return cmd.Start() // Explorer's exit code means nothing; do not wait
}
