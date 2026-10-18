package model

import "encoding/json"

// UnmarshalJSON preserves presence of the three protocol counters required by
// seenv2. Missing/null counters must not be mistaken for observed zero values.
func (m *TMessage) UnmarshalJSON(data []byte) error {
	type plain TMessage
	var decoded plain
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	decoded.SeenFieldsPresent = true
	for _, name := range []string{"st", "at", "cmd"} {
		raw, ok := fields[name]
		if !ok || string(raw) == "null" {
			decoded.SeenFieldsPresent = false
		}
	}
	*m = TMessage(decoded)
	return nil
}

// Decode the embedding explicitly: an embedded Unmarshaler otherwise consumes
// the entire group object and silently drops mentions.
func (m *TGroupMessage) UnmarshalJSON(data []byte) error {
	var base TMessage
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}
	var extras struct {
		Mentions []*TMention `json:"mentions,omitempty"`
	}
	if err := json.Unmarshal(data, &extras); err != nil {
		return err
	}
	*m = TGroupMessage{TMessage: base, Mentions: extras.Mentions}
	return nil
}
