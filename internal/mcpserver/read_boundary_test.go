package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	storeadapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type observedReads struct {
	*storeadapter.Reader
	pages    atomic.Int32
	trims    atomic.Int32
	messages atomic.Int32
	deny     atomic.Bool
}

func (r *observedReads) GroupPage(ctx context.Context, input search.GroupRequest) (*search.Page, error) {
	r.pages.Add(1)
	page, err := r.Reader.GroupPage(ctx, input)
	if err != nil {
		return nil, err
	}
	return search.NewPage(page.Hits, page.NextCursor, page.SnapshotAt, func(out *search.Page, n int) error {
		r.trims.Add(1)
		if err := page.Shorten(n); err != nil {
			return err
		}
		out.Hits = page.Hits
		out.NextCursor = page.NextCursor
		out.SnapshotAt = page.SnapshotAt
		return nil
	}), nil
}
func (r *observedReads) Message(ctx context.Context, input conversations.MessageRequest) (domain.Message, error) {
	r.messages.Add(1)
	if r.deny.Load() {
		return domain.Message{}, &domain.Error{Code: "PERMISSION_DENIED", Message: "Current policy revoked this read."}
	}
	return r.Reader.Message(ctx, input)
}
func decodeReadResult(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	if result == nil || result.IsError {
		t.Fatalf("failed read: %+v", result)
	}
	body, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func TestReadBoundaryShortensNativePageOnceAndRechecksResourceAccess(t *testing.T) {
	// Arrange: three valid hits exceed the wire budget due to legitimate metadata.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	owner, err := storage.Open(ctx, filepath.Join(dir, "messages.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = owner.ReplaceCatalog(ctx, []domain.Group{{ID: "g", Name: strings.Repeat("N", 14000)}}); err != nil {
		t.Fatal(err)
	}
	instant := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := int64(0); i < 3; i++ {
		if err = owner.Put(ctx, domain.Message{GroupID: "g", ID: fmt.Sprint(int64(9007199254740993) + i), SenderID: "peer", SentAt: instant.Add(time.Duration(i) * time.Minute), Text: "search Cà phê 🍀", Source: "live"}); err != nil {
			t.Fatal(err)
		}
	}
	reads := &observedReads{Reader: storeadapter.New(owner)}
	ports := ReadPorts{Search: &search.Handler{Reads: reads}, Conversations: &conversations.Handler{Reads: reads}}
	server, err := New(owner, dir, ports)
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "read-boundary", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	// Act: the real wire budget invokes continuation shortening, not another FTS.
	result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_search_messages", Arguments: map[string]any{"query": "search", "limit": 3}})
	if err != nil {
		t.Fatal(err)
	}
	first := decodeReadResult(t, result)
	wire, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: preserve precise IDs and expose the shortened native cursor.
	items := first["messages"].([]any)
	if reads.pages.Load() != 1 || reads.trims.Load() != 1 || len(items) != 2 || len(wire) > 64<<10 || first["has_more"] != true || items[0].(map[string]any)["message_id"] != "9007199254740995" || items[1].(map[string]any)["message_id"] != "9007199254740994" {
		t.Fatal(reads.pages.Load(), reads.trims.Load(), len(items), len(wire), first["has_more"])
	}
	// Arrange: a backdated row added after the first request must not enter its snapshot.
	if err = owner.Put(ctx, domain.Message{GroupID: "g", ID: "late", SenderID: "peer", SentAt: instant.Add(-time.Minute), Text: "search late", Source: "live"}); err != nil {
		t.Fatal(err)
	}
	// Act
	result, err = cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_search_messages", Arguments: map[string]any{"query": "search", "limit": 3, "cursor": first["next_cursor"]}})
	if err != nil {
		t.Fatal(err)
	}
	second := decodeReadResult(t, result)
	// Assert: the one omitted original ID remains, and the native high-water mark holds.
	remaining := second["messages"].([]any)
	if len(remaining) != 1 || remaining[0].(map[string]any)["message_id"] != "9007199254740993" || first["snapshot_at"] != second["snapshot_at"] || reads.pages.Load() != 2 {
		t.Fatal(second, reads.pages.Load())
	}
	// Arrange: resources use the same read port and current policy independently.
	uri := "zalo://groups/g/messages/9007199254740995"
	// Act
	resource, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	reads.deny.Store(true)
	_, denied := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	// Assert: prior search/resource success cannot preserve revoked authorization.
	if err != nil || resource.Contents[0].Text != "search Cà phê 🍀" || denied == nil || reads.messages.Load() != 2 {
		t.Fatal(resource, err, denied, reads.messages.Load())
	}
}
