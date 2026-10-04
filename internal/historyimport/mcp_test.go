package historyimport_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestMCPHistoryImportDiscoveryStatusAndReads(t *testing.T) {
	// Arrange: real MCP transport and schemas around the durable backend/worker.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err = s.BindAccount(ctx, "synthetic-account"); err != nil {
		t.Fatal(err)
	}
	server, err := mcpserver.NewWithControl(s, &historyimport.Manager{Store: s})
	if err != nil {
		t.Fatal(err)
	}
	ct, st := mcp.NewInMemoryTransports()
	ss, err := server.Connect(ctx, st, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-history-client", Version: "1"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	tools, err := cs.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, tool := range tools.Tools {
		if tool.Name == "zalo_import_conversation_history" || tool.Name == "zalo_cancel_history_import" || tool.Name == "zalo_get_history_import_status" {
			seen[tool.Name] = true
			wantRead := tool.Name == "zalo_get_history_import_status"
			wantWorld := tool.Name == "zalo_import_conversation_history"
			if tool.Annotations == nil || tool.Annotations.ReadOnlyHint != wantRead || tool.Annotations.OpenWorldHint == nil || *tool.Annotations.OpenWorldHint != wantWorld || !tool.Annotations.IdempotentHint {
				t.Fatalf("incorrect history annotations: %s", tool.Name)
			}
		}
	}
	if len(seen) != 3 {
		t.Fatal("history capability missing")
	}
	args := map[string]any{"conversation_type": "group", "conversation_id": "synthetic-group", "request_id": "00000000-0000-4000-8000-000000000001", "since": "2026-09-01T00:00:00Z", "until": "2026-11-01T00:00:00Z"}
	// Act
	accepted, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_import_conversation_history", Arguments: args})
	// Assert
	if err != nil || accepted.IsError {
		t.Fatalf("import rejected: result=%+v error=%v", accepted, err)
	}
	body := accepted.StructuredContent.(map[string]any)
	id := body["operation_id"].(string)
	if body["state"] != "queued" || body["notification_policy"] != "none" {
		t.Fatalf("unexpected initial status: %#v", body)
	}
	workerCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() {
		done <- historyimport.Run(workerCtx, s, pageSource(func(ctx context.Context, ref domain.ConversationRef, cursor string, limit int) (domain.HistoryPage, error) {
			more := false
			return domain.HistoryPage{Messages: []domain.Message{message(ref, "historical-message")}, HasMore: &more}, nil
		}))
	}()
	defer func() { stop(); <-done }()
	awaitState(t, s, id, "completed")
	status, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_get_history_import_status", Arguments: map[string]any{"operation_id": id}})
	if err != nil || status.IsError || status.StructuredContent.(map[string]any)["history_complete"] != false {
		t.Fatal("status contract failed", err)
	}
	read, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_get_conversation_message_context", Arguments: map[string]any{"conversation_type": "group", "conversation_id": "synthetic-group", "message_id": "historical-message"}})
	if err != nil || read.IsError {
		t.Fatalf("imported context unavailable: result=%+v error=%v", read, err)
	}
	resource, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: storage.ConversationMessageURI(domain.ConversationRef{Type: "group", ID: "synthetic-group"}, "historical-message")})
	if err != nil || len(resource.Contents) != 1 || resource.Contents[0].Text != "Synthetic history" {
		t.Fatal("historical full-text resource failed", err)
	}
	again, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_import_conversation_history", Arguments: args})
	if err != nil || again.IsError || again.StructuredContent.(map[string]any)["operation_id"] != id {
		t.Fatal("MCP retry lost identity", err)
	}
	args["notification_policy"] = "notify"
	invalid, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "zalo_import_conversation_history", Arguments: args})
	if err != nil || !invalid.IsError {
		t.Fatal("notification override accepted", err)
	}
}
