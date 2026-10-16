package zalo

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestOwnerContentShapeExcludesValuesAndPublicMessageJSON(t *testing.T) {
	// Arrange: protocol tokens plus private field values and unsafe keys.
	m := model.TMessage{Content: model.Content{Attachment: &model.TAttachmentContent{
		Action: "sendBubbleMessage", Type: "https://private.invalid/token", Description: "private-text",
		Params: `{"status":42,"private/unsafe/key":"private-value","callId":"private-id"}`,
	}}}
	// Act.
	shape := contentShape(m)
	body, err := json.Marshal(shape)
	// Assert: keys/tokens only; no raw text, URL, values or unsafe key.
	if err != nil || shape.ActionToken == nil || *shape.ActionToken != "sendBubbleMessage" || shape.TypeToken != nil ||
		shape.ParamsShape != "object" || !shape.KeysTruncated || strings.Join(shape.ParamsKeys, ",") != "callId,status" {
		t.Fatal("unexpected owner shape")
	}
	for _, private := range []string{"private-text", "private-value", "private-id", "private/unsafe", "https://"} {
		if strings.Contains(string(body), private) {
			t.Fatal("shape exposed native values")
		}
	}
	public, err := json.Marshal(domain.Message{ProtocolContentShape: shape})
	if err != nil || strings.Contains(string(public), "sendBubbleMessage") ||
		strings.Contains(string(public), "params_keys") {
		t.Fatal("ephemeral owner shape entered public message JSON")
	}
}

func TestOwnerContentShapeBoundsAndJSONShapes(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"", "absent"}, {"{", "invalid"}, {"[]", "array"}, {`"text"`, "string"}, {"1", "scalar"},
		{strings.Repeat("x", 65537), "oversized"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			// Arrange / Act.
			shape := contentShape(
				model.TMessage{Content: model.Content{Attachment: &model.TAttachmentContent{Params: tc.raw}}},
			)
			// Assert.
			if shape.ParamsShape != tc.want || len(shape.ParamsKeys) != 0 {
				t.Fatal("params shape/bounds changed")
			}
		})
	}
	// Arrange: many safe keys; values never leave the parser.
	keys := map[string]int{}
	for i := range 40 {
		keys[strings.Repeat("a", i+1)] = i
	}
	raw, err := json.Marshal(keys)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	shape := contentShape(
		model.TMessage{Content: model.Content{Attachment: &model.TAttachmentContent{Params: string(raw)}}},
	)
	// Assert.
	if len(shape.ParamsKeys) != 0 || !shape.KeysTruncated || shape.UnknownKeyCount != 40 {
		t.Fatal("unbounded owner key list")
	}
}

func TestOwnerContentShapeWithholdsSafeSpelledPrivateValues(t *testing.T) {
	// Arrange / Act: ordinary ASCII can still be private; lexical validity is insufficient.
	shape := contentShape(model.TMessage{Content: model.Content{Attachment: &model.TAttachmentContent{
		Action: "PrivateSecret", Type: "private.host.example", Params: `{"PrivateContactName":"private-value"}`,
	}}})
	// Assert.
	if shape.ActionToken != nil || shape.TypeToken != nil || !shape.ActionPresent || !shape.TypePresent ||
		len(shape.ParamsKeys) != 0 || shape.UnknownKeyCount != 1 || !shape.KeysTruncated {
		t.Fatal("private ASCII data passed the closed diagnostic vocabulary")
	}
}

func TestOwnerParamShapesDistinguishMissingNullAndEmpty(t *testing.T) {
	for _, tc := range []struct {
		raw   string
		kind  string
		empty *bool
	}{
		{`null`, "null", new(true)},
		{`""`, "string", new(true)},
		{`" "`, "string", new(false)},
		{`0`, "number", nil},
		{`1e400`, "number", nil},
		{`false`, "boolean", nil},
		{`[]`, "array", new(true)},
		{`["private"]`, "array", new(false)},
		{`{}`, "object", new(true)},
		{`{"private":"secret"}`, "object", new(false)},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			// Arrange: a present allowlisted parameter alongside private keys.
			message := model.TMessage{Content: model.Content{Attachment: &model.TAttachmentContent{
				Params: `{"callId":` + tc.raw + `,"private":"secret"}`,
			}}}
			// Act.
			shape := contentShape(message)
			got := shape.ParamShapes["callId"]
			body, err := json.Marshal(shape.ParamShapes)
			// Assert: only shape, absent fields omitted, numbers/booleans not absence.
			if got.Type != tc.kind || (got.Empty == nil) != (tc.empty == nil) ||
				(got.Empty != nil && *got.Empty != *tc.empty) || len(shape.ParamShapes) != 1 ||
				err != nil || strings.Contains(string(body), "private") || strings.Contains(string(body), "secret") {
				t.Fatal("owner parameter shape changed or leaked values")
			}
		})
	}
}

func TestOwnerContentShapeKnownActionVocabularyIsDiagnosticOnly(t *testing.T) {
	for _, action := range []string{"recommened.calltime", "recommened.misscall", "recommened.groupcall", "recommened.link", "recommened.user"} {
		t.Run(action, func(t *testing.T) {
			// Arrange: exact vocabulary grounded in the referenced forensic source.
			message := model.TMessage{Content: model.Content{Attachment: &model.TAttachmentContent{Action: action}}}
			// Act.
			shape := contentShape(message)
			// Assert: disclose only the allowlisted token; this does not normalize a call.
			if shape.ActionToken == nil || *shape.ActionToken != action {
				t.Fatal("known diagnostic discriminator was withheld")
			}
		})
	}
}
