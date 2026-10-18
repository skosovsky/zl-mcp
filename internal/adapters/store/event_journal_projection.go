package store

import (
	"encoding/json"
	"errors"
	"maps"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

const invalidJournalPayload = "invalid_payload"

// persistedJournalProjection binds the cached contract at the serialization
// boundary. The owner calls this pure projection after privacy/identity guards
// and before its response budget, so malformed large bytes cannot poison a head.
func persistedJournalProjection() (func(map[string]any) error, error) {
	validator, err := contracts.Compile("zalo_read_subscription_events", "output")
	if err != nil {
		return nil, err
	}
	return func(entry map[string]any) error {
		if entry["processing_state"] != "ready" {
			return nil
		}
		return normalizePersistedJournalEntry(entry, validator.Validate)
	}, nil
}

func persistedEntryProjection(entry map[string]any) map[string]any {
	return map[string]any{
		"subscription_id":     "serialization-contract-check",
		"readiness_mode":      domain.EventReadinessPersisted,
		"events":              []any{entry},
		"has_more":            false,
		"blocked_on_delivery": false,
	}
}

func normalizePersistedJournalEntry(entry map[string]any, validate func(any) error) error {
	// First distinguish malformed projection metadata from invalid envelope bytes.
	// An invalid receipt/state must remain an infrastructure error, not a gap.
	gap := maps.Clone(entry)
	setInvalidPayloadGap(gap)
	if err := validate(persistedEntryProjection(gap)); err != nil {
		return errors.New("invalid local persisted journal entry metadata")
	}
	body, err := json.Marshal(entry["event"])
	if err != nil {
		return err
	}
	// EventData.UnmarshalJSON invokes the closed Activity.Validate union. Check
	// schema and each DTO before Page decoding; either data failure is a gap.
	var event eventaccess.Event
	if validate(persistedEntryProjection(entry)) != nil || json.Unmarshal(body, &event) != nil {
		setInvalidPayloadGap(entry)
	}
	return nil
}

func setInvalidPayloadGap(entry map[string]any) {
	entry["event"] = nil
	entry["gap_reason"] = invalidJournalPayload
	entry["processing_state"] = "gap"
}
