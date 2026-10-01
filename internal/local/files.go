package local

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

// Lock is held for the lifetime of the collector or interactive login.
func Lock(dir string) (func(), error) {
	f, e := os.OpenFile(filepath.Join(dir, "account.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if e != nil {
		return nil, e
	}
	if e = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); e != nil {
		f.Close()
		return nil, fmt.Errorf("another collector or login owns this account")
	}
	return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }, nil
}
func WritePrivate(path string, b []byte) error {
	f, e := os.CreateTemp(filepath.Dir(path), ".private-*")
	if e != nil {
		return e
	}
	tmp := f.Name()
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
		return nil, fmt.Errorf("private file must be regular and accessible only to its owner")
	}
	return os.ReadFile(path)
}
