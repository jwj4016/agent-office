//go:build windows

package proc

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func alive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	return windows.GetExitCodeProcess(h, &code) == nil && code == 259 // STILL_ACTIVE
}

func killPid(pid int) {
	if h, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid)); err == nil {
		windows.TerminateProcess(h, 1)
		windows.CloseHandle(h)
	}
}

// Closing the job (the app exiting, even by crashing) ends what is left.
func TestCloseEndsTheJob(t *testing.T) {
	g, pid := startOrphan(t)
	g.Close()
	waitGone(t, pid)
}

func TestHideSetsNoWindow(t *testing.T) {
	c := exec.Command("cmd")
	Hide(c)
	if !c.SysProcAttr.HideWindow || c.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("%+v", c.SysProcAttr)
	}
}
