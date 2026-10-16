package zalo

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestContentDiagnosticsDoNotLogNativePayloadOrIdentity(t *testing.T) {
	// Arrange: attacker-controlled text/metadata, invalid native-type string.
	old := slog.Default()
	var output bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	message := model.TMessage{
		MsgType: "private native type\nsecret",
		MsgID:   "private-message-id",
		UIDFrom: "private-actor",
		Content: model.Content{
			Other: model.TOtherContent{
				"href":  "https://synthetic.invalid/private-token",
				"title": "private words",
			},
		},
	}
	ref := domain.ConversationRef{Type: "direct", ID: "private-peer"}
	// Act.
	logContentProjection(message, ref, "live", normalizeContent(message))
	// Assert: classification and non-reversible local correlation remain useful.
	text := output.String()
	for _, private := range []string{"private native", "private-message", "private-actor", "private-peer", "private-token", "private words", "href"} {
		if strings.Contains(text, private) {
			t.Fatal("private native field leaked")
		}
	}
	if !strings.Contains(text, "record_fingerprint") || !strings.Contains(text, "unsupported") {
		t.Fatal("shape diagnostics missing")
	}
	// Act / Assert: replay does not flood ordinary live diagnostics.
	before := output.Len()
	logContentProjection(message, ref, "replay", normalizeContent(message))
	if output.Len() != before {
		t.Fatal("historical replay logged as live")
	}
}

func TestReactionDiagnosticsExposeOnlyBoundedShape(t *testing.T) {
	// Arrange: private values and attacker-controlled diagnostic fields.
	old := slog.Default()
	var output bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(old) })
	reaction := model.Reaction{Data: model.TReaction{Content: model.ReactionContent{
		ReactionData: model.ReactionData{RIcon: "private-icon"},
		Shape: model.ReactionContentShape{
			References:     "private-shape",
			Count:          999,
			PositiveGlobal: -1,
			UnknownFields:  999,
		},
	}}}
	activity := domain.Activity{
		ConversationType: "direct",
		ConversationID:   "private-peer",
		SourceID:         "private-action",
	}
	// Act.
	logReactionProjection(reaction, "live", activity)
	// Assert: no raw shape, identity or reaction; counters cannot grow unbounded.
	text := output.String()
	for _, private := range []string{"private-"} {
		if strings.Contains(text, private) {
			t.Fatal("unbounded or private reaction field logged")
		}
	}
	var fields map[string]any
	if err := json.Unmarshal(output.Bytes(), &fields); err != nil ||
		fields["reference_count"] != float64(16) ||
		fields["positive_global_refs"] != float64(0) {
		t.Fatal("diagnostic counters exceed their bounds")
	}
	if !strings.Contains(text, `"reference_shape":"unavailable"`) ||
		!strings.Contains(text, `"reference_count":16`) ||
		!strings.Contains(text, `"positive_global_refs":0`) {
		t.Fatal("bounded reaction shape missing")
	}
}

func TestMediaShapeLogExcludesValuesAndNestedMetadata(t *testing.T) {
	// Arrange: proven key vocabulary with adversarial values and one private key.
	previous := slog.Default()
	var output bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	message := model.TMessage{
		MsgType: "chat.photo",
		MsgID:   "private-message",
		UIDFrom: "private-actor",
		Content: model.Content{Attachment: &model.TAttachmentContent{
			Title:       "private-caption",
			Description: "private-caption",
			Action:      "PrivateSecret",
			Type:        "PrivateContactName",
			Href:        "https://private.invalid/signed-token",
			Params:      `{"desc":"private-caption","fileName":{"private-child":"private-filename"},"fileSize":987654321,"groupLayoutId":"private-layout-id","width":123456,"PrivateContactName":"private-value"}`,
		}},
	}
	// Act: the exact production logger receives the full native object.
	logContentProjection(
		message,
		domain.ConversationRef{Type: "direct", ID: "private-peer"},
		"live",
		normalizeContent(message),
	)
	// Assert: only fixed keys, type/emptiness and equality booleans survive.
	text := output.String()
	for _, value := range []string{"private-", "PrivateSecret", "PrivateContactName", "987654321", "123456", "https://"} {
		if strings.Contains(text, value) {
			t.Fatal("native media values leaked into shape log")
		}
	}
	var row map[string]any
	if err := json.Unmarshal(output.Bytes(), &row); err != nil {
		t.Fatal(err)
	}
	shape := row["content_shape"].(map[string]any)
	if shape["description_matches_title"] != true || shape["param_desc_matches_title"] != true {
		t.Fatal("bounded equality evidence missing")
	}
	fields := shape["param_shapes"].(map[string]any)
	if fields["fileName"].(map[string]any)["type"] != "object" ||
		fields["fileSize"].(map[string]any)["type"] != "number" {
		t.Fatal("parameter types lost")
	}
	for _, native := range []string{"PrivateSecret", "private.host.example"} {
		// Act: lexically valid private strings must not enter ordinary logs.
		output.Reset()
		message.MsgType = native
		logContentProjection(message, domain.ConversationRef{Type: "direct", ID: "private-peer"},
			"live", normalizeContent(message))
		// Assert: preserve presence while withholding the arbitrary type value.
		if strings.Contains(output.String(), native) {
			t.Fatal("private native type leaked into shape log")
		}
		var row map[string]any
		if err := json.Unmarshal(output.Bytes(), &row); err != nil ||
			row["native_type"] != "unknown" || row["native_type_present"] != true {
			t.Fatal("closed native type diagnostic missing")
		}
	}
}
