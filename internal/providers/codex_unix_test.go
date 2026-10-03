//go:build !windows

package providers

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

func TestCodexCancelKillsStuckProcessTree(t *testing.T) {
	pidfile := filepath.Join(t.TempDir(), "pid")
	ctx := context.Background()
	s, _ := fakeCodexProvider(t, "stuck", "FAKE_PIDFILE="+pidfile).Start(ctx, codexReq())
	next(t, s)
	var pid int
	for i := 0; i < 50 && pid == 0; i++ {
		time.Sleep(20 * time.Millisecond)
		b, _ := os.ReadFile(pidfile)
		pid, _ = strconv.Atoi(string(b))
	}
	if pid == 0 {
		t.Fatal("grandchild did not start")
	}
	s.Cancel(ctx)
	if c := completed(t, drain(t, s)); c.Status != StatusCancelled {
		t.Fatalf("status = %s", c.Status)
	}
	deadline := time.Now().Add(3 * time.Second)
	for syscall.Kill(pid, 0) == nil {
		if time.Now().After(deadline) {
			t.Fatalf("grandchild %d still running after cancel", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
