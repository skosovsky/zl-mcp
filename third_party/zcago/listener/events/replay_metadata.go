package events

import (
	"bytes"
	"encoding/json"
)

// ContinuationMetadata deliberately leaves messages usable when metadata is
// absent or malformed. Queue exhaustion is distinct from missing metadata.
func (d OldMessagesEventData) ContinuationMetadata() (more *bool, actionID string, valid bool) {
	valid = true
	flag := bytes.TrimSpace(d.More)
	switch string(flag) {
	case "", "null":
	case "true", "1":
		v := true
		more = &v
	case "false", "0":
		v := false
		more = &v
	default:
		valid = false
	}
	raw := bytes.TrimSpace(d.LastActionID)
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	if raw[0] == '"' {
		if json.Unmarshal(raw, &actionID) != nil {
			return more, "", false
		}
	} else {
		actionID = string(raw)
	}
	if len(actionID) == 0 || len(actionID) > 128 {
		return more, "", false
	}
	for _, r := range actionID {
		if r < '0' || r > '9' {
			return more, "", false
		}
	}
	return
}
