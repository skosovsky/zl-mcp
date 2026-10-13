package logging

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

// ReportStartupFailure records a bounded, private diagnostic without raw error
// strings, which may contain caller-controlled configuration or private data.
func ReportStartupFailure(err error) bool {
	home, homeErr := os.UserHomeDir()
	if homeErr != nil {
		return false
	}
	return reportStartupFailure(filepath.Join(home, "Library", "Logs", "zl-mcp", "startup.log"), err, os.Stderr)
}
func reportStartupFailure(path string, cause error, fallback io.Writer) bool {
	category := "startup_or_runtime_failure"
	switch {
	case errors.Is(cause, os.ErrPermission):
		category = "permission_denied"
	case errors.Is(cause, os.ErrNotExist):
		category = "file_not_found"
	case strings.HasPrefix(cause.Error(), "invalid config TOML:") || strings.HasPrefix(cause.Error(), "invalid MCP service configuration:") || strings.HasPrefix(cause.Error(), "invalid service logging configuration:"):
		category = "invalid_config"
	}
	body, _ := json.Marshal(
		map[string]any{
			"time":     time.Now().UTC().Format(time.RFC3339Nano),
			"level":    "ERROR",
			"msg":      "zl-mcp service failed; run interactively for details",
			"category": category,
			"pid":      os.Getpid(),
		},
	)
	body = append(body, '\n')
	sink, err := Open(path, startupLogSizeMB, startupLogBackups, fallback)
	if err != nil {
		//nolint:errcheck,gosec // Last-resort diagnostic output cannot report its own failure recursively.
		fallback.Write(body)
		return false
	}
	// G104: Cleanup preserves the original setup/operation error.
	defer sink.Close()
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		//nolint:errcheck,gosec // Last-resort diagnostic output cannot report its own failure recursively.
		fallback.Write(body)
		return false
	}
	// G104: Cleanup preserves the original setup/operation error.
	defer lock.Close()
	deadline := time.Now().Add(time.Second)
	for {
		err = unix.Flock(int(lock.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, unix.EWOULDBLOCK) || time.Now().After(deadline) {
			//nolint:errcheck,gosec // Last-resort diagnostic output cannot report its own failure recursively.
			fallback.Write(body)
			return false
		}
		time.Sleep(startupLogDrainInterval)
	}
	//nolint:errcheck // Unlock is fallback cleanup; closing lock releases the OS lock.
	defer unix.Flock(int(lock.Fd()), unix.LOCK_UN)
	_, err = sink.Write(body)
	return err == nil
}
