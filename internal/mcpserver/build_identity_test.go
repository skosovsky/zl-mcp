package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/buildinfo"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestSDKReportsActualBackendIdentityOnSuccessAndError(t *testing.T) {
	// Arrange: actual SDK connection and isolated local corpus, without upstream.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), nil, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	server, err := New(migrationReadPorts(store))
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	owner, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "build-identity-test", Version: "1"}, nil).
		Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	schema, err := contracts.Compile("build_identity", "output")
	if err != nil {
		t.Fatal(err)
	}
	expected := buildinfo.Current()
	// Act
	initial := client.InitializeResult()
	// Assert
	if initial.ServerInfo.Version != expected.DisplayVersion() {
		t.Fatalf("wrong application identity: %+v", initial.ServerInfo)
	}
	if initial.ProtocolVersion == initial.ServerInfo.Version {
		t.Fatal("protocol and application version conflated")
	}
	for _, args := range []map[string]any{{}, {"unexpected": true}} {
		// Act
		result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_get_status", Arguments: args})
		// Assert
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(result.Meta[buildinfo.MetadataKey])
		if err != nil {
			t.Fatal(err)
		}
		var identity map[string]any
		if err := json.Unmarshal(raw, &identity); err != nil {
			t.Fatal(err)
		}
		if err := schema.Validate(identity); err != nil {
			t.Fatal(err)
		}
		var actual struct {
			Backend buildinfo.Identity `json:"backend"`
		}
		if err := json.Unmarshal(raw, &actual); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(expected, actual.Backend) {
			t.Fatalf("identity changed crossing SDK: %+v", actual)
		}
	}
}
