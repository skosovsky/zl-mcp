package storage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func processingFixture(t *testing.T) (*Store, string, string, time.Time) {
	t.Helper()
	s, path, generation, at := journalFixture(t)
	// Canonical envelope shape, with synthetic IDs/content. The fixture's original
	// queue encoder deliberately exercises a smaller transport-only JSON format.
	for i, name := range []string{"one", "two", "three"} {
		var eventID string
		if err := s.DB.QueryRow(`SELECT event_id FROM event_deliveries WHERE id=?`, i+1).Scan(&eventID); err != nil {
			t.Fatal(err)
		}
		payload := processingFixturePayload(t, eventID, name, at)
		if _, err := s.DB.Exec(`UPDATE event_deliveries SET payload=? WHERE id=?`, payload, i+1); err != nil {
			t.Fatal(err)
		}
	}
	return s, path, generation, at
}

func processingFixturePayload(t *testing.T, id, message string, at time.Time) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"eventId":   id,
		"name":      domain.ConversationMessageCreatedV2,
		"timestamp": recoveryStamp(at),
		"cursor":    nil,
		"data": map[string]any{
			"schema_version":    2,
			"conversation_id":   "peer",
			"conversation_type": "direct",
			"conversation_name": nil,
			"direction":         "incoming",
			"first_incoming":    true,
			"message_id":        message,
			"sender_id":         "peer",
			"sender_name":       nil,
			"sent_at":           recoveryStamp(at),
			"text":              message,
			"text_truncated":    false,
			"text_resource_uri": nil,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func processingPage(t *testing.T, s *Store, limit int) map[string]any {
	t.Helper()
	ctx := domain.WithPersistedEventReadiness(context.Background())
	page, err := s.ReadSubscriptionEvents(ctx, "owner", "sub", limit)
	if err != nil {
		t.Fatal(err)
	}
	if page["readiness_mode"] != domain.EventReadinessPersisted || page["blocked_on_delivery"] != false {
		t.Fatalf("readiness=%v", page)
	}
	schema, err := contracts.Compile("zalo_read_subscription_events", "output")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	var value any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(value); err != nil {
		t.Fatalf("invalid output %v: %s", err, raw)
	}
	return page
}

func processingRecords(t *testing.T, s *Store, limit int) []map[string]any {
	t.Helper()
	return processingPage(t, s, limit)["events"].([]map[string]any)
}

func TestPersistedReadinessEarlyAckKeepsOutboxAndRestartRetry(t *testing.T) {
	// Arrange: nothing is delivered and no companion exists.
	s, path, _, at := processingFixture(t)
	defer func() { _ = s.Close() }()
	ctx := context.Background()
	legacy, err := s.ReadSubscriptionEvents(ctx, "owner", "sub", 20)
	if err != nil || legacy["blocked_on_delivery"] != true || len(legacy) != 4 {
		t.Fatalf("legacy=%v %v", legacy, err)
	}
	before := processingRecords(t, s, 20)
	if len(before) != 3 || before[0]["delivery_state"] != statePendingText || before[0]["processing_state"] != "ready" {
		t.Fatal(before)
	}
	// Act: acknowledge while HTTP still owns its sending lease.
	delivery, err := s.ClaimDelivery(ctx, at, DefaultDeliveryPolicy())
	if err != nil || delivery == nil {
		t.Fatalf("claim=%v %v", delivery, err)
	}
	sending := processingRecords(t, s, 1)[0]
	if sending["delivery_state"] != "sending" {
		t.Fatal("transport was promoted")
	}
	receipt := sending["receipt"].(string)
	if _, err = s.AckSubscriptionEvents(ctx, "owner", "sub", receipt); err != nil {
		t.Fatal(err)
	}
	assertUnfinishedProcessingBytes(t, s, delivery.ID, "sending", delivery.Payload)
	if err = s.FinishDelivery(
		ctx,
		*delivery,
		DeliveryOutcome{State: statePendingText, Reason: "network", RetryAt: at.Add(time.Second)},
		at,
	); err != nil {
		t.Fatal(err)
	}
	assertUnfinishedProcessingBytes(t, s, delivery.ID, statePendingText, delivery.Payload)
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	// Assert: restart retries the exact callback, while processing resumes at two.
	retry, err := s.ClaimDelivery(ctx, at.Add(time.Second), DefaultDeliveryPolicy())
	if err != nil || retry == nil || retry.EventID != delivery.EventID ||
		string(retry.Payload) != string(delivery.Payload) {
		t.Fatalf("retry changed %v %v", retry, err)
	}
	remaining := processingRecords(t, s, 20)
	if len(remaining) != 2 ||
		remaining[0]["event"].(map[string]any)["eventId"] != before[1]["event"].(map[string]any)["eventId"] {
		t.Fatal("ack replayed after restart")
	}
	if _, err = s.AckSubscriptionEvents(ctx, "owner", "sub", receipt); err != nil {
		t.Fatal("ack retry lost idempotency", err)
	}
	if err = s.PruneDeliveries(ctx, at.Add(8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	assertUnfinishedProcessingBytes(t, s, retry.ID, "sending", retry.Payload)
}

func assertUnfinishedProcessingBytes(t *testing.T, s *Store, id int64, want string, payload []byte) {
	t.Helper()
	var state string
	var body []byte
	if err := s.DB.QueryRow(`SELECT state,payload FROM event_deliveries WHERE id=?`, id).
		Scan(&state, &body); err != nil || state != want ||
		string(body) != string(payload) {
		t.Fatalf("outbox mutated %s %v", state, err)
	}
}

func TestPersistedReadinessRetainedFailedAndExplicitGaps(t *testing.T) {
	for _, scenario := range []string{"failed_retained", "empty", "invalid_json", "scalar", "array", "missing_fields", "cancelled", "deleted", "policy_revoked", "wrong_event_id", "wrong_profile"} {
		t.Run(scenario, func(t *testing.T) { assertPersistedGap(t, scenario) })
	}
}

func assertPersistedGap(t *testing.T, scenario string) {
	t.Helper()
	// Arrange.
	s, _, _, at := processingFixture(t)
	defer s.Close()
	ctx := context.Background()
	expected := reasonInvalidPayload
	switch scenario {
	case "failed_retained":
		terminalBusiness(t, s, at, stateFailedText)
		expected = ""
	case "empty":
		setProcessingBody(t, s, "")
		expected = "payload_unavailable"
	case "invalid_json":
		setProcessingBody(t, s, "{")
	case "scalar":
		setProcessingBody(t, s, `"scalar"`)
	case "array":
		setProcessingBody(t, s, `[]`)
	case "missing_fields":
		setProcessingBody(t, s, `{}`)
	case "wrong_event_id", "wrong_profile":
		setMismatchedProcessingEnvelope(t, s, scenario)
	case "cancelled":
		if _, err := s.DB.Exec(`UPDATE event_deliveries SET state='cancelled' WHERE id=1`); err != nil {
			t.Fatal(err)
		}
		expected = "delivery_cancelled"
	case "deleted":
		if err := s.DeleteConversation(ctx, domain.ConversationRef{Type: "direct", ID: "peer"}, "one"); err != nil {
			t.Fatal(err)
		}
		expected = "message_unavailable_or_access_revoked"
	case "policy_revoked":
		s.policy = domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{}}
		expected = "message_unavailable_or_access_revoked"
	}
	// Act.
	entry := processingRecords(t, s, 1)[0]
	// Assert: failed transport is separate from retained data; unavailable data
	// never reappear through a freshly read corpus record under an old event ID.
	if expected == "" {
		if entry["processing_state"] != "ready" || entry["gap_reason"] != nil || entry["event"] == nil ||
			entry["delivery_state"] != stateFailedText {
			t.Fatal(entry)
		}
	} else if entry["processing_state"] != "gap" || entry["gap_reason"] != expected || entry["event"] != nil {
		t.Fatalf("gap %s: %v", scenario, entry)
	}
	if _, err := s.AckSubscriptionEvents(ctx, "owner", "sub", entry["receipt"].(string)); err != nil {
		t.Fatal(err)
	}
	if scenario == "wrong_event_id" || scenario == "wrong_profile" {
		next := processingRecords(t, s, 1)[0]
		if next["processing_state"] != "ready" ||
			next["event"].(map[string]any)["data"].(map[string]any)["message_id"] != "two" {
			t.Fatal("invalid head stranded valid tail", next)
		}
	}
}

func setProcessingBody(t *testing.T, s *Store, body string) {
	t.Helper()
	if _, err := s.DB.Exec(`UPDATE event_deliveries SET payload=? WHERE id=1`, []byte(body)); err != nil {
		t.Fatal(err)
	}
}

func TestPersistedReadinessReceiptsRemainOrderedAndGenerationScoped(t *testing.T) {
	// Arrange.
	s, _, _, at := processingFixture(t)
	defer s.Close()
	ctx := context.Background()
	entries := processingRecords(t, s, 2)
	// Act/Assert: no skipping, foreign owner or old generation is accepted.
	if _, err := s.AckSubscriptionEvents(ctx, "owner", "sub", entries[1]["receipt"].(string)); err == nil {
		t.Fatal("skipped ordered prefix")
	}
	if _, err := s.AckSubscriptionEvents(ctx, "foreign", "sub", entries[0]["receipt"].(string)); err == nil {
		t.Fatal("foreign ack")
	}
	if err := s.CancelSubscription(ctx, "sub", "owner", at); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadSubscriptionEvents(domain.WithPersistedEventReadiness(ctx), "owner", "sub", 1); err == nil {
		t.Fatal("cancelled generation reopened")
	}
	revision, err := s.SubscriptionRevision(ctx, "sub")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateSubscription(
		ctx,
		EventSubscription{
			ID:        "sub",
			Principal: "owner",
			Profile:   domain.ConversationMessageCreatedV2,
			Scope:     "all",
			Callback:  "https://receiver.example",
			Secret:    "synthetic",
		},
		revision,
		at,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AckSubscriptionEvents(ctx, "owner", "sub", entries[0]["receipt"].(string)); err == nil {
		t.Fatal("old generation ack accepted")
	}
}

func TestPersistedReadinessExecutableContractRejectsContradictions(t *testing.T) {
	// Arrange: mutate a real storage projection, not an independent schema-shaped fixture.
	s, _, _, _ := processingFixture(t)
	defer s.Close()
	raw, err := json.Marshal(processingPage(t, s, 1))
	if err != nil {
		t.Fatal(err)
	}
	schema, err := contracts.Compile("zalo_read_subscription_events", "output")
	if err != nil {
		t.Fatal(err)
	}
	for _, mutation := range []string{"ready_null_event", "gap_with_event", "blocked", "missing_processing_state", "legacy_pending", "legacy_processing_state"} {
		t.Run(mutation, func(t *testing.T) {
			var page map[string]any
			if decodeErr := json.Unmarshal(raw, &page); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			entry := page["events"].([]any)[0].(map[string]any)
			// Act.
			mutateProcessingContract(page, entry, mutation)
			// Assert.
			if validateErr := schema.Validate(page); validateErr == nil {
				t.Fatal("contradictory projection accepted")
			}
		})
	}
	input, err := contracts.Compile("zalo_read_subscription_events", "input")
	if err != nil {
		t.Fatal(err)
	}
	if err = input.Validate(map[string]any{"subscription_id": "sub", "readiness_mode": "unknown"}); err == nil {
		t.Fatal("unknown readiness mode accepted")
	}
}

func mutateProcessingContract(page, entry map[string]any, mutation string) {
	switch mutation {
	case "ready_null_event":
		entry["event"] = nil
	case "gap_with_event":
		entry["processing_state"] = "gap"
		entry["gap_reason"] = "synthetic_gap"
	case "blocked":
		page["blocked_on_delivery"] = true
	case "missing_processing_state":
		delete(entry, "processing_state")
	case "legacy_pending":
		delete(page, "readiness_mode")
		delete(entry, "processing_state")
	case "legacy_processing_state":
		delete(page, "readiness_mode")
		entry["delivery_state"] = stateDeliveredText
	}
}

func TestRetainedProcessingEnvelopeRejectsMalformedStructure(t *testing.T) {
	// Arrange: only structure is checked here; canonical encoders and SDK schemas
	// validate profile-specific data at their existing respective boundaries.
	valid := processingFixturePayload(t, "id", "message", time.Now().UTC())
	for _, mutation := range []string{"eventId", "name", "timestamp", "data", "cursor", "foreign_profile", "activity_mismatch"} {
		t.Run(mutation, func(t *testing.T) {
			var event map[string]any
			if err := json.Unmarshal(valid, &event); err != nil {
				t.Fatal(err)
			}
			switch mutation {
			case "foreign_profile":
				event["name"] = domain.JournalReadyV1
			case "activity_mismatch":
				event["name"] = domain.CallObservedV1
			default:
				delete(event, mutation)
			}
			body, err := json.Marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			// Act/Assert.
			if _, ok := retainedProcessingEnvelope(body, false); ok {
				t.Fatal("malformed or foreign envelope accepted")
			}
		})
	}
}

func TestPersistedReadinessActivityExpiryCannotResurrectBytes(t *testing.T) {
	// Arrange: a retained reaction observation is independently queued, still pending.
	s := reactionTargetStore(t)
	ctx := context.Background()
	at := time.Now().UTC()
	revision, err := s.SubscriptionRevision(ctx, "sub")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.ActivateSubscription(
		ctx,
		EventSubscription{
			ID:        "sub",
			Principal: "owner",
			Profile:   domain.ReactionObservedV1,
			Scope:     "all",
			Callback:  "https://receiver.example",
			Secret:    "synthetic",
		},
		revision,
		at,
	); err != nil {
		t.Fatal(err)
	}
	observation := clientReaction()
	if err = s.PutActivity(ctx, observation); err != nil {
		t.Fatal(err)
	}
	_, err = s.FanoutActivityEvents(
		ctx,
		at,
		DefaultDeliveryPolicy(),
		func(profile, id string, a domain.Activity) ([]byte, error) {
			return json.Marshal(
				map[string]any{
					"eventId":   id,
					"name":      profile,
					"timestamp": recoveryStamp(a.OccurredAt),
					"data":      a,
					"cursor":    nil,
				},
			)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	before := processingRecords(t, s, 1)
	if len(before) != 1 || before[0]["processing_state"] != "ready" || before[0]["delivery_state"] != statePendingText {
		t.Fatal(before)
	}
	// Act: retention intentionally removes both source and outbox content.
	if err = s.PruneActivities(ctx, at.Add(91*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// Assert: a receipt-bearing explicit gap is still processable; no bytes are reconstructed.
	after := processingRecords(t, s, 1)[0]
	if after["processing_state"] != "gap" || after["event"] != nil ||
		after["gap_reason"] != "activity_unavailable_or_access_revoked" {
		t.Fatal(after)
	}
	if _, err = s.AckSubscriptionEvents(ctx, "owner", "sub", after["receipt"].(string)); err != nil {
		t.Fatal(err)
	}
	if err = s.PutActivity(ctx, observation); err != nil {
		t.Fatal(err)
	}
	if len(processingRecords(t, s, 1)) != 0 {
		t.Fatal("expired activity replayed")
	}
}

func setMismatchedProcessingEnvelope(t *testing.T, s *Store, scenario string) {
	t.Helper()
	var raw []byte
	if err := s.DB.QueryRow(`SELECT payload FROM event_deliveries WHERE id=1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	if scenario == "wrong_event_id" {
		event["eventId"] = "foreign-event"
	} else {
		event["name"] = domain.LegacyMessageCreated
		event["data"] = map[string]any{
			"group_id":          "peer",
			"message_id":        "one",
			"sender_id":         "peer",
			"sender_name":       nil,
			"sent_at":           event["timestamp"],
			"text":              "one",
			"text_truncated":    false,
			"text_resource_uri": nil,
		}
	}
	body, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	setProcessingBody(t, s, string(body))
}
