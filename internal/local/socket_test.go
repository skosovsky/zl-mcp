package local

import (
	"strings"
	"testing"
)

func TestSocketPathRejectsLongUTF8Path(t *testing.T) {
	// Arrange
	dir := "/tmp/" + strings.Repeat("я", 60)
	// Act
	_, err := SocketPath(dir)
	short, shortErr := SocketPath("/tmp/zl-mcp")
	// Assert
	if err == nil || shortErr != nil || short != "/tmp/zl-mcp/collector.sock" {
		t.Fatalf("long=%v short=%q %v", err, short, shortErr)
	}
}
