package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// maxLineBytes bounds one JSONL message from a child process.
const maxLineBytes = 16 << 20

// proc is a managed child process exchanging newline-delimited JSON over
// stdin/stdout. Executable, args and stdin are always kept separate; user
// text never goes through a shell.
type proc struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  chan []byte
	exited chan struct{}
	err    error // set before exited closes

	writeMu sync.Mutex
	stderr  *tailBuffer
}

func startProc(ctx context.Context, exe string, args []string, dir string, env []string) (*proc, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.WaitDelay = 3 * time.Second
	configureProcAttr(cmd)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	p := &proc{cmd: cmd, stdin: stdin, lines: make(chan []byte, 64), exited: make(chan struct{}), stderr: newTailBuffer(64 << 10)}
	cmd.Stderr = p.stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s: %w", exe, err)
	}
	go func() {
		sc := bufio.NewScanner(stdout)
		sc.Buffer(make([]byte, 64<<10), maxLineBytes)
		for sc.Scan() {
			line := append([]byte(nil), sc.Bytes()...)
			if len(strings.TrimSpace(string(line))) > 0 {
				p.lines <- line
			}
		}
		p.err = cmd.Wait()
		close(p.lines)
		close(p.exited)
	}()
	return p, nil
}

func (p *proc) send(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	_, err = p.stdin.Write(append(b, '\n'))
	return err
}

// stop closes stdin and, if the process has not exited within grace,
// kills it (and its process group where supported).
func (p *proc) stop(grace time.Duration) {
	p.stdin.Close()
	select {
	case <-p.exited:
		return
	case <-time.After(grace):
	}
	killProc(p.cmd)
	<-p.exited
}

// stderrTail returns recent stderr output for diagnostics.
func (p *proc) stderrTail() string { return p.stderr.String() }

// tailBuffer keeps only the last n bytes written.
type tailBuffer struct {
	mu  sync.Mutex
	n   int
	buf []byte
}

func newTailBuffer(n int) *tailBuffer { return &tailBuffer{n: n} }

func (t *tailBuffer) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if over := len(t.buf) - t.n; over > 0 {
		t.buf = t.buf[over:]
	}
	return len(b), nil
}

func (t *tailBuffer) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

var errProcExited = errors.New("provider process exited")
