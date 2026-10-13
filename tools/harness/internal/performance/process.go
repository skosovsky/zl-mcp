package performance

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// Process owns one process group. Even successful workers cannot leave descendants.
// Explicit pipes keep inherited descendant descriptors out of exec.Cmd.Wait.
//
//nolint:gocognit,funlen // The watchdog owns process-group cancellation, bounded output, Kill, Wait and drain in one ordered lifetime.
func process(ctx context.Context, dir string, env []string, args []string, limit int) ([]byte, []byte, int, error) {
	if len(args) == 0 {
		return nil, nil, -1, errors.New("empty command")
	}
	outR, outW, err := os.Pipe()
	if err != nil {
		return nil, nil, -1, err
	}
	defer outR.Close()
	errR, errW, err := os.Pipe()
	if err != nil {
		//nolint:gosec // G104: process/temporary-workspace cleanup cannot replace the original failure or watchdog outcome.
		outW.Close()
		return nil, nil, -1, err
	}
	defer errR.Close()
	//nolint:gosec,noctx // The group watchdog owns cancellation, Kill and Wait. G204: explicitly selected executable/argv are invoked without shell interpolation under the group watchdog.
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdout = outW
	cmd.Stderr = errW
	cmd.Stdin = nil
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err = cmd.Start(); err != nil {
		//nolint:gosec // G104: process/temporary-workspace cleanup cannot replace the original failure or watchdog outcome.
		outW.Close()
		//nolint:gosec // G104: process/temporary-workspace cleanup cannot replace the original failure or watchdog outcome.
		errW.Close()
		return nil, nil, -1, err
	}
	//nolint:gosec // G104: process/temporary-workspace cleanup cannot replace the original failure or watchdog outcome.
	outW.Close()
	//nolint:gosec // G104: process/temporary-workspace cleanup cannot replace the original failure or watchdog outcome.
	errW.Close()
	var mu sync.Mutex
	var stdout, stderr bytes.Buffer
	remaining := limit
	overflow := make(chan struct{}, 1)
	var drains sync.WaitGroup
	drain := func(r io.Reader, b *bytes.Buffer) {
		defer drains.Done()
		buf := make([]byte, 8192)
		for {
			n, e := r.Read(buf)
			if n > 0 {
				mu.Lock()
				keep := min(n, max(remaining, 0))
				b.Write(buf[:keep])
				remaining -= n
				if remaining < 0 {
					select {
					case overflow <- struct{}{}:
					default:
					}
				}
				mu.Unlock()
			}
			if e != nil {
				return
			}
		}
	}
	drains.Add(2)
	go drain(outR, &stdout)
	go drain(errR, &stderr)
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	var waitErr, runErr error
	select {
	case waitErr = <-waited:
	case <-ctx.Done():
		runErr = fmt.Errorf("wall-clock budget exceeded: %w", ctx.Err())
	case <-overflow:
		runErr = errors.New("output budget exceeded")
	}
	// Killing the owned group also closes descriptors inherited by descendants.
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	if runErr != nil {
		waitErr = <-waited
	}
	drained := make(chan struct{})
	go func() { drains.Wait(); close(drained) }()
	select {
	case <-drained:
	case <-time.After(2 * time.Second):
		//nolint:gosec // G104: process/temporary-workspace cleanup cannot replace the original failure or watchdog outcome.
		outR.Close()
		//nolint:gosec // G104: process/temporary-workspace cleanup cannot replace the original failure or watchdog outcome.
		errR.Close()
		<-drained
		runErr = errors.New("inherited output descriptors did not close")
	}
	mu.Lock()
	defer mu.Unlock()
	if remaining < 0 {
		runErr = errors.New("output budget exceeded")
	}
	code := cmd.ProcessState.ExitCode()
	if runErr == nil && waitErr != nil {
		runErr = waitErr
	}
	return append([]byte(nil), stdout.Bytes()...), append([]byte(nil), stderr.Bytes()...), code, runErr
}
