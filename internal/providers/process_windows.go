//go:build windows

package providers

import (
	"os/exec"
	"strconv"
	"time"
)

func configureProcAttr(cmd *exec.Cmd) {
	cmd.Cancel = func() error { killProc(cmd); return nil }
}

// killProc terminates the whole process tree. Windows has no process
// groups like Unix; taskkill /T walks the child tree. taskkill itself can
// take seconds on a busy machine, so the main process is killed directly
// once that wait runs out (the tree walk still finishes in the
// background).
// TODO(M6): switch to a Job Object so orphaned grandchildren are covered.
func killProc(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	done := make(chan struct{})
	go func() {
		exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		cmd.Process.Kill()
	}
}
