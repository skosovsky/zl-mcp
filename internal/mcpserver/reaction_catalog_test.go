package mcpserver

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestReactionChoicesAreDiscoverableReadOnlyContractData(t *testing.T) {
	// Arrange: no native client or action permission is available to this server.
	owner, err := storage.OpenWithPolicy(
		t.Context(),
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	server, err := New(migrationReadPorts(owner))
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	service, err := server.Connect(t.Context(), st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "reaction-catalog-test", Version: "1"}, nil).
		Connect(t.Context(), ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	// Act: discover then read through the real SDK resource route.
	listed, err := client.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, resource := range listed.Resources {
		if resource.URI == reactionCatalogURI {
			found = true
		}
	}
	result, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: reactionCatalogURI})
	// Assert: exact supported choices are visible; no operation was created.
	if !found || err != nil || len(result.Contents) != 1 {
		t.Fatal("reaction catalog unavailable", err)
	}
	var wire struct {
		Reactions []domain.ReactionOption `json:"reactions"`
	}
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &wire); err != nil {
		t.Fatal(err)
	}
	expected := domain.ReactionCatalog()
	if len(wire.Reactions) != len(expected) {
		t.Fatal("catalogue lost choices")
	}
	for i, option := range expected {
		if wire.Reactions[i] != option {
			t.Fatal("reaction code/name changed")
		}
	}
	schema, err := contracts.Compile("reaction_catalog", "output")
	if err != nil {
		t.Fatal(err)
	}
	var raw any
	if err = json.Unmarshal([]byte(result.Contents[0].Text), &raw); err != nil || schema.Validate(raw) != nil {
		t.Fatal("resource violates executable schema")
	}
	var intents int
	if err = owner.DB.QueryRow(`SELECT count(*) FROM message_action_operations`).
		Scan(&intents); err != nil ||
		intents != 0 {
		t.Fatal("catalog read created a write intent")
	}
}
