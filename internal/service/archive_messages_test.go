package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
)

func testOfflineArchiveMessages(t *testing.T, offline, restricted *membershipPort, sourceID string) {
	t.Helper()
	// Arrange: immutable synthetic SQLite source, empty corpus and disconnected collector.
	ctx := context.Background()
	args := map[string]any{"source_id": sourceID, "conversation_type": "direct", "conversation_id": "12", "since": "2000-01-01T00:00:00Z", "until": "2030-01-01T00:00:00Z", "order": "asc", "limit": 1}
	schema, err := contracts.Compile("zalo_list_conversation_messages", "output")
	if err != nil {
		t.Fatal(err)
	}
	binding, err := offline.store.ArchiveAccountKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := offline.library.PreservationStatus(ctx, sourceID, binding)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	testedResource := false
	ended := false
	// Act: follow examined-row pages, including empty pages, then replay one resource.
	for index := 0; index < 20; index++ {
		result, err := offline.Call(ctx, "zalo_list_conversation_messages", args)
		if err != nil {
			t.Fatal("offline archived page failed", err)
		}
		body, _ := json.Marshal(result)
		var value any
		json.Unmarshal(body, &value)
		if err = schema.Validate(value); err != nil {
			t.Fatal("archive page contract", err)
		}
		for _, record := range result["messages"].([]map[string]any) {
			id := record["archive_row_id"].(string)
			if seen[id] || record["quote_anchor_eligible"] != false {
				t.Fatal("archive identity repeated or promoted to quote anchor")
			}
			seen[id] = true
			if !testedResource {
				token := archiveReadToken{Version: 1, Kind: "resource", Account: binding, Source: sourceID, Digest: receipt.Source.Digest, Ref: domain.ConversationRef{Type: "direct", ID: "12"}, Since: args["since"].(string), Until: args["until"].(string), Order: "asc", Target: id, Limit: 1}
				if prior, ok := args["cursor"].(string); ok {
					saved, err := offline.openArchiveRead(ctx, "cursor", prior)
					if err != nil {
						t.Fatal(err)
					}
					token.Position = saved.Position
				}
				sealed, err := offline.sealArchiveRead(ctx, "resource", token)
				if err != nil {
					t.Fatal(err)
				}
				full, err := offline.Call(ctx, "read_archive_resource", map[string]any{"token": sealed})
				if err != nil || full["archive_row_id"] != id {
					t.Fatal("resource did not replay exact source record", err)
				}
				fullText, ok := full["text"].(string)
				if !ok || (record["text_truncated"] == true && (len([]rune(fullText)) <= 2048 || string([]rune(fullText)[:2048]) != record["text"])) || (record["text_truncated"] != true && fullText != record["text"]) {
					t.Fatal("resource changed the excerpt or lost full text")
				}
				if _, err = restricted.Call(ctx, "read_archive_resource", map[string]any{"token": sealed}); err == nil {
					t.Fatal("resource bypassed current collection policy")
				}
				testArchiveSDKResource(t, offline, sealed, id)
				if _, err = offline.Call(ctx, "zalo_list_conversation_messages", map[string]any{"source_id": sourceID, "conversation_type": "direct", "conversation_id": "12", "since": args["since"], "until": args["until"], "cursor": sealed}); err == nil {
					t.Fatal("resource substituted for cursor")
				}
				testedResource = true
			}
		}
		if !result["has_more"].(bool) {
			if result["next_cursor"] != nil {
				t.Fatal("terminal archive page has cursor")
			}
			ended = true
			break
		}
		args["cursor"] = result["next_cursor"]
	}
	// Assert: bounded history is available without network, phone dispatch or corpus import.
	if !ended || len(seen) == 0 || !testedResource {
		t.Fatal("synthetic archive walk did not yield readable records")
	}
	if _, err = restricted.Call(ctx, "zalo_list_conversation_messages", args); err == nil {
		t.Fatal("archive messages bypassed policy")
	}
}

func testArchiveSDKResource(t *testing.T, owner *membershipPort, token, id string) {
	t.Helper()
	// Arrange: actual MCP SDK transport wired to the offline service owner.
	ctx := context.Background()
	server, err := mcpserver.NewWithControl(owner.store, owner)
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "archive-contract-test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	// Act: source discovery and full-record read through public MCP handlers.
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_list_archive_sources", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatal("MCP archive discovery failed", err)
	}
	claim, err := owner.openArchiveRead(ctx, "resource", token)
	if err != nil {
		t.Fatal(err)
	}
	page, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_list_conversation_messages", Arguments: map[string]any{"source_id": claim.Source, "conversation_type": claim.Ref.Type, "conversation_id": claim.Ref.ID, "since": claim.Since, "until": claim.Until, "order": claim.Order, "limit": 1}})
	if err != nil || page.IsError {
		t.Fatal("MCP archive message output failed validation", err)
	}
	resource, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "zalo://archives/" + token})
	// Assert: resource contains the exact authenticated archived identity, not corpus fallback.
	if err != nil || len(resource.Contents) != 1 {
		t.Fatal("MCP archive resource failed", err)
	}
	var value map[string]any
	if json.Unmarshal([]byte(resource.Contents[0].Text), &value) != nil || value["archive_row_id"] != id || value["quote_anchor_eligible"] != false {
		t.Fatal("MCP resource changed source identity")
	}
}

func TestArchiveExcerptBoundsUnicodeAndBindsOriginalReadPage(t *testing.T) {
	// Arrange: long multibyte text and private original page position.
	ctx := context.Background()
	library, err := mobilebackup.NewArchiveLibraryStore(filepath.Join(t.TempDir(), "library"), 8<<20)
	if err != nil {
		t.Fatal(err)
	}
	defer library.Close()
	p := &membershipPort{library: library}
	ref := domain.ConversationRef{Type: "direct", ID: "12"}
	token := archiveReadToken{Version: 1, Kind: "cursor", Account: strings.Repeat("a", 64), Source: "00000000-0000-4000-8000-000000000001", Digest: strings.Repeat("b", 64), Ref: ref, Since: "2026-09-26T00:00:00Z", Until: "2026-09-27T00:00:00Z", Order: "desc", Position: &archivePosition{RowID: 99, Snapshot: true, Descending: true}}
	record := mobilebackup.SnapshotRecord{ArchiveRowID: "ar:" + token.Source + ":0:98", Conversation: ref, Text: strings.Repeat("я🙂", 1500)}
	// Act: produce bounded excerpt and decode only the authenticated resource claim.
	value, err := p.archiveExcerpt(ctx, record, token, 20)
	if err != nil {
		t.Fatal(err)
	}
	text := value["text"].(string)
	sealed := strings.TrimPrefix(value["resource_uri"].(string), "zalo://archives/")
	saved, err := p.openArchiveRead(ctx, "resource", sealed)
	// Assert: Unicode remains intact, with exact source/page identity and no plaintext body in token.
	if err != nil || !utf8.ValidString(text) || utf8.RuneCountInString(text) != 2048 || value["text_truncated"] != true || saved.Target != record.ArchiveRowID || saved.Limit != 20 || saved.Position.RowID != 99 || saved.Source != token.Source || strings.Contains(sealed, "я") {
		t.Fatal("archive excerpt or read capability changed", err)
	}
	record.Text = "short text"
	short, err := p.archiveExcerpt(ctx, record, token, 20)
	if err != nil || short["text_truncated"] != false || short["resource_uri"] != nil || short["text"] != record.Text {
		t.Fatal("short text received unnecessary resource", err)
	}
}

func testArchiveRecalledResources(t *testing.T, owner *membershipPort, sourceID string) {
	t.Helper()
	// Arrange: an authenticated resource claim deliberately targets a recalled row.
	// Its undo is outside the requested interval, in both typed source files.
	ctx := context.Background()
	binding, err := owner.store.ArchiveAccountKey(ctx)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := owner.library.PreservationStatus(ctx, sourceID, binding)
	if err != nil {
		t.Fatal(err)
	}
	server, err := mcpserver.NewWithControl(owner.store, owner)
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "archive-recall-resource-test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	for index, kind := range []string{"direct", "group"} {
		token := archiveReadToken{Version: 1, Kind: "resource", Account: binding, Source: sourceID, Digest: receipt.Source.Digest, Ref: domain.ConversationRef{Type: kind, ID: "12"}, Since: "2026-09-26T00:00:00Z", Until: "2026-09-26T01:00:00Z", Order: "asc", Target: fmt.Sprintf("ar:%s:%d:1", sourceID, index), Limit: 1}
		sealed, err := owner.sealArchiveRead(ctx, "resource", token)
		if err != nil {
			t.Fatal(err)
		}
		// Act: replay the same authenticated capability through owner and public MCP.
		value, ownerErr := owner.Call(ctx, "read_archive_resource", map[string]any{"token": sealed})
		result, resourceErr := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "zalo://archives/" + sealed})
		// Assert: no recalled plaintext or resource contents, despite valid ownership.
		if ownerErr == nil || value != nil || resourceErr == nil || result != nil {
			t.Fatal("recalled resource bypassed whole-source visibility")
		}
	}
}
