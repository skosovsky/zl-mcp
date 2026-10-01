package local

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// SocketPath validates the byte limit of sockaddr_un before any network login.
func SocketPath(dir string) (string, error) {
	path := filepath.Join(dir, "collector.sock")
	limit := 108
	if runtime.GOOS == "darwin" {
		limit = 104
	}
	if len([]byte(path)) >= limit {
		return "", fmt.Errorf("state_dir is too long for the Unix socket; use a shorter absolute path (socket path must be less than %d bytes)", limit)
	}
	return path, nil
}
