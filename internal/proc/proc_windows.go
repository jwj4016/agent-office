//go:build windows

package proc

import (
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Hide keeps a console child from opening a window.
func Hide(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_NO_WINDOW
}

// Group is a Job Object holding the child. Processes the child starts
// join the job too, so Kill reaches grandchildren even after their parent
// exited, and the job closing (including the app crashing) kills them.
// Without a job (assigning failed) Kill falls back to taskkill /T.
type Group struct {
	mu      sync.Mutex
	job     windows.Handle
	process *os.Process
}

// Prepare hides the console window. Call before Start.
func Prepare(cmd *exec.Cmd) { Hide(cmd) }

// Attach puts a started child into a new kill-on-close Job Object. The
// child is assigned right after it is created, before it has loaded
// enough to start children of its own. The error says why the job could
// not be used; the returned Group still works through taskkill.
func Attach(cmd *exec.Cmd) (*Group, error) {
	g := &Group{process: cmd.Process}
	job, err := newJob(cmd.Process.Pid)
	if err != nil {
		return g, err
	}
	g.job = job
	return g, nil
}

func newJob(pid int) (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	defer windows.CloseHandle(h)
	if err := windows.AssignProcessToJobObject(job, h); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

// Kill terminates every process in the job.
func (g *Group) Kill() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.job != 0 {
		windows.TerminateJobObject(g.job, 1)
		return
	}
	// taskkill walks the child tree but can take seconds on a busy
	// machine, so the child itself is killed directly once that wait
	// runs out (the tree walk still finishes in the background).
	done := make(chan struct{})
	go func() {
		c := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(g.process.Pid))
		Hide(c)
		c.Run()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		g.process.Kill()
	}
}

// Close kills what is left and releases the job.
func (g *Group) Close() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.job != 0 {
		windows.CloseHandle(g.job) // kill-on-close ends any survivors
		g.job = 0
	}
}
