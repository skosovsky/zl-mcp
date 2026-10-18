package store

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// normalizePersistedJournal exercises the same production projector over a
// synthetic owner page; real owner budget ordering is covered through the SDK.
func normalizePersistedJournal(value map[string]any) error {
	project, err := persistedJournalProjection()
	if err != nil {
		return err
	}
	entries, ok := value["events"].([]map[string]any)
	if !ok {
		return errors.New("invalid synthetic journal page")
	}
	for _, entry := range entries {
		if err := project(entry); err != nil {
			return err
		}
	}
	return nil
}

func projectionEvent(data map[string]any, name string) map[string]any {
	return map[string]any{
		"eventId":   "synthetic-event",
		"name":      name,
		"timestamp": "2026-10-18T00:00:00Z",
		"data":      data,
		"cursor":    nil,
	}
}
func projectionMessage() map[string]any {
	return projectionEvent(map[string]any{
		"schema_version":    2,
		"conversation_id":   "peer",
		"conversation_type": "direct",
		"conversation_name": nil,
		"direction":         "incoming",
		"first_incoming":    true,
		"message_id":        "synthetic-message",
		"sender_id":         "peer",
		"sender_name":       nil,
		"sent_at":           "2026-10-18T00:00:00Z",
		"text":              "Synthetic retained text",
		"text_truncated":    false,
		"text_resource_uri": nil,
	}, domain.ConversationMessageCreatedV2)
}
func projectionEntry(event any) map[string]any {
	return map[string]any{
		"event":            event,
		"processing_state": "ready",
		"delivery_state":   "sending",
		"gap_reason":       nil,
		"receipt":          "synthetic-authenticated-receipt",
	}
}
func projectionPage(entries ...map[string]any) map[string]any {
	return map[string]any{
		"subscription_id":     "synthetic-subscription",
		"readiness_mode":      domain.EventReadinessPersisted,
		"events":              entries,
		"has_more":            false,
		"blocked_on_delivery": false,
	}
}
func projectionActivity() map[string]any {
	return projectionEvent(map[string]any{
		"schema_version":    1,
		"family":            "reaction",
		"conversation_id":   "peer",
		"conversation_type": "direct",
		"conversation_name": nil,
		"actor_id":          "peer",
		"actor_name":        nil,
		"occurred_at":       "0001-01-01T00:00:00Z",
		"direction":         "incoming",
		"source_id":         "synthetic-reaction",
		"identity_basis":    "upstream_action_id",
		"source":            "live",
		"native_type":       nil,
		"reaction": map[string]any{
			"target_message_ids": []any{"target"},
			"reaction":           "heart",
			"action":             "add",
		},
	}, domain.ReactionObservedV1)
}

//nolint:gocognit // AAA compares raw schema, fallible DTO decode, ordered gap and following valid entry for both corruption families.
func TestPersistedEntryProjectionContainsInvalidPayloadBeforeTypedPageDecode(t *testing.T) {
	for _, test := range []struct {
		name        string
		event       map[string]any
		schemaValid bool
	}{
		{"incomplete-message-data", projectionEvent(map[string]any{}, domain.ConversationMessageCreatedV2), false},
		{"schema-valid-domain-invalid-activity", projectionActivity(), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Arrange: corrupt retained head followed by a complete message. All receipt
			// and actual transport metadata is distinct from processing readiness.
			head, following := projectionEntry(test.event), projectionEntry(projectionMessage())
			following["receipt"] = "following-receipt"
			page := projectionPage(head, following)
			schema, err := contracts.Compile("zalo_read_subscription_events", "output")
			if err != nil {
				t.Fatal(err)
			}
			if (schema.Validate(persistedEntryProjection(head)) == nil) != test.schemaValid {
				t.Fatal("fixture does not exercise intended schema/DTO boundary")
			}
			if test.schemaValid {
				raw, marshalErr := json.Marshal(test.event)
				if marshalErr != nil {
					t.Fatal(marshalErr)
				}
				var event eventaccess.Event
				if decodeErr := json.Unmarshal(raw, &event); decodeErr == nil {
					t.Fatal("fixture must fail Activity.Validate despite valid public schema")
				}
			}
			// Act
			err = normalizePersistedJournal(page)
			typed, decodeErr := catalogValue[eventaccess.Page](page, err)
			// Assert: Page remains decodable with receipt/order/actual sending intact.
			if decodeErr != nil || len(typed.Events) != 2 {
				t.Fatalf("one corrupt entry poisoned page: %v %+v", decodeErr, typed)
			}
			first := typed.Events[0]
			if first.Event != nil || first.ProcessingState != "gap" || first.GapReason == nil ||
				*first.GapReason != invalidJournalPayload ||
				first.Receipt != "synthetic-authenticated-receipt" ||
				first.DeliveryState != "sending" {
				t.Fatalf("invalid payload lost gap/receipt/transport semantics: %+v", first)
			}
			if typed.Events[1].Event == nil || typed.Events[1].ProcessingState != "ready" ||
				typed.Events[1].Receipt != "following-receipt" {
				t.Fatal("valid following entry changed")
			}
			if err := schema.Validate(canonicalCatalog(t, page)); err != nil {
				t.Fatalf("normalized page violates same full executable contract: %v", err)
			}
		})
	}
}

func TestPersistedEntryProjectionPreservesCanonicalValidEnvelope(t *testing.T) {
	// Arrange
	event := projectionMessage()
	before := canonicalCatalog(t, event)
	page := projectionPage(projectionEntry(event))
	// Act
	err := normalizePersistedJournal(page)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	entry := page["events"].([]map[string]any)[0]
	if !reflect.DeepEqual(before, canonicalCatalog(t, entry["event"])) || entry["processing_state"] != "ready" ||
		entry["gap_reason"] != nil {
		t.Fatal("valid canonical envelope changed")
	}
}

func TestPersistedEntryProjectionInfrastructureFailureIsNotDataGap(t *testing.T) {
	// Arrange: a contract/metadata validation failure cannot be repaired by masking
	// the author's event. The same applies to an unserializable local projection.
	entry := projectionEntry(projectionMessage())
	original := canonicalCatalog(t, entry)
	// Act
	err := normalizePersistedJournalEntry(
		entry,
		func(any) error { return errors.New("synthetic unavailable validator") },
	)
	// Assert
	if err == nil || !reflect.DeepEqual(original, canonicalCatalog(t, entry)) {
		t.Fatal("infrastructure failure converted to data gap")
	}
	// Arrange/Act/Assert: metadata must retain its own failure semantics.
	entry["receipt"] = ""
	page := projectionPage(entry)
	if err := normalizePersistedJournal(page); err == nil || entry["processing_state"] != "ready" {
		t.Fatal("invalid receipt was disguised as author payload failure")
	}
	entry = projectionEntry(projectionMessage())
	entry["event"] = make(chan int)
	validator, compileErr := contracts.Compile("zalo_read_subscription_events", "output")
	if compileErr != nil {
		t.Fatal(compileErr)
	}
	if err := normalizePersistedJournalEntry(
		entry,
		validator.Validate,
	); err == nil ||
		entry["processing_state"] != "ready" {
		t.Fatal("serialization infrastructure failure was disguised as author payload failure")
	}
}

func TestPersistedEntryProjectionKeepsValidOversizedEnvelopeForOwnerBudget(t *testing.T) {
	// Arrange: every field satisfies the real v3 schema, but 16 bounded Unicode
	// descriptors exceed the existing owner byte budget. Size is not corruption.
	event := projectionMessage()
	data := fixtureProjectionObject(t, event["data"])
	data["schema_version"] = 3
	data["content_resource_uri"] = "zalo://conversations/direct/peer/messages/synthetic-message/content"
	elements := []any{}
	for range 16 {
		elements = append(elements, map[string]any{
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
		})
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
	event["name"] = domain.ConversationMessageCreatedV3
	entry := projectionEntry(event)
	schema, err := contracts.Compile("zalo_read_subscription_events", "output")
	if err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(persistedEntryProjection(entry)); err != nil {
		t.Fatalf("large control must be schema valid: %v", err)
	}
	before, err := json.Marshal(entry)
	if err != nil || len(before) <= 28<<10 {
		t.Fatal("control does not exceed owner budget", err)
	}
	// Act
	project, err := persistedJournalProjection()
	if err != nil {
		t.Fatal(err)
	}
	err = project(entry)
	typed, decodeErr := catalogValue[eventaccess.Page](projectionPage(entry), err)
	// Assert: owner retains its independent budget error instead of classifying
	// valid data as an invalid_payload gap or silently truncating it.
	after, marshalErr := json.Marshal(entry)
	if decodeErr != nil || marshalErr != nil || !reflect.DeepEqual(before, after) ||
		typed.Events[0].ProcessingState != "ready" ||
		typed.Events[0].Event == nil {
		t.Fatalf("valid large payload altered: decode%v marshal%v", decodeErr, marshalErr)
	}
}

func fixtureProjectionObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", value)
	}
	return object
}
