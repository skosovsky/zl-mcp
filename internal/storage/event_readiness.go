package storage

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (s *Store) projectJournalEvent(
	payload []byte,
	state string,
	kind, peer sql.NullString,
	activity, persisted bool,
	expectedEventID, expectedProfile string,
) (any, any) {
	if !persisted {
		return s.projectRecoveryEvent(payload, state, kind, peer, activity)
	}
	// Processing does not promote a transport result. Privacy/lifecycle checks
	// precede envelope access, including when transport has not completed yet.
	if !kind.Valid || !s.AllowsConversation(domain.ConversationRef{Type: kind.String, ID: peer.String}) {
		if activity {
			return nil, "activity_unavailable_or_access_revoked"
		}
		return nil, "message_unavailable_or_access_revoked"
	}
	if state == stateCancelledText {
		return nil, "delivery_cancelled"
	}
	if len(payload) == 0 {
		return nil, "payload_unavailable"
	}
	event, valid := retainedProcessingEnvelope(payload, activity)
	if !valid || event["eventId"] != expectedEventID || event["name"] != expectedProfile {
		return nil, reasonInvalidPayload
	}
	return event, nil
}

// Retained bytes come from canonical fanout encoders. This structural
// guard rejects corrupt/scalar/foreign envelopes without duplicating the full
// profile contracts enforced by the encoder and public SDK output boundary.
func retainedProcessingEnvelope(payload []byte, activity bool) (map[string]any, bool) {
	var event map[string]any
	if json.Unmarshal(payload, &event) != nil || event == nil {
		return nil, false
	}
	id, idOK := event["eventId"].(string)
	name, nameOK := event["name"].(string)
	timestamp, timeOK := event["timestamp"].(string)
	data, dataOK := event["data"].(map[string]any)
	cursor, cursorOK := event["cursor"]
	if !idOK || id == "" || !nameOK || !timeOK || !dataOK || data == nil || !cursorOK || cursor != nil {
		return nil, false
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		return nil, false
	}
	if activity {
		return event, domain.IsActivityProfile(name)
	}
	return event, name == domain.LegacyMessageCreated || domain.IsConversationMessageProfile(name)
}

func addProcessingState(entry map[string]any) {
	state := "ready"
	if entry["event"] == nil {
		state = "gap"
	}
	entry["processing_state"] = state
}

func (view journalReadView) annotateEntries(entries []map[string]any) {
	if !view.persisted {
		return
	}
	for _, entry := range entries {
		addProcessingState(entry)
	}
}

func (view journalReadView) projectEntry(entry map[string]any) error {
	if !view.persisted || view.project == nil {
		return nil
	}
	return view.project(entry)
}

func (view journalReadView) annotateEntry(s *Store, entry map[string]any, id int64, lastReason string) {
	if view.persisted {
		addProcessingState(entry)
	}
	if view.diagnostics {
		entry["delivery_reason"] = safeDeliveryReason(lastReason)
		entry["delivery_reference"] = s.DeliveryReference(id)
	}
}
