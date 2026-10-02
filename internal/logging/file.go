// Package logging provides a private file sink with observable write failures.
package logging

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	"gopkg.in/natefinch/lumberjack.v2"
)

type File struct {
	writer   io.WriteCloser
	fallback io.Writer
	failed   chan struct{}
	mu       sync.Mutex
	err      error
}

func Open(path string, maxSizeMB, backups int, fallback io.Writer) (*File, error) {
	if maxSizeMB < 1 || backups < 1 || fallback == nil {
		return nil, errors.New("invalid file log options")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !info.IsDir() || !ok || owner.Uid != uint32(os.Getuid()) {
		return nil, errors.New("log directory must belong to current user and be a real directory")
	}
	if err := os.Chmod(dir, 0700); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		owner, ok := info.Sys().(*syscall.Stat_t)
		if !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 || !ok || owner.Uid != uint32(os.Getuid()) {
			return nil, errors.New("log file must be regular, private and owned by current user")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	if err = f.Close(); err != nil {
		return nil, err
	}
	return &File{writer: &lumberjack.Logger{Filename: path, MaxSize: maxSizeMB, MaxBackups: backups}, fallback: fallback, failed: make(chan struct{})}, nil
}
func (f *File) Write(p []byte) (int, error) {
	n, err := f.writer.Write(p)
	if err == nil && n != len(p) {
		err = io.ErrShortWrite
	}
	if err != nil {
		f.mu.Lock()
		if f.err == nil {
			f.err = fmt.Errorf("file logging failed: %w", err)
			// The diagnostic is fixed: it contains neither failed content nor credentials.
			_, _ = io.WriteString(f.fallback, "{\"level\":\"ERROR\",\"msg\":\"file logging failed; stopping service\"}\n")
			close(f.failed)
		}
		f.mu.Unlock()
	}
	return n, err
}
func (f *File) Failed() <-chan struct{} { return f.failed }
func (f *File) Err() error              { f.mu.Lock(); defer f.mu.Unlock(); return f.err }
func (f *File) Close() error            { return f.writer.Close() }
