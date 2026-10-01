package local

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExclusiveLockAndPrivateSession(t *testing.T) {
	// Arrange
	dir := t.TempDir()
	release, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Act
	_, second := Lock(dir)
	release()
	releaseAgain, again := Lock(dir)
	// Assert
	if second == nil {
		t.Fatal("second collector acquired account lock")
	}
	if again != nil {
		t.Fatal(again)
	}
	releaseAgain()
	// Arrange
	path := filepath.Join(dir, "session.json")
	// Act
	err = WritePrivate(path, []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := ReadPrivate(path)
	// Assert
	if err != nil || string(data) != "secret" {
		t.Fatal("private session read failed")
	}
	if err = os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadPrivate(path); err == nil {
		t.Fatal("world-readable session accepted")
	}
}
