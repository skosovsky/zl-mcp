package zalo

import (
	"crypto/sha256"
	"encoding/hex"
	"log/slog"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// logContentProjection records only bounded shape/classification evidence for
// live non-text content. No native payload, actor, text or media URL is logged.
func logContentProjection(d model.TMessage, ref domain.ConversationRef, source string, content domain.MessageContent) {
	if source != sourceLive || content.Kind == contentText {
		return
	}
	shape := wireNull
	switch {
	case d.Content.String != nil:
		shape = "string"
	case d.Content.Attachment != nil:
		shape = "attachment"
	case d.Content.Other != nil:
		shape = "object"
	case d.Content.DeletedContent != nil:
		shape = "control_array"
	case d.Content.Unsupported != nil:
		shape = contentUnsupported
	}
	native := metadataUnavailable
	if content.NativeType != nil {
		native = loggedNativeType(*content.NativeType)
	}
	fingerprint := sha256.Sum256([]byte(ref.Type + "\x00" + ref.ID + "\x00" + d.MsgID))
	titlePresent, descriptionPresent, typePresent, actionPresent := false, false, false, false
	childShape := metadataUnavailable
	if a := d.Content.Attachment; a != nil {
		titlePresent, descriptionPresent = a.Title != "", a.Description != ""
		typePresent, actionPresent = a.Type != "", a.Action != ""
		switch {
		case a.ChildNumber == 0:
			childShape = "zero"
		case a.ChildNumber > 0:
			childShape = "positive"
		default:
			childShape = "negative"
		}
	}
	//nolint:sloglint // Use the service redacting logger with a closed bounded allowlist, never raw native fields.
	slog.Info(
		"zalo_content_projection",
		"native_type",
		native,
		"native_type_present", d.MsgType != "",
		"shape",
		shape,
		"content_kind",
		content.Kind,
		"availability",
		content.Availability,
		"title_present", titlePresent,
		"description_present", descriptionPresent,
		"attachment_type_present", typePresent,
		"attachment_action_present", actionPresent,
		"child_count_shape", childShape,
		"record_fingerprint",
		hex.EncodeToString(fingerprint[:]),
		"content_shape", contentShape(d),
	)
}

// Ordinary diagnostics use a closed vocabulary; lexical validation alone would
// allow a private name or hostname to masquerade as a native message type.
func loggedNativeType(value string) string {
	switch value {
	case nativeWebchat, nativePhoto, "chat.sticker", "chat.voice", nativeFile,
		"chat.link", "chat.location.new", nativeRecommended:
		return value
	default:
		return nativeUnknown
	}
}
