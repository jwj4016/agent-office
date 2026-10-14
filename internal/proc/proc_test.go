package proc

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The test binary doubles as the processes under test: a parent that
// starts a long-lived grandchild, prints its pid and exits at once.
func TestMain(m *testing.M) {
	switch os.Getenv("PROC_TEST_ROLE") {
	case "parent":
		c := exec.Command(os.Args[0], "-test.run=^$")
		c.Env = append(os.Environ(), "PROC_TEST_ROLE=sleeper")
		Hide(c)
		if err := c.Start(); err != nil {
			os.Exit(2)
		}
		os.Stdout.WriteString(strconv.Itoa(c.Process.Pid) + "\n")
		os.Exit(0)
	case "sleeper":
		time.Sleep(2 * time.Minute)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// startOrphan runs a parent that leaves a grandchild behind and returns
// the group and the grandchild's pid once the parent has exited.
func startOrphan(t *testing.T) (*Group, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(), "PROC_TEST_ROLE=parent")
	Prepare(cmd)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	g, err := Attach(cmd)
	if err != nil {
		t.Fatalf("attach: %v", err)
	}
	line, _ := bufio.NewReader(out).ReadString('\n')
	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("grandchild pid %q: %v", line, err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if !alive(pid) {
		t.Fatal("grandchild should still run after its parent exited")
	}
	t.Cleanup(func() { killPid(pid) })
	return g, pid
}

func waitGone(t *testing.T, pid int) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d survived", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestKillReachesOrphanedGrandchild(t *testing.T) {
	g, pid := startOrphan(t)
	g.Kill()
	waitGone(t, pid)
	g.Close()
}
