//go:build !windows

package providers

import (
	"os/exec"
	"syscall"
)

// configureProcAttr puts the child in its own process group so cancel can
// stop every subprocess it spawned (shells, test runners).
func configureProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

func killProc(cmd *exec.Cmd) {
	if cmd.Process != nil {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
