package local

import (
	"errors"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// Lock is held for the lifetime of the collector or interactive login.
func Lock(dir string) (func(), error) {
	f, e := os.OpenFile(filepath.Join(dir, "account.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
		f.Close()
		return nil, errors.New("another collector or login owns this account")
	}
	return func() {
		//nolint:errcheck,gosec // Closing the descriptor also releases the advisory lock.
		unix.Flock(int(f.Fd()), unix.LOCK_UN)
		//nolint:gosec // Lock cleanup has no caller error channel and must run exactly once.
		f.Close()
	}, nil
}
func WritePrivate(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".private-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
	// G104: Scratch cleanup preserves the write/rename result.
	defer os.Remove(tmp)
	if e = f.Chmod(0600); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return os.Rename(tmp, path)
}
func ReadPrivate(path string) ([]byte, error) {
	st, e := os.Lstat(path)
	if e != nil {
		return nil, e
	}
	if !st.Mode().IsRegular() || st.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private file must be regular and accessible only to its owner")
	}
	return os.ReadFile(path)
}
