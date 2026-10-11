package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// These snapshots contain synthetic discovery data only. Updating requires an
// explicit environment variable; ordinary CI can never bless a changed catalog.
func baselineJSON(t *testing.T, name string, value any) {
	t.Helper()
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	path := filepath.Join("testdata", "migration", name+".json")
	if os.Getenv("ZL_MIGRATION_FREEZE") == "1" {
		if err := os.WriteFile(path, b, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var a, z any
	da := json.NewDecoder(bytes.NewReader(b))
	da.UseNumber()
	if err := da.Decode(&a); err != nil {
		t.Fatal(err)
	}
	dz := json.NewDecoder(bytes.NewReader(want))
	dz.UseNumber()
	if err := dz.Decode(&z); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, z) {
		t.Fatalf("compatibility snapshot changed: %s", path)
	}
}

func TestMigrationCatalogBaseline(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "migration-baseline", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	// Act
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	resources, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	templates, err := cs.ListResourceTemplates(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	names := contracts.Names()
	errorBytes, err := contracts.Files.ReadFile("error.json")
	if err != nil {
		t.Fatal(err)
	}
	var errorDoc map[string]any
	if err := json.Unmarshal(errorBytes, &errorDoc); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource("https://zl-mcp.local/error.json", errorDoc); err != nil {
		t.Fatal(err)
	}
	errorSchema, err := compiler.Compile("https://zl-mcp.local/error.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != len(names) || tools.NextCursor != "" {
		t.Fatal("catalog is incomplete")
	}
	for i, tool := range tools.Tools {
		if tool.Name != names[i] {
			t.Fatalf("catalog mismatch: %s != %s", tool.Name, names[i])
		}
	}
	baselineJSON(t, "tools", tools)
	baselineJSON(t, "resources", resources)
	baselineJSON(t, "resource-templates", templates)
	manifestBytes, err := os.ReadFile("../../docs/mcp-migration/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Tools []map[string]any `json:"tools"`
	}
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Tools) != len(tools.Tools) {
		t.Fatal("manifest tool coverage differs from discovery")
	}
	for i, tool := range tools.Tools {
		b, err := json.Marshal(tool)
		if err != nil {
			t.Fatal(err)
		}
		var descriptor map[string]any
		if err := json.Unmarshal(b, &descriptor); err != nil {
			t.Fatal(err)
		}
		for field, value := range descriptor {
			if !reflect.DeepEqual(manifest.Tools[i][field], value) {
				t.Fatalf("manifest mismatch for %s.%s", tool.Name, field)
			}
		}
		for _, field := range []string{"handler", "permissions", "effects", "operation_owner", "transports", "error_shape", "byte_limit", "normalization", "deadline", "admission"} {
			if manifest.Tools[i][field] == nil {
				t.Fatalf("missing manifest %s.%s", tool.Name, field)
			}
		}
	}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			// Arrange: forbidden property exercises strict schema validation without effects.
			time.Sleep(110 * time.Millisecond) // Respect the shared baseline 10/s admission budget.
			args := map[string]any{"migration_unexpected": "synthetic"}
			// Act
			result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
			// Assert
			if err != nil || result == nil || !result.IsError {
				t.Fatalf("invalid input dispatched: %v %+v", err, result)
			}
			var failure domain.Error
			if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &failure); err != nil || failure.Code != "INVALID_ARGUMENT" {
				t.Fatalf("expected validation error: %+v %v", failure, err)
			}
			var errorValue map[string]any
			if err := json.Unmarshal([]byte(result.Content[0].(*mcp.TextContent).Text), &errorValue); err != nil {
				t.Fatal(err)
			}
			if err := errorSchema.Validate(errorValue); err != nil {
				t.Fatal(err)
			}
			baselineJSON(t, name+"-invalid", result)
		})
	}
}
