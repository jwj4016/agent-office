//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
)

// Hide is a no-op outside Windows.
func Hide(cmd *exec.Cmd) {}

// Group is the child's process group (the child's pid).
type Group struct{ pid int }

// Prepare runs the child in its own process group so Kill reaches shells
// and test runners it spawned. Call before Start.
func Prepare(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// Attach returns the group of a started child.
func Attach(cmd *exec.Cmd) (*Group, error) { return &Group{pid: cmd.Process.Pid}, nil }

// Kill stops every process in the group, including ones whose parent
// already exited.
func (g *Group) Kill() { syscall.Kill(-g.pid, syscall.SIGKILL) }

// Close releases the group; on Unix there is nothing to release.
func (g *Group) Close() {}
