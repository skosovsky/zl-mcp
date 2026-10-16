package zalo

import (
	"bytes"
	"encoding/json"
	"unicode/utf8"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// refineDirectContent applies only the incoming direct forms established by the
// controlled native batch. Call after direction resolution on every source path.
func refineDirectContent(message *domain.Message, native model.TMessage) {
	if message.Ref().Type != domain.ConversationDirect || message.Direction != directionIncoming ||
		message.Content == nil || native.Content.Attachment == nil {
		return
	}
	a := native.Content.Attachment
	if native.MsgType == nativeFile && a.Action == "" && a.Type == "" &&
		a.Title != "" && utf8.ValidString(a.Title) {
		for i := range message.Content.Elements {
			message.Content.Elements[i].FileName = boundedContentString(a.Title, contentFileNameLimit)
		}
		return
	}
	if native.MsgType == nativeRecommended && a.Action == "recommened.link" && a.Type == "" {
		message.Content.Kind = contentLink
		message.Content.Summary = "Link message; metadata limited"
		message.Content.Availability = metadataOnly
		message.Content.Reason = new("metadata_limited")
		for i := range message.Content.Elements {
			message.Content.Elements[i].Kind = contentLink
		}
		return
	}
	if native.MsgType != nativePhoto || message.Content.Kind != contentImage ||
		a.Title == "" || !utf8.ValidString(a.Title) || a.Action != "" || a.Type != "" ||
		a.ChildNumber != 0 || native.Quote != nil || !singlePhotoParams(a.Params) {
		return
	}
	message.Content.TextRole = "author"
	for i := range message.Content.Elements {
		message.Content.Elements[i].Label = nil
	}
}

// Width/height establish a shape, not units or published dimensions. Any album
// marker, including a present empty/zero marker, excludes this caption mapping.
func singlePhotoParams(raw string) bool {
	if len(raw) > 64<<10 || !json.Valid([]byte(raw)) {
		return false
	}
	var params map[string]any
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.UseNumber()
	if decoder.Decode(&params) != nil || params == nil {
		return false
	}
	for _, key := range []string{nativePhotoWidthField, nativePhotoHeightField} {
		if _, ok := params[key].(json.Number); !ok {
			return false
		}
	}
	for _, key := range []string{nativeAlbumLayoutIDField, nativeAlbumLayoutFlagField, nativeAlbumIndexField, nativeAlbumTotalField} {
		if _, present := params[key]; present {
			return false
		}
	}
	return true
}
