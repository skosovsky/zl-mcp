package evaluation

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

var ErrOutputBudget = errors.New("child output budget exceeded")

type boundedBuffer struct {
	mu       sync.Mutex
	buffer   bytes.Buffer
	limit    int
	cancel   context.CancelFunc
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(p) > b.limit-b.buffer.Len() {
		b.exceeded = true
		b.cancel()
		return 0, ErrOutputBudget
	}
	return b.buffer.Write(p)
}

// Process is the external watchdog. It always terminates and Waits a child group.
// stdin is absent and the environment contains no account/provider credentials.
func Process(ctx context.Context, binary string, args []string, limit int) ([]byte, []byte, error) {
	return process(ctx, binary, args, limit, true)
}

// A model driver inherits the case process group so the outer watchdog owns descendants.
func process(ctx context.Context, binary string, args []string, limit int, ownGroup bool) ([]byte, []byte, error) {
	if limit <= 0 {
		return nil, nil, errors.New("invalid output bound")
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	stdout := &boundedBuffer{limit: limit, cancel: cancel}
	stderr := &boundedBuffer{limit: limit, cancel: cancel}
	cmd := exec.Command(binary, args...)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "TMPDIR=" + os.TempDir(), "TZ=UTC"}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	cmd.WaitDelay = 2 * time.Second
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: ownGroup}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-run.Done():
		if ownGroup {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		} else {
			_ = cmd.Process.Kill()
		}
		<-done // Reap the child; cancellation/output budget determines the returned error.
		if ctx.Err() != nil {
			err = ctx.Err()
		} else {
			err = ErrOutputBudget
		}
	}
	// The child may spawn descendants; do not leave them after normal exit either.
	if ownGroup {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	} else {
		_ = cmd.Process.Kill()
	}
	stdout.mu.Lock()
	out := bytes.Clone(stdout.buffer.Bytes())
	overflow := stdout.exceeded
	stdout.mu.Unlock()
	stderr.mu.Lock()
	logs := bytes.Clone(stderr.buffer.Bytes())
	overflow = overflow || stderr.exceeded
	stderr.mu.Unlock()
	if overflow {
		err = ErrOutputBudget
	}
	return out, logs, err
}
