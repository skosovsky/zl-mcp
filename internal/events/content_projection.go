package events

import (
	"encoding/json"
	"errors"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// Leave room for the processing receipt/entry inside the journal's 28 KiB page.
const maximumRecoveryEnvelopeBytes = 26 << 10

func encodeConversationEnvelope(envelope map[string]any, schema *jsonschema.Schema) ([]byte, error) {
	body, err := json.Marshal(envelope)
	if err != nil || len(body) > maximumCallbackBodyBytes {
		return nil, errors.New("event exceeds delivery body limit")
	}
	var value any
	if json.Unmarshal(body, &value) != nil || schema.Validate(value) != nil {
		return nil, errors.New("conversation message does not satisfy event contract")
	}
	if envelope[fieldName] != domain.ConversationMessageCreatedV3 {
		return body, nil
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		return nil, errors.New("invalid conversation event data")
	}
	fullContent, ok := data["content"].(*domain.MessageContent)
	if !ok || fullContent == nil {
		return nil, errors.New("invalid conversation event content")
	}
	// Copy the projection: trimming the event must never mutate stored content.
	content := *fullContent
	for len(body) > maximumRecoveryEnvelopeBytes && len(content.Elements) > 0 {
		content.Elements = content.Elements[:len(content.Elements)-1]
		data["content"] = &content
		data["content_truncated"] = true
		body, err = json.Marshal(envelope)
		if err != nil {
			return nil, err
		}
	}
	if len(body) > maximumRecoveryEnvelopeBytes {
		return nil, errors.New("event exceeds recovery envelope limit")
	}
	return body, nil
}
