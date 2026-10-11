package mcpserver

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	adapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type invocationReader struct {
	*adapter.Reader
	group, conversation, context, typedContext atomic.Int32
}

func (r *invocationReader) GroupPage(ctx context.Context, in search.GroupRequest) (*search.Page, error) {
	r.group.Add(1)
	return r.Reader.GroupPage(ctx, in)
}
func (r *invocationReader) ConversationPage(ctx context.Context, in search.ConversationRequest) (*search.Page, error) {
	r.conversation.Add(1)
	return r.Reader.ConversationPage(ctx, in)
}
func (r *invocationReader) GroupContext(ctx context.Context, in conversations.ContextRequest) (conversations.GroupContext, error) {
	r.context.Add(1)
	return r.Reader.GroupContext(ctx, in)
}
func (r *invocationReader) ConversationContext(ctx context.Context, in conversations.ContextRequest) (conversations.ConversationContext, error) {
	r.typedContext.Add(1)
	return r.Reader.ConversationContext(ctx, in)
}

func TestRuntimeSDKDispatchAndSingleAdmission(t *testing.T) {
	// Arrange: real SDK, borrowed store and observable application reads. Triggers
	// exist only in this synthetic test DB and count actual admission statements.
	ctx := context.Background()
	owner, err := storage.Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	for _, ddl := range []string{
		"CREATE TABLE test_admissions(n INTEGER)",
		"CREATE TRIGGER test_admission_insert AFTER INSERT ON read_budget BEGIN INSERT INTO test_admissions VALUES(1); END",
		"CREATE TRIGGER test_admission_update AFTER UPDATE ON read_budget BEGIN INSERT INTO test_admissions VALUES(1); END",
	} {
		if _, err = owner.DB.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	if err = owner.Put(ctx, domain.Message{GroupID: "g", ID: "9007199254740993", SenderID: "p", Text: "synthetic Cà phê 🍀", SentAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Source: "live"}); err != nil {
		t.Fatal(err)
	}
	reader := &invocationReader{Reader: adapter.New(owner)}
	server, err := New(owner, t.TempDir(), ReadPorts{Search: &search.Handler{Reads: reader}, Conversations: &conversations.Handler{Reads: reader}, Authority: reader})
	if err != nil {
		t.Fatal(err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-runtime-test", Version: "1"}, nil)
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	calls := []struct {
		name   string
		args   map[string]any
		denied bool
	}{
		{"zalo_search_messages", map[string]any{"query": "synthetic", "group_id": "g", "limit": "1"}, false},
		{"zalo_search_conversation_messages", map[string]any{"query": "synthetic", "conversation_type": "group", "conversation_id": "g"}, false},
		{"zalo_get_message_context", map[string]any{"group_id": "g", "message_id": "9007199254740993", "before": "0", "after": 0}, false},
		{"zalo_get_conversation_message_context", map[string]any{"conversation_type": "group", "conversation_id": "g", "message_id": "9007199254740993", "before": 0, "after": 0}, false},
		{"zalo_search_messages", map[string]any{"query": "synthetic", "since": "invalid-format"}, true},
		{"zalo_search_messages", map[string]any{"query": "synthetic", "group_id": "forbidden"}, true},
	}
	// Act.
	for _, call := range calls {
		result, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
		if err != nil {
			t.Fatal(err)
		}
		if result.IsError != call.denied {
			t.Fatal(call.name, result)
		}
	}
	resource, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: "zalo://groups/g/messages/9007199254740993"})
	// Assert: each accepted tool selects once; rejected calls select nothing;
	// resource has its own one admission and full opaque-ID access.
	if err != nil || len(resource.Contents) != 1 || resource.Contents[0].Text != "synthetic Cà phê 🍀" {
		t.Fatal(resource, err)
	}
	if reader.group.Load() != 1 || reader.conversation.Load() != 1 || reader.context.Load() != 1 || reader.typedContext.Load() != 1 {
		t.Fatal(reader.group.Load(), reader.conversation.Load(), reader.context.Load(), reader.typedContext.Load())
	}
	var admissions int
	if err = owner.DB.QueryRowContext(ctx, "SELECT count(*) FROM test_admissions").Scan(&admissions); err != nil || admissions != len(calls)+1 {
		t.Fatal(admissions, err)
	}
}
