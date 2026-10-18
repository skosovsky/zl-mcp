package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

//nolint:gocognit // One AAA transport scenario keeps native dispatch counts and post-revocation status assertions together.
func TestPublicMessageActionsThroughHTTPAndNativeStdioBridge(t *testing.T) {
	// Arrange: only upstream is synthetic; authority, SQLite, owner and SDK are real.
	binary := filepath.Join(t.TempDir(), "zl-mcp")
	build := exec.CommandContext(t.Context(), "go", "build", "-o", binary, "./cmd/zl-mcp")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	for _, mode := range []string{"http", "stdio"} {
		t.Run(mode, func(t *testing.T) {
			port := publicActionPort(t)
			source := port.actions.(*actionProbeSource)
			dir := t.TempDir()
			token := strings.Repeat("t", 64)
			protocol, err := events.NewSubscriptionManager(port.store, dir)
			if err != nil {
				t.Fatal(err)
			}
			handler, err := mcpserver.NewHTTP(token, mcpReadPorts(port.store, port), protocol)
			if err != nil {
				t.Fatal(err)
			}
			endpoint := httptest.NewServer(handler)
			defer endpoint.Close()
			var transport mcp.Transport = &mcp.StreamableClientTransport{Endpoint: endpoint.URL + "/mcp", HTTPClient: &http.Client{Transport: bearerTransport{token: token, base: http.DefaultTransport}}, DisableStandaloneSSE: true, MaxRetries: -1}
			if mode == "stdio" {
				tokenPath := filepath.Join(dir, "token")
				if err = os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
					t.Fatal(err)
				}
				configPath := filepath.Join(dir, "config.toml")
				config := fmt.Sprintf(
					"state_dir = %q\n[collection]\nmode = \"all\"\n[mcp]\nlisten = %q\ntoken_file = %q\n",
					filepath.Join(dir, "unused-state"),
					strings.TrimPrefix(endpoint.URL, "http://"),
					tokenPath,
				)
				if err = os.WriteFile(configPath, []byte(config), 0600); err != nil {
					t.Fatal(err)
				}
				transport = &mcp.CommandTransport{
					Command: exec.CommandContext(t.Context(), binary, "-config", configPath, "serve"),
				}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
			defer cancel()
			session, err := mcp.NewClient(&mcp.Implementation{Name: "action-parity", Version: "1"}, nil).
				Connect(ctx, transport, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			readID := "a0000000-0000-4000-8000-000000000001"
			reactionID := "a0000000-0000-4000-8000-000000000002"
			readArgs := map[string]any{
				"request_id":        readID,
				"conversation_type": "direct",
				"conversation_id":   "peer",
				"message_ids":       []string{"123"},
			}
			reactionArgs := map[string]any{
				"request_id":        reactionID,
				"conversation_type": "direct",
				"conversation_id":   "peer",
				"message_id":        "123",
				"action":            "set",
				"reaction":          "heart",
			}
			// Act: both writes and UUID replay use the actual public transport path.
			for _, call := range []struct {
				name string
				args map[string]any
			}{{"zalo_mark_messages_read", readArgs}, {"zalo_set_message_reaction", reactionArgs}, {"zalo_set_message_reaction", reactionArgs}} {
				result, callErr := session.CallTool(ctx, &mcp.CallToolParams{Name: call.name, Arguments: call.args})
				if callErr != nil || result.IsError {
					t.Fatal(call.name, callErr, result)
				}
				raw, _ := json.Marshal(result.StructuredContent)
				var value map[string]any
				if json.Unmarshal(raw, &value) != nil || value["status"] != "accepted" {
					t.Fatal("missing accepted result", string(raw))
				}
				schema, schemaErr := contracts.Compile(call.name, "output")
				if schemaErr != nil || schema.Validate(value) != nil {
					t.Fatal("output contract", schemaErr)
				}
			}
			// Assert: one receipt and exactly clear+add, without replayed writes.
			if source.calls != 3 {
				t.Fatal("duplicated or missing dispatch", source.calls)
			}

			// Act: ordinary browsing/search/journal discovery and resource reads.
			for _, read := range []struct {
				name string
				args map[string]any
			}{
				{"zalo_search_conversation_messages", map[string]any{"query": "synthetic"}},
				{"zalo_list_conversation_messages", map[string]any{"conversation_type": "direct", "conversation_id": "peer"}},
				{"zalo_list_event_subscriptions", map[string]any{}},
			} {
				result, readErr := session.CallTool(ctx, &mcp.CallToolParams{Name: read.name, Arguments: read.args})
				if readErr != nil || result.IsError {
					t.Fatal(read.name, readErr, result)
				}
			}
			if _, readErr := session.ReadResource(
				ctx,
				&mcp.ReadResourceParams{URI: "zalo://catalog/reactions"},
			); readErr != nil {
				t.Fatal(readErr)
			}
			assertEventJournalAckDoesNotAct(t, port, session)
			// Assert: active action grants do not make ordinary reads create intents.
			var intents int
			err = port.store.DB.QueryRowContext(ctx, "SELECT count(*) FROM message_action_operations").Scan(&intents)
			if err != nil || intents != 2 || source.calls != 3 {
				t.Fatal("read caused action", err, intents, source.calls)
			}
			port.mu.Lock()
			port.readChats = nil
			port.reactionChats = nil
			port.actions = nil
			port.mu.Unlock()
			// Act / Assert: saved status works after grants/session removal, still no write.
			result, err := session.CallTool(
				ctx,
				&mcp.CallToolParams{
					Name:      "zalo_get_message_action_status",
					Arguments: map[string]any{"request_id": reactionID},
				},
			)
			if err != nil || result.IsError || source.calls != 3 {
				t.Fatal("status required source or wrote", err, result)
			}
		})
	}
}

func assertEventJournalAckDoesNotAct(t *testing.T, port *membershipPort, session *mcp.ClientSession) {
	t.Helper()
	// Arrange: a synthetic delivered event for a fresh unread incoming message.
	ctx := t.Context()
	at := time.Now().UTC()
	subscription := storage.EventSubscription{
		ID:        "synthetic-actions-sub",
		Principal: "local:" + strconv.Itoa(os.Getuid()),
		Profile:   domain.ConversationMessageCreatedV2,
		Scope:     "all",
		Callback:  "https://receiver.example/events",
		Secret:    "synthetic-event-secret",
	}
	revision, err := port.store.SubscriptionRevision(ctx, subscription.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = port.store.ActivateSubscription(ctx, subscription, revision, at); err != nil {
		t.Fatal(err)
	}
	message := domain.Message{
		Conversation:            domain.ConversationRef{Type: "direct", ID: "peer"},
		ID:                      "789",
		SenderID:                "peer",
		Direction:               "incoming",
		Source:                  "live",
		SentAt:                  at,
		Text:                    "synthetic unread incoming",
		ProtocolClientMessageID: "901",
		SeenMetadata: &domain.SeenMetadata{
			ClientMessageID: "901",
			RecipientID:     "owner",
			MessageType:     "webchat",
			Timestamp:       strconv.FormatInt(at.UnixMilli(), 10),
		},
	}
	if err = port.store.Put(ctx, message); err != nil {
		t.Fatal(err)
	}
	encoder, err := events.NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	if _, fanoutErr := port.store.FanoutProfileEvents(
		ctx,
		at,
		storage.DefaultDeliveryPolicy(),
		encoder.EncodeProfile,
	); fanoutErr != nil {
		t.Fatal("synthetic fanout", fanoutErr)
	}
	delivery, err := port.store.ClaimDelivery(ctx, at.Add(time.Second), storage.DefaultDeliveryPolicy())
	if err != nil || delivery == nil {
		t.Fatal("synthetic delivery", err)
	}
	if err = port.store.FinishDelivery(
		ctx,
		*delivery,
		storage.DeliveryOutcome{State: "delivered", Reason: "accepted"},
		at.Add(time.Second),
	); err != nil {
		t.Fatal(err)
	}
	// Act: read the actual public journal and ACK its opaque returned receipt.
	result, err := session.CallTool(
		ctx,
		&mcp.CallToolParams{
			Name:      "zalo_read_subscription_events",
			Arguments: map[string]any{"subscription_id": subscription.ID},
		},
	)
	if err != nil || result.IsError {
		t.Fatal("journal read", err, result)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var page struct {
		Events []struct {
			Receipt string `json:"receipt"`
			Event   struct {
				Data struct {
					MessageID string `json:"message_id"`
				} `json:"data"`
			} `json:"event"`
		} `json:"events"`
	}
	if json.Unmarshal(raw, &page) != nil || len(page.Events) != 1 || page.Events[0].Receipt == "" ||
		page.Events[0].Event.Data.MessageID != "789" {
		t.Fatal("missing actual receipt")
	}
	result, err = session.CallTool(
		ctx,
		&mcp.CallToolParams{
			Name:      "zalo_ack_subscription_events",
			Arguments: map[string]any{"subscription_id": subscription.ID, "receipt": page.Events[0].Receipt},
		},
	)
	// Assert: ACK succeeded; the enclosing test checks unchanged native/intent counts
	// while both read and reaction grants and the native source remain active.
	if err != nil || result.IsError {
		t.Fatal("journal ack", err, result)
	}
}
