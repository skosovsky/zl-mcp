package logging

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestRotationPreservesSizeArchiveCountAndPrivacy(t *testing.T) {
	// Arrange: use the production 5 MiB / three archives policy.
	dir := filepath.Join(t.TempDir(), "logs")
	path := filepath.Join(dir, "service.log")
	var fallback bytes.Buffer
	sink, err := Open(path, 5, 3, &fallback)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	record := []byte(strings.Repeat("a", 1<<20) + "\n")
	// Act: cross five rotation boundaries, ensuring distinct millisecond names.
	for range 24 {
		if _, writeErr := sink.Write(record); writeErr != nil {
			t.Fatal(writeErr)
		}
		time.Sleep(2 * time.Millisecond)
	}
	if closeErr := sink.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	deadline := time.Now().Add(3 * time.Second)
	var files []string
	for {
		files, err = filepath.Glob(filepath.Join(dir, "*.log"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) == 4 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("cleanup left %d files", len(files))
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Assert: archives are bounded and every file remains private.
	for _, file := range files {
		info, statErr := os.Stat(file)
		if statErr != nil {
			t.Fatal(statErr)
		}
		if info.Size() > 5<<20 || info.Mode().Perm() != 0600 {
			t.Fatalf("unsafe file: %s size=%d mode=%v", file, info.Size(), info.Mode())
		}
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0700 || fallback.Len() != 0 || sink.Err() != nil {
		t.Fatal("unsafe directory or logging failure")
	}
}
func TestOversizedRecordProducesObservableFailure(t *testing.T) {
	// Arrange
	var fallback bytes.Buffer
	sink, err := Open(filepath.Join(t.TempDir(), "service.log"), 1, 3, &fallback)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()
	// Act
	_, err = sink.Write([]byte(strings.Repeat("private-message", 100000)))
	// Assert: slog cannot hide the failure, and original data is not copied to stderr.
	select {
	case <-sink.Failed():
	default:
		t.Fatal("failure not signaled")
	}
	if err == nil || sink.Err() == nil || !strings.Contains(fallback.String(), "stopping service") ||
		strings.Contains(fallback.String(), "private-message") {
		t.Fatal("missing or unsafe failure diagnostic")
	}
}
func TestUnsafeLogPathFailsBeforeWriting(t *testing.T) {
	for _, kind := range []string{"public-file", "symlink-file", "symlink-directory"} {
		t.Run(kind, func(t *testing.T) {
			// Arrange
			dir := t.TempDir()
			path := filepath.Join(dir, "service.log")
			switch kind {
			case "public-file":
				if err := os.WriteFile(path, []byte("existing"), 0644); err != nil {
					t.Fatal(err)
				}
			case "symlink-file":
				target := filepath.Join(dir, "target")
				os.WriteFile(target, []byte("existing"), 0600)
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			default:
				target := filepath.Join(dir, "real")
				os.Mkdir(target, 0700)
				link := filepath.Join(dir, "link")
				os.Symlink(target, link)
				path = filepath.Join(link, "service.log")
			}
			// Act
			_, err := Open(path, 5, 3, &bytes.Buffer{})
			// Assert
			if err == nil {
				t.Fatal("unsafe log path accepted")
			}
		})
	}
}
