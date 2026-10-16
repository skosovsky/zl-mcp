package zalo

import (
	"unicode/utf8"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

const contentLabelLimit = 512
const contentText = "text"
const contentUnsupported = "unsupported"
const nativeWebchat = "webchat"

// normalizeContent is shared by message sources. It deliberately projects only
// known shapes: native descriptions/URLs/params are not captions or authority.
func normalizeContent(d model.TMessage) domain.MessageContent {
	content := domain.MessageContent{
		TextRole: "none", Kind: contentUnsupported, NativeType: safeNativeType(d.MsgType),
		Summary: "Unsupported message content", Availability: metadataUnavailable,
		Reason: new("unknown_native_type"), Elements: []domain.ContentElement{},
	}
	if d.Content.String != nil && (d.MsgType == nativeWebchat || d.MsgType == "") {
		if *d.Content.String == "" {
			content.Reason = new("empty_content")
			return content
		}
		if !utf8.ValidString(*d.Content.String) {
			content.Reason = new("invalid_metadata")
			return content
		}
		content.Kind, content.Summary, content.Availability, content.Reason = contentText, "Text message", contentText, nil
		content.TextRole = "author"
		return content
	}
	kind, summary := mediaContentKind(d.MsgType)
	if kind == "" {
		if d.Content.Attachment != nil && d.Content.Attachment.Title != "" {
			content.Availability = metadataOnly
			content.Elements = []domain.ContentElement{
				{
					Kind:           "unsupported",
					Label:          boundedContentString(d.Content.Attachment.Title, contentLabelLimit),
					MIMEType:       nil,
					FileName:       nil,
					SizeBytes:      nil,
					DurationMS:     nil,
					Width:          nil,
					Height:         nil,
					StickerID:      nil,
					BytesAvailable: false,
				},
			}
		}
		return content
	}
	content.Kind, content.Summary = kind, summary
	if d.Content.Attachment == nil && d.Content.Other == nil {
		content.Reason = new("unsupported_shape")
		return content
	}
	content.Availability, content.Reason = metadataOnly, new("metadata_limited")
	// Other preserves object-shaped messages such as stickers. Its arbitrary
	// fields are not published until their semantics are independently proven.
	label := (*string)(nil)
	if d.Content.Attachment != nil {
		label = boundedContentString(d.Content.Attachment.Title, contentLabelLimit)
	}
	content.Elements = []domain.ContentElement{{
		Kind: kind, Label: label, MIMEType: nil, FileName: nil,
		SizeBytes: nil, DurationMS: nil, Width: nil, Height: nil,
		StickerID: nil, BytesAvailable: false,
	}}
	return content
}

// These names are grounded in zca-js getClientMessageType, not in guesses from
// empty text. chat.recommended is ambiguous (link/contact), so stays unsupported.
func mediaContentKind(native string) (string, string) {
	switch native {
	case nativePhoto:
		return contentImage, "Image message; media bytes unavailable"
	case "chat.sticker":
		return "sticker", "Sticker message; media bytes unavailable"
	case "chat.voice":
		return "audio", "Audio message; media bytes unavailable"
	case nativeFile:
		return "file", "File message; media bytes unavailable"
	case "chat.link":
		return contentLink, "Link message; metadata limited"
	case "chat.location.new":
		return "location", "Location message; metadata limited"
	default:
		return "", ""
	}
}

func boundedContentString(s string, limit int) *string {
	if s == "" || !utf8.ValidString(s) {
		return nil
	}
	r := []rune(s)
	if len(r) > limit {
		s = string(r[:limit])
	}
	return &s
}

func safeNativeType(s string) *string {
	if len(s) == 0 || len(s) > 64 {
		return nil
	}
	for _, r := range s {
		letter := r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
		valid := letter || r >= '0' && r <= '9' || r == '_' || r == '.' || r == '-'
		if !valid {
			return nil
		}
	}
	return &s
}
