package mobilebackup

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
)

func TestQuoteUTF8PresenceOwnershipAndUnknown(t *testing.T) {
	// Arrange: literal nested stream, Unicode text with embedded NUL, empty attachment.
	input, _ := hex.DecodeString("0000005600000004c3a90078000000570000000000000058000000027b7d0000005a000000013100000059000000017a")
	original := append([]byte(nil), input...)
	// Act.
	got, err := ParseQuote(context.Background(), input)
	// Assert: presence distinct from absence, unsupported field counted, no interpretation.
	if err != nil || !got.Message.Present || !bytes.Equal(got.Message.Bytes, []byte{0xc3, 0xa9, 0, 'x'}) || !got.Attachment.Present || len(got.Attachment.Bytes) != 0 || string(got.FromD.Bytes) != "{}" || string(got.Status.Bytes) != "1" || got.UnsupportedFields != 1 || got.Scalars.OwnerID != nil {
		t.Fatal("quote fields mismatch", err)
	}
	b, _ := json.Marshal(got)
	if string(b) != "{}" || fmt.Sprintf("%#v", got) != "mobile backup quote [redacted]" || fmt.Sprintf("%#v", got.Message) != "mobile backup quote value [redacted]" {
		t.Fatal("quote fields exposed")
	}
	got.Message.Bytes[0] = 'q'
	if !bytes.Equal(input, original) {
		t.Fatal("quote aliases input")
	}
	owned := got.Message.Bytes
	got.Clear()
	if got.Message.Present || got.Message.Bytes != nil || !bytes.Equal(owned, make([]byte, len(owned))) {
		t.Fatal("quote bytes retained")
	}
}
func TestQuoteRejectsMalformedByteFieldsWithoutPartialResult(t *testing.T) {
	// Arrange: a valid ID prefix followed by invalid/duplicate UTF-8 byte fields.
	prefix := "00000051000000080020000000000001"
	for _, suffix := range []string{"0000005600000001ff", "0000005600000001780000005600000000", "0000005700000001c3", "0000005800000000ff"} {
		input, _ := hex.DecodeString(prefix + suffix)
		// Act.
		got, err := ParseQuote(context.Background(), input)
		// Assert: neither scalar nor byte prefix survives.
		if err == nil || got.Scalars.ClientMessageID != nil || got.Message.Present {
			t.Fatal("malformed quote accepted")
		}
	}
}
func FuzzQuoteWholeResult(f *testing.F) {
	f.Add([]byte{0, 0, 0, 86, 0, 0, 0, 1, 'x'})
	f.Fuzz(func(t *testing.T, input []byte) {
		got, err := ParseQuote(context.Background(), input)
		if err != nil {
			if got.Scalars.ClientMessageID != nil || got.Message.Present || got.Attachment.Present || got.FromD.Present || got.Status.Present || got.UnsupportedFields != 0 {
				t.Fatal("partial quote result")
			}
			return
		}
		got.Clear()
	})
}
