package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type archiveRoutingControl struct{ calls []string }

func (c *archiveRoutingControl) Call(_ context.Context, method string, _ any) (map[string]any, error) {
	c.calls = append(c.calls, method)
	if method == "zalo_list_archive_sources" {
		return map[string]any{"sources": []map[string]any{}, "history_complete": false}, nil
	}
	return nil, &domain.Error{Code: "SOURCE_UNAVAILABLE", Message: "Synthetic source unavailable.", Details: map[string]any{}}
}

func TestArchiveRequestsNeverFallBackToCorpus(t *testing.T) {
	// Arrange: local corpus remains available while the selected archive is unavailable.
	ctx := context.Background()
	store, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	backend := &archiveRoutingControl{}
	service := &Service{Store: store, Control: backend, input: map[string]*jsonschema.Schema{}, output: map[string]*jsonschema.Schema{}}
	for _, name := range []string{"zalo_list_archive_sources", "zalo_list_conversations", "zalo_list_conversation_messages"} {
		service.input[name], err = contracts.Compile(name, "input")
		if err != nil {
			t.Fatal(err)
		}
		service.output[name], err = contracts.Compile(name, "output")
		if err != nil {
			t.Fatal(err)
		}
	}
	// Act: discover sources, browse explicit archive and browse the default corpus.
	inventory := service.call(ctx, "zalo_list_archive_sources", json.RawMessage(`{}`))
	catalogue := service.call(ctx, "zalo_list_conversations", json.RawMessage(`{"source_id":"00000000-0000-4000-8000-000000000001"}`))
	messages := service.call(ctx, "zalo_list_conversation_messages", json.RawMessage(`{"source_id":"00000000-0000-4000-8000-000000000001","conversation_type":"direct","conversation_id":"12","since":"2026-09-01T00:00:00Z","until":"2026-10-01T00:00:00Z"}`))
	corpus := service.call(ctx, "zalo_list_conversations", json.RawMessage(`{}`))
	// Assert: explicit archive failures stay failures; corpus defaults and inventory validate.
	if inventory.IsError || !catalogue.IsError || !messages.IsError || corpus.IsError {
		t.Fatal("archive request silently used corpus or broke defaults")
	}
	if len(backend.calls) != 3 || backend.calls[1] != "zalo_list_conversations" || backend.calls[2] != "zalo_list_conversation_messages" {
		t.Fatal("archive routing bypassed service owner")
	}
}
