package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/app/archives"
	"github.com/skosovsky/zl-mcp/internal/app/contentread"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:gocognit // Keep actual SDK paging, resource compatibility and post-page recall in one ordered AAA scenario.
func TestToolsOnlyContentReconstructsRetainedUnicodeAndRechecksRecall(t *testing.T) {
	// Arrange: actual SDK, retained Unicode text, zero native command calls required.
	ctx := t.Context()
	store, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: "peer"}
	text := strings.Repeat("\x00", 6000) + strings.Repeat("👩🏽‍💻🙂", 2000)
	if err = store.Put(
		ctx,
		domain.Message{
			Conversation: ref,
			ID:           "retained",
			SenderID:     "peer",
			SentAt:       time.Now(),
			Text:         text,
			Source:       "replay",
		},
	); err != nil {
		t.Fatal(err)
	}
	server, err := New(migrationReadPorts(store))
	if err != nil {
		t.Fatal(err)
	}
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "tools-only-content", Version: "1"}, nil).
		Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	uri := "zalo://conversations/direct/peer/messages/retained"
	args := map[string]any{"resource_uri": uri, "limit": 8192}
	var reconstructed strings.Builder
	firstCursor := ""
	// Act: exclusively tools/call reads reconstruct the text; resources are not needed.
	for {
		result, e := client.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_read_message_content", Arguments: args})
		if e != nil || result.IsError {
			t.Fatalf("content call: %v %+v", e, result)
		}
		wire, wireErr := json.Marshal(result)
		if wireErr != nil || len(wire) > 64<<10 {
			t.Fatalf("full SDK response exceeds 64 KiB: bytes=%d error=%v", len(wire), wireErr)
		}
		raw, e := json.Marshal(result.StructuredContent)
		if e != nil {
			t.Fatal(e)
		}
		var page contentread.Page
		if e = json.Unmarshal(raw, &page); e != nil {
			t.Fatal(e)
		}
		if len(raw) > contentread.MaxPageBytes {
			t.Fatal("oversized page")
		}
		reconstructed.WriteString(page.Text)
		if !page.HasMore {
			break
		}
		if firstCursor == "" {
			firstCursor = *page.NextCursor
		}
		args["cursor"] = *page.NextCursor
	}
	// Assert: exact Unicode reconstruction and legacy resource compatibility.
	if reconstructed.String() != text {
		t.Fatal("missing or duplicate text")
	}
	resource, e := client.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if e != nil || resource.Contents[0].Text != text {
		t.Fatal("existing resource failed", e)
	}
	if err = store.DeleteConversation(ctx, ref, "retained"); err != nil {
		t.Fatal(err)
	}
	args["cursor"] = firstCursor
	denied, e := client.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_read_message_content", Arguments: args})
	if e != nil || !denied.IsError {
		t.Fatal("recall failed to revoke continuation", e)
	}
}

// contentArchiveFixture represents the archive capability owner, not a corpus row.
// It accepts one issued token and rechecks its access state on every page.
type contentArchiveFixture struct {
	token, text string
	denied      bool
	calls       int
}

func (f *contentArchiveFixture) Sources(context.Context) (archives.Sources, error) {
	return archives.Sources{}, errors.New("unexpected sources")
}
func (f *contentArchiveFixture) Conversations(context.Context, archives.DirectoryRequest) (archives.Directory, error) {
	return archives.Directory{}, errors.New("unexpected directory")
}
func (f *contentArchiveFixture) Browse(context.Context, archives.BrowseRequest) (archives.Browse, error) {
	return archives.Browse{}, errors.New("unexpected browse")
}
func (f *contentArchiveFixture) Record(_ context.Context, token string) (archives.Record, error) {
	f.calls++
	if f.denied || token != f.token {
		return archives.Record{}, domain.Invalid("unavailable capability")
	}
	return archives.Record{ArchiveRowID: "ar:source:0:1", ZaloMessageID: nil, Text: f.text, TextKind: "rich"}, nil
}

//nolint:gocognit // Keep archive capability paging, provenance and current-access revocation in one ordered AAA scenario.
func TestToolsOnlyContentReadsIssuedArchiveCapabilityWithoutMessageIdentity(t *testing.T) {
	// Arrange: the owner exposes only an issued opaque archive capability; genuine Zalo ID absent.
	ctx := t.Context()
	owner, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	fixture := &contentArchiveFixture{
		token: strings.Repeat("issued-capability-", 300),
		text:  strings.Repeat("\x00", 6000) + strings.Repeat("存档🙂", 3000),
	}
	ports := migrationReadPorts(owner)
	ports.Archives = &archives.Handler{Reads: fixture}
	server, err := New(ports)
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "archive-tools-only", Version: "1"}, nil).
		Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	args := map[string]any{"resource_uri": "zalo://archives/" + fixture.token, "limit": 8192}
	var all strings.Builder
	firstCursor := ""
	// Act: reconstruct the exact archive projection without resource calls or fabricated message IDs.
	for {
		result, e := client.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_read_message_content", Arguments: args})
		if e != nil || result.IsError {
			t.Fatalf("archive: %v %+v", e, result)
		}
		wire, wireErr := json.Marshal(result)
		if wireErr != nil || len(wire) > 64<<10 {
			t.Fatalf("full archive response exceeds 64 KiB: %d %v", len(wire), wireErr)
		}
		raw, _ := json.Marshal(result.StructuredContent)
		var page contentread.Page
		if e = json.Unmarshal(raw, &page); e != nil {
			t.Fatal(e)
		}
		if page.Source != "archive" || page.TextProvenance != "saved_archive_projection" {
			t.Fatal("wrong provenance")
		}
		all.WriteString(page.Text)
		if !page.HasMore {
			break
		}
		if firstCursor == "" {
			firstCursor = *page.NextCursor
		}
		args["cursor"] = *page.NextCursor
	}
	// Assert: complete text, current owner invoked per page and revocation enforced.
	if all.String() != fixture.text || fixture.calls < 2 {
		t.Fatal("archive paging incomplete")
	}
	fixture.denied = true
	args["cursor"] = firstCursor
	denied, e := client.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_read_message_content", Arguments: args})
	if e != nil || !denied.IsError {
		t.Fatal("revoked archive capability accepted", e)
	}
}

//nolint:gocognit // One AAA scenario preserves native encoder URI, namespace collision, paging and legacy resource equality.
func TestToolsOnlyContentAcceptsActualLegacyEventURIAndPreservesProjection(t *testing.T) {
	// Arrange: a legacy event issues the URI; direct ID collision must never win.
	ctx := t.Context()
	owner, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(t.TempDir(), "state.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	group := domain.ConversationRef{Type: domain.ConversationGroup, ID: "peer/opaque%"}
	text := strings.Repeat("legacy 🙂 ", 1500)
	metadata := domain.LegacyContent()
	metadata.TextRole = "none"
	message := domain.Message{
		Conversation: group,
		GroupID:      group.ID,
		ID:           "message/opaque%",
		SenderID:     "sender",
		SentAt:       time.Now(),
		Text:         text,
		Source:       "replay",
		Content:      metadata,
	}
	if err = owner.Put(ctx, message); err != nil {
		t.Fatal(err)
	}
	direct := message
	direct.Conversation.Type = domain.ConversationDirect
	direct.GroupID = ""
	direct.Text = "wrong direct namespace"
	if err = owner.Put(ctx, direct); err != nil {
		t.Fatal(err)
	}
	encoder, err := events.NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := encoder.Encode("synthetic-event", message)
	if err != nil {
		t.Fatal(err)
	}
	var event struct {
		Data struct {
			URI string `json:"text_resource_uri"`
		} `json:"data"`
	}
	if err = json.Unmarshal(encoded, &event); err != nil || event.Data.URI == "" {
		t.Fatal("encoder URI unavailable", err)
	}
	server, err := New(migrationReadPorts(owner))
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "legacy-tools-only", Version: "1"}, nil).
		Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	args := map[string]any{"resource_uri": event.Data.URI, "limit": 8192}
	var reconstructed strings.Builder
	// Act: read all text solely through tools from the exact URI emitted by Events.
	for {
		result, e := client.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_read_message_content", Arguments: args})
		if e != nil || result.IsError {
			t.Fatalf("legacy URI rejected: %v %+v", e, result)
		}
		raw, _ := json.Marshal(result.StructuredContent)
		var page contentread.Page
		if e = json.Unmarshal(raw, &page); e != nil {
			t.Fatal(e)
		}
		if page.TextKind != "legacy_projection" {
			t.Fatal("legacy text misrepresented")
		}
		reconstructed.WriteString(page.Text)
		if !page.HasMore {
			break
		}
		args["cursor"] = *page.NextCursor
	}
	// Assert: >8k text preserved exactly and compatible with legacy resource semantics.
	if reconstructed.String() != text {
		t.Fatal("legacy text or namespace lost")
	}
	resource, err := client.ReadResource(ctx, &mcp.ReadResourceParams{URI: event.Data.URI})
	if err != nil || len(resource.Contents) != 1 || resource.Contents[0].Text != text {
		t.Fatal("legacy resource mismatch", err)
	}
}
