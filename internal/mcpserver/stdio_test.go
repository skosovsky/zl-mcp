package mcpserver

import (
	"context"
	"fmt"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	token := strings.Repeat("t", 64)
	tokenPath := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	store, err := storage.Open(context.Background(), filepath.Join(dir, "messages.sqlite"), nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	httpService, err := NewHTTP(store, dir, token)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := httptest.NewServer(httpService)
	defer endpoint.Close()
	if err := os.WriteFile(config, []byte(fmt.Sprintf("state_dir = %q\n[collection]\ngroup_ids = []\n[mcp]\nlisten = %q\ntoken_file = %q\n", state, strings.TrimPrefix(endpoint.URL, "http://"), tokenPath)), 0600); err != nil {
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
	if err != nil || status.IsError || len(tools.Tools) != 22 {
		t.Fatalf("stdio failed: %+v %v", status, err)
	}
	resource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: "zalo://capabilities"})
	if err != nil || len(resource.Contents) != 1 || !strings.Contains(resource.Contents[0].Text, "Single personal account") {
		t.Fatalf("STDIO resource forwarding failed: %v %v", resource, err)
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Fatal("STDIO bridge created its own state directory", err)
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
}
