package store

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func canonicalCatalog(t *testing.T, value any) any {
	t.Helper()
	b, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	var result any
	if err = decoder.Decode(&result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCatalogPortsPreserveNativeNullableDiagnosticsAndIdentity(t *testing.T) {
	// Arrange: one borrowed owner, synthetic names/IDs and an auth-required state.
	ctx := context.Background()
	owner, err := storage.Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), []string{"9007199254740993"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = owner.ReplaceCatalog(ctx, []domain.Group{{ID: "9007199254740993", Name: "Cà phê 🍀"}}); err != nil {
		t.Fatal(err)
	}
	if err = owner.SetState(ctx, map[string]any{"authenticated": false, "collector_state": "auth_required", "last_connected_at": nil, "last_event_at": nil, "last_persisted_at": nil, "last_error": map[string]any{"code": "NOT_AUTHENTICATED", "message": "Run local login."}}); err != nil {
		t.Fatal(err)
	}
	reader := New(owner)
	for _, test := range []struct {
		name, schema string
		native       func() (any, error)
		typed        func() (any, error)
	}{
		{"collector", "zalo_get_status", func() (any, error) { return owner.State(ctx) }, func() (any, error) { return reader.CollectorStatus(ctx) }},
		{"collection", "conversation_collection", func() (any, error) { return owner.CollectionStatus(ctx) }, func() (any, error) { return reader.CollectionStatus(ctx) }},
		{"group", "zalo_get_group", func() (any, error) { return owner.Group(ctx, "9007199254740993") }, func() (any, error) { return reader.Group(ctx, "9007199254740993") }},
		{"groups", "zalo_list_groups", func() (any, error) { return owner.Groups(ctx, "Cà phê", 20, "") }, func() (any, error) {
			return reader.Groups(ctx, conversations.GroupRequest{Query: "Cà phê", Limit: 20})
		}},
		{"contact", "catalog_diagnostics", func() (any, error) { return owner.ContactStatus(ctx) }, func() (any, error) { return reader.ContactDiagnostics(ctx) }},
		{"profiles", "", func() (any, error) { return owner.ProfileStatus(ctx) }, func() (any, error) { return reader.ProfileDiagnostics(ctx) }},
		{"preload", "", func() (any, error) { return owner.PreloadStatus(ctx) }, func() (any, error) { return reader.PreloadDiagnostics(ctx) }},
		{"delivery", "events_diagnostics", func() (any, error) {
			return owner.DeliveryDiagnostics(ctx, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		}, func() (any, error) {
			return reader.DeliveryDiagnostics(ctx, time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC))
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Act.
			native, err := test.native()
			if err != nil {
				t.Fatal(err)
			}
			typed, err := test.typed()
			if err != nil {
				t.Fatal(err)
			}
			// Assert: no dropped null, rewritten opaque ID, new completeness claim or source change.
			if !reflect.DeepEqual(canonicalCatalog(t, typed), canonicalCatalog(t, native)) {
				t.Fatalf("projection changed: got %#v want %#v", typed, native)
			}
			if test.schema != "" {
				schema, err := contracts.Compile(test.schema, "output")
				if err != nil {
					t.Fatal(err)
				}
				b, _ := json.Marshal(typed)
				var wire any
				if err = json.Unmarshal(b, &wire); err != nil {
					t.Fatal(err)
				}
				if err = schema.Validate(wire); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
