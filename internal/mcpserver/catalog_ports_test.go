package mcpserver

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type unavailableCatalogRefresh struct{ calls int }

func (b *unavailableCatalogRefresh) Call(_ context.Context, name string, args any) (map[string]any, error) {
	b.calls++
	if name != "zalo_get_group" || args.(map[string]any)["group_id"] != "9007199254740993" {
		return nil, errors.New("unexpected control dispatch")
	}
	return nil, errors.New("synthetic live refresh unavailable")
}

//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestCatalogToolsAndStaticResourcesShareSingleNativeAdmission(t *testing.T) {
	// Arrange: real MCP, complete synthetic catalogue, fixed trusted account and
	// test-only DB triggers which observe the actual shared bucket writes.
	ctx := context.Background()
	owner, err := storage.Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), []string{"9007199254740993"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = owner.BindAccount(ctx, "synthetic-catalog-account"); err != nil {
		t.Fatal(err)
	}
	if err = owner.ReplaceCatalog(ctx, []domain.Group{{ID: "9007199254740993", Name: "Cà phê 🍀"}}); err != nil {
		t.Fatal(err)
	}
	for _, ddl := range []string{
		"CREATE TABLE test_catalog_admissions(n INTEGER)",
		"CREATE TRIGGER test_catalog_admission_insert AFTER INSERT ON read_budget BEGIN INSERT INTO test_catalog_admissions VALUES(1); END",
		"CREATE TRIGGER test_catalog_admission_update AFTER UPDATE ON read_budget BEGIN INSERT INTO test_catalog_admissions VALUES(1); END",
	} {
		if _, err = owner.DB.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	backend := &unavailableCatalogRefresh{}
	server, err := New(migrationReadPorts(owner, backend))
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "synthetic-catalog-ports", Version: "1"}, nil).
		Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	// Act: scoped tools and independent native resources use the same port/bucket.
	for _, call := range []struct {
		name string
		args map[string]any
	}{
		{"zalo_get_status", map[string]any{}},
		{"zalo_list_groups", map[string]any{"query": "Cà phê"}},
		{"zalo_get_group", map[string]any{"group_id": "9007199254740993"}},
	} {
		result, callToolErr := cs.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if callToolErr != nil || result.IsError {
			t.Fatalf("catalog tool %s failed: %v %+v", call.name, callToolErr, result)
		}
		if call.name == "zalo_get_group" && result.StructuredContent.(map[string]any)["stale"] != true {
			t.Fatal("native fallback lost stale=true")
		}
	}
	for _, uri := range []string{"zalo://capabilities", "zalo://catalog/diagnostics", "zalo://collection", "zalo://events/diagnostics"} {
		result, readResourceErr := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if readResourceErr != nil || len(result.Contents) != 1 || result.Contents[0].URI != uri {
			t.Fatalf("diagnostics resource %s failed: %v", uri, readResourceErr)
		}
	}
	// Assert: no extra admission or command from the live-read fallback.
	var admitted int
	if err = owner.DB.QueryRowContext(ctx, "SELECT count(*) FROM test_catalog_admissions").Scan(&admitted); err != nil {
		t.Fatal(err)
	}
	if admitted != 7 || backend.calls != 1 {
		t.Fatalf("changed admission/native read count: admitted%d control%d", admitted, backend.calls)
	}
}
