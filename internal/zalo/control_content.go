package zalo

import (
	"errors"
	"log/slog"

	"github.com/amrakk/zcago/model"
)

var errControlRecord = errors.New("control observation is not a message")

func isControlContent(d model.TMessage) bool {
	if d.MsgType == "chat.undo" || len(d.Content.DeletedContent) > 0 {
		return true
	}
	_, hasDelete := d.Content.Other["deleteMsg"]
	return hasDelete
}

// Controls bypass message persistence/novelty. Do not infer a delete target or
// mutation from an actionType enum that has no proven upstream mapping.
func logControlRecord(source string) {
	switch source {
	case sourceLive, "replay", "history", "preload":
	default:
		source = "unknown"
	}
	//nolint:sloglint // Fixed bounded fields only, never control payload/identities.
	slog.Info("zalo_control_observed", "source", source, "action", "not_projected_as_message")
}

func isNonMessageRecord(err error) bool {
	return errors.Is(err, errControlRecord) || errors.Is(err, errCallRecord)
}
