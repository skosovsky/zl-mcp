package mcpserver

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func poisonJournalHead(t *testing.T, h *recoveryReadyHarness, mutate func(map[string]any)) map[string]any {
	t.Helper()
	var id int64
	var raw []byte
	if err := h.store.DB.QueryRow("SELECT id,payload FROM event_deliveries WHERE subscription_id=? ORDER BY id LIMIT 1", h.business).
		Scan(&id, &raw); err != nil {
		t.Fatal(err)
	}
	var envelope map[string]any
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	mutate(envelope)
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.DB.Exec("UPDATE event_deliveries SET payload=? WHERE id=?", raw, id); err != nil {
		t.Fatal(err)
	}
	return envelope
}
func assertPoisonGapAckReleasesNextSDKEntry(t *testing.T, h *recoveryReadyHarness, nextIdentity string, activity bool) {
	t.Helper()
	var before []byte
	if err := h.store.DB.QueryRow("SELECT payload FROM event_deliveries WHERE subscription_id=? ORDER BY id LIMIT 1", h.business).
		Scan(&before); err != nil {
		t.Fatal(err)
	}
	// Act: the actual SDK/HTTP path must normalize one corrupt head before page
	// DTO validation and budgeting, preserving a verifiable ordered receipt.
	legacy, err := defaultJournal(h)
	if err != nil || legacy["blocked_on_delivery"] != true || len(legacy["events"].([]any)) != 0 {
		t.Fatal("legacy behavior changed", err)
	}
	result := h.tool(
		"zalo_read_subscription_events",
		map[string]any{"subscription_id": h.business, "readiness_mode": "persisted", "limit": 1},
	)
	entries := result["events"].([]any)
	if len(entries) != 1 || result["has_more"] != true {
		t.Fatal("poison head blocked ordered paging")
	}
	entry := entries[0].(map[string]any)
	if entry["processing_state"] != "gap" || entry["event"] != nil || entry["gap_reason"] != "invalid_payload" ||
		entry["delivery_state"] != "pending" {
		t.Fatal("malformed event was not an explicit truthful gap")
	}
	receipt, ok := entry["receipt"].(string)
	if !ok || receipt == "" {
		t.Fatal("poison gap lacks receipt")
	}
	encoded, err := json.Marshal(result)
	if err != nil || len(encoded) > 64<<10 || strings.Contains(string(encoded), "poison-padding-marker") {
		t.Fatal("poison payload leaked or exceeded response budget", err)
	}
	// Assert: authorized explicit gap acknowledgement frees the following valid
	// event in the same checkpoint without overwriting unfinished transport bytes.
	h.tool("zalo_ack_subscription_events", map[string]any{"subscription_id": h.business, "receipt": receipt})
	var after []byte
	var transport string
	if err := h.store.DB.QueryRow("SELECT payload,state FROM event_deliveries WHERE subscription_id=? ORDER BY id LIMIT 1", h.business).
		Scan(&after, &transport); err != nil ||
		!bytes.Equal(before, after) ||
		transport != "pending" {
		t.Fatal("gap ack modified unfinished transport bytes/state", err)
	}
	next := h.tool(
		"zalo_read_subscription_events",
		map[string]any{"subscription_id": h.business, "readiness_mode": "persisted", "limit": 1},
	)
	nextEntries := next["events"].([]any)
	if len(nextEntries) != 1 {
		t.Fatal("valid following event unavailable after gap acknowledgement")
	}
	valid := nextEntries[0].(map[string]any)
	if valid["processing_state"] != "ready" || valid["gap_reason"] != nil || valid["delivery_state"] != "pending" {
		t.Fatal("valid following event lost readiness")
	}
	event := valid["event"].(map[string]any)
	data := event["data"].(map[string]any)
	identityField := "message_id"
	if activity {
		identityField = "source_id"
	}
	if data[identityField] != nextIdentity {
		t.Fatal("wrong following event")
	}
	h.assertNoNativeActions()
}

func TestPersistedSDKPoisonNestedMessageHeadBecomesGapBeforeBudget(t *testing.T) {
	for _, large := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "missing-required-data", true: "100k-invalid-nested-data"}[large],
			func(t *testing.T) {
				// Arrange: retained valid envelopes were committed by normal fanout; corruption
				// affects only the first serialized payload, never target identity or order.
				h := newRecoveryReadyHarness(t)
				h.put("corrupt-head")
				h.put("valid-next")
				poisonJournalHead(t, h, func(envelope map[string]any) {
					data := map[string]any{}
					if large {
						data["padding"] = strings.Repeat("poison-padding-marker", 5000)
					}
					envelope["data"] = data
				})
				assertPoisonGapAckReleasesNextSDKEntry(t, h, "valid-next", false)
			},
		)
	}
}

func TestPersistedSDKSchemaValidDomainInvalidActivityHeadBecomesReceiptedGap(t *testing.T) {
	// Arrange: a reaction subscription, real activity validation/storage and fanout.
	h := newRecoveryReadyHarness(t)
	h.params["name"] = domain.ReactionObservedV1
	delete(h.params["arguments"].(map[string]any), "first_incoming_only")
	subscribed := httpRPC(t, h.endpoint.URL+"/mcp", h.token, "events/subscribe", h.params)
	h.business = subscribed["id"].(string)
	reaction := ":)"
	for _, id := range []string{"invalid-domain-head", "valid-activity-next"} {
		observation := domain.Activity{
			Family:           "reaction",
			SchemaVersion:    1,
			ConversationType: "direct",
			ConversationID:   "peer",
			ActorID:          "peer",
			OccurredAt:       time.Now(),
			Direction:        "incoming",
			SourceID:         id,
			IdentityBasis:    "upstream_action_id",
			Source:           "live",
			Reaction: &domain.ReactionObservation{
				TargetMessageIDs: []string{},
				Reaction:         &reaction,
				Action:           "add",
			},
		}
		if err := h.store.PutActivity(t.Context(), observation); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.store.FanoutActivityEvents(
		t.Context(),
		time.Now(),
		h.worker.Policy,
		h.worker.Encoder.EncodeActivity,
	); err != nil {
		t.Fatal(err)
	}
	envelope := poisonJournalHead(t, h, func(envelope map[string]any) {
		envelope["data"].(map[string]any)["occurred_at"] = "0001-01-01T00:00:00Z"
	})
	schema, err := contracts.Compile("reaction_v1_event", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(envelope); err != nil {
		t.Fatal("witness must pass public JSON Schema", err)
	}
	raw, err := json.Marshal(envelope["data"])
	if err != nil {
		t.Fatal(err)
	}
	var activity domain.Activity
	if err = json.Unmarshal(raw, &activity); err != nil {
		t.Fatal(err)
	}
	if activity.Validate() == nil {
		t.Fatal("witness must violate domain activity decoder semantics")
	}
	// Act / Assert: per-entry DTO failure is a gap, not a poisoned whole page.
	assertPoisonGapAckReleasesNextSDKEntry(t, h, "valid-activity-next", true)
}

//nolint:gocognit // One ordered AAA witness checks published schema, DTO, actual SDK error and unchanged durable state.
func TestPersistedSDKValidOversizedEntryRemainsBudgetErrorWithoutGapOrAck(t *testing.T) {
	// Arrange: canonical v3 subscription and normal committed envelope identity.
	h := newRecoveryReadyHarness(t)
	h.put("valid-large-head")
	envelope := poisonJournalHead(t, h, func(envelope map[string]any) {
		data := envelope["data"].(map[string]any)
		elements := []any{}
		for range 16 {
			elements = append(
				elements,
				map[string]any{
					"kind":            "image",
					"label":           strings.Repeat("界", 512),
					"file_name":       strings.Repeat("界", 256),
					"mime_type":       nil,
					"size_bytes":      nil,
					"duration_ms":     nil,
					"width":           nil,
					"height":          nil,
					"sticker_id":      nil,
					"bytes_available": false,
				},
			)
		}
		data["content"] = map[string]any{
			"kind":         "mixed",
			"native_type":  "chat.photo",
			"summary":      "Synthetic image metadata",
			"availability": "metadata_only",
			"reason":       "metadata_limited",
			"elements":     elements,
			"text_role":    "author",
		}
		data["content_resource_uri"] = "zalo://conversations/direct/peer/messages/valid-large-head/content"
	})
	schema, err := contracts.Compile("conversation_v3_event", "delivery")
	if err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(envelope); err != nil {
		t.Fatal("large control must pass published schema", err)
	}
	before, err := json.Marshal(envelope)
	if err != nil || len(before) <= 28<<10 {
		t.Fatal("control must exceed owner page budget", err)
	}
	var decoded eventaccess.Event
	if err = json.Unmarshal(before, &decoded); err != nil {
		t.Fatal("control must pass domain DTO decode", err)
	}
	// Act: repeat the actual HTTP SDK call, proving the head never mutates into a
	// gap merely because the otherwise valid representation is too large.
	for range 2 {
		result, callErr := h.client.CallTool(
			t.Context(),
			&mcp.CallToolParams{
				Name:      "zalo_read_subscription_events",
				Arguments: map[string]any{"subscription_id": h.business, "readiness_mode": "persisted", "limit": 1},
			},
		)
		if callErr != nil || !result.IsError || len(result.Content) != 1 {
			t.Fatal("oversized valid entry must return a bounded tool error", callErr)
		}
		content, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatal("missing structured error text")
		}
		var failure map[string]any
		if err = json.Unmarshal([]byte(content.Text), &failure); err != nil || failure["code"] != "RESPONSE_TOO_LARGE" {
			t.Fatal("valid size error was replaced with invalid-payload gap", err)
		}
	}
	// Assert: no receipt/ack advance, silent truncation, or false delivery result.
	var retained []byte
	var state string
	if err = h.store.DB.QueryRow("SELECT payload,state FROM event_deliveries WHERE subscription_id=? ORDER BY id LIMIT 1", h.business).
		Scan(&retained, &state); err != nil ||
		!bytes.Equal(before, retained) ||
		state != "pending" {
		t.Fatal("valid oversized outbox bytes/state changed", err)
	}
	var ack int64
	if err = h.store.DB.QueryRow("SELECT COALESCE(sum(ack_id),0) FROM event_processing WHERE subscription_id=?", h.business).
		Scan(&ack); err != nil ||
		ack != 0 {
		t.Fatal("oversized entry advanced processing checkpoint", err)
	}
	legacy, err := defaultJournal(h)
	if err != nil || legacy["blocked_on_delivery"] != true {
		t.Fatal("legacy behavior changed", err)
	}
	h.assertNoNativeActions()
}
