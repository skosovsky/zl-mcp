package contracts

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

func TestSendContractsRejectInvalidRequestsAndUnconfirmedIDs(t *testing.T) {
	// Arrange.
	s, err := Compile("send_request", "input")
	if err != nil {
		t.Fatal(err)
	}
	r := map[string]any{"recipient_id": "peer", "request_id": "10000000-0000-4000-8000-000000000001", "text": strings.Repeat("界", 2048)}
	// Act / Assert.
	if err = s.Validate(r); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{" ", strings.Repeat("界", 2049)} {
		r["text"] = text
		if s.Validate(r) == nil {
			t.Fatal("invalid text accepted")
		}
	}
	o, err := Compile("send_operation", "output")
	if err != nil {
		t.Fatal(err)
	}
	op := map[string]any{"request_id": r["request_id"], "recipient_id": "peer", "status": "unknown", "message_id": nil, "reason": "interrupted", "updated_at": "2026-10-03T12:00:00Z", "retry_safe": false}
	if err = o.Validate(op); err != nil {
		t.Fatal(err)
	}
	op["message_id"] = "unconfirmed"
	if o.Validate(op) == nil {
		t.Fatal("unknown operation claimed message ID")
	}
	op["status"] = "sent"
	op["reason"] = nil
	if err = o.Validate(op); err != nil {
		t.Fatal(err)
	}
}

func TestSendTextPatternSupportsFullMatchConnectorValidation(t *testing.T) {
	for _, name := range []string{"send_request", "zalo_send_direct_message"} {
		// Arrange: connector validators may apply the pattern to the whole string.
		raw, err := Files.ReadFile(name + ".input.json")
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Properties map[string]struct{ Pattern string }
		}
		if err = json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		pattern, err := regexp.Compile(document.Properties["text"].Pattern)
		if err != nil {
			t.Fatal(err)
		}
		schema, err := Compile(name, "input")
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"hello there", " Тест сообщения. ", "line one\nline two", "\n界\n"} {
			// Act.
			span := pattern.FindStringIndex(text)
			validation := schema.Validate(map[string]any{"recipient_id": "12", "request_id": "00000000-0000-4000-8000-000000000031", "text": text})
			// Assert: accepted text spans the whole string, including whitespace/newlines.
			if span == nil || span[0] != 0 || span[1] != len(text) || validation != nil {
				t.Fatal("valid text rejected", name, validation)
			}
		}
		for _, text := range []string{"", " ", "\n\t\r"} {
			if pattern.MatchString(text) {
				t.Fatal("blank text accepted")
			}
		}
	}
}
