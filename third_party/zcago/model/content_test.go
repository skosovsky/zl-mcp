package model

import (
	"encoding/json"
	"testing"
)

func TestContentWithoutTitleAndReuse(t *testing.T) {
	// Arrange: reuse a previously decoded value as streaming decoders may do.
	var value Content
	if err := json.Unmarshal([]byte(`"previous text"`), &value); err != nil {
		t.Fatal(err)
	}
	// Act: a photo-like object with no title must survive as structured content.
	if err := json.Unmarshal(
		[]byte(`{"href":"https://synthetic.invalid/image","thumb":"x","title":""}`),
		&value,
	); err != nil {
		t.Fatal(err)
	}
	// Assert: no stale text arm and no dependence on a nonempty title.
	if value.String != nil || value.Attachment == nil || value.Attachment.Href == "" {
		t.Fatal("attachment lost or stale string retained")
	}
	// Act: JSON null is not an empty textual message.
	if err := json.Unmarshal([]byte(`null`), &value); err != nil {
		t.Fatal(err)
	}
	// Assert.
	if value.String != nil || value.Attachment != nil || value.Other != nil {
		t.Fatal("null retained a union arm")
	}
}

func TestContentUnknownObjectAndMalformedKnownField(t *testing.T) {
	// Arrange.
	for _, raw := range []string{`{"unknown":17}`, `{"title":3,"href":"x"}`, `{}`} {
		var value Content
		// Act.
		err := json.Unmarshal([]byte(raw), &value)
		// Assert: unknown objects are preserved rather than coerced to empty attachments.
		if err != nil || value.Other == nil || value.Attachment != nil {
			t.Fatal("unknown object was lost", err)
		}
	}
}

func TestValidUnknownContentShapeDoesNotFailWholeMessage(t *testing.T) {
	for _, raw := range []string{`true`, `17`, `[1,"x"]`} {
		// Arrange / Act: valid JSON of an unknown content shape is not a frame failure.
		var content Content
		if err := json.Unmarshal([]byte(raw), &content); err != nil {
			t.Fatal("unknown content terminated decode", err)
		}
		result, err := json.Marshal(content)
		// Assert: source shape survives internally without invented attachment/text arms.
		if err != nil || string(result) != raw || content.String != nil || content.Attachment != nil {
			t.Fatal("unknown content lost", err)
		}
	}
}

func TestDeletionControlsRequireExplicitShape(t *testing.T) {
	for _, tc := range []struct {
		wire    string
		control bool
	}{
		{`[{"actionType":1,"globalDelMsgId":42}]`, true},
		{`[{}]`, false}, {`[]`, false}, {`[{"title":"photo"}]`, false},
		{`[{"globalDelMsgId":42}]`, false},
		{`[{"actionType":1,"globalDelMsgId":42},{}]`, false},
	} {
		// Arrange / Act.
		var content Content
		err := json.Unmarshal([]byte(tc.wire), &content)
		// Assert: unsupported arrays stay losslessly retained, not invented controls.
		if err != nil || (len(content.DeletedContent) > 0) != tc.control {
			t.Fatal(tc.wire, err)
		}
		if !tc.control && len(content.Unsupported) == 0 {
			t.Fatal("unknown array lost")
		}
	}
}

func TestControlMarkerPrecedesAttachmentProjection(t *testing.T) {
	// Arrange / Act: attachment-like keys must not swallow the control marker.
	var content Content
	err := json.Unmarshal([]byte(`{"deleteMsg":1,"globalMsgId":42,"title":"x","href":"private"}`), &content)
	// Assert: application can classify it without reconstructing discarded fields.
	if err != nil || content.Attachment != nil || content.Other["deleteMsg"] == nil {
		t.Fatal("control marker lost", err)
	}
}
