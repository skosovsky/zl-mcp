package events

import (
	"encoding/json"
	"errors"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (e *MessageEncoder) EncodeActivity(profile, id string, a domain.Activity) ([]byte, error) {
	if profile != a.Profile() || id == "" {
		return nil, errors.New("invalid activity profile or identity")
	}
	if err := a.Validate(); err != nil {
		return nil, err
	}
	a.OccurredAt = a.OccurredAt.UTC()
	wire, err := json.Marshal(
		map[string]any{"eventId": id, "name": profile, "timestamp": a.OccurredAt, "data": a, fieldCursor: nil},
	)
	if err != nil || len(wire) > 256<<10 {
		return nil, errors.New("activity exceeds delivery body limit")
	}
	schema := e.reactionSchema
	if a.Family == "call" {
		schema = e.callSchema
	}
	var value any
	if json.Unmarshal(wire, &value) != nil || schema.Validate(value) != nil {
		return nil, errors.New("activity violates event contract")
	}
	return wire, nil
}
