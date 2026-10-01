package mcpserver

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestActualBinaryStdioLifecycle(t *testing.T) {
	// Arrange: compile the CLI and connect through an actual child-process pipe.
	dir := t.TempDir()
	bin := filepath.Join(dir, "zl-mcp")
	build := exec.Command("go", "build", "-o", bin, "./cmd/zl-mcp")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", output, err)
	}
	config := filepath.Join(dir, "config.toml")
	state := filepath.Join(dir, "state")
	if err := os.WriteFile(config, []byte(fmt.Sprintf("state_dir = %q\n[collection]\ngroup_ids = []\n", state)), 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	child := exec.Command(bin, "-config", config, "serve")
	child.Stderr = os.Stderr
	client := mcp.NewClient(&mcp.Implementation{Name: "stdio-acceptance", Version: "1"}, nil)
	// Act: stdout must contain only MCP, otherwise initialize or these calls fail.
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: child}, nil)
	if err != nil {
		t.Fatal(err)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	status, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_get_status", Arguments: map[string]any{}})
	// Assert
	if err != nil || status.IsError || len(tools.Tools) != 8 {
		t.Fatalf("stdio failed: %+v %v", status, err)
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
}
