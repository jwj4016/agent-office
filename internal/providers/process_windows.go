//go:build windows

package providers

import (
	"os/exec"
	"strconv"
)

func configureProcAttr(cmd *exec.Cmd) {
	cmd.Cancel = func() error { killProc(cmd); return nil }
}

// killProc terminates the whole process tree. Windows has no process
// groups like Unix; taskkill /T walks the child tree.
// TODO(M6): switch to a Job Object so orphaned grandchildren are covered.
func killProc(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}
