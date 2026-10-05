package mobilebackup

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
)

func TestMentionRawSignedGoldenAndPresence(t *testing.T) {
	// Arrange: literal fields include signed negative, zero and an unknown tag.
	input, _ := hex.DecodeString("0000006400000004000000000000006500000004ffffffff000000660000000400000002000000670000000400000003000000680000000178")
	// Act.
	got, e := ParseMention(context.Background(), input)
	// Assert: preserve signed values/presence without guessing offset/ID meaning.
	if e != nil || got.Type == nil || *got.Type != 0 || got.UID == nil || *got.UID != -1 || got.Position == nil || *got.Position != 2 || got.Length == nil || *got.Length != 3 || got.UnsupportedFields != 1 {
		t.Fatal("mention mismatch", e)
	}
	b, _ := json.Marshal(got)
	if string(b) != "{}" || fmt.Sprintf("%#v", got) != "mobile backup mention [redacted]" {
		t.Fatal("mention exposed")
	}
	owned := got.UID
	got.Clear()
	if got.UID != nil || *owned != 0 {
		t.Fatal("mention retained")
	}
}
func TestBinNetRepeatedMentionsAndWholeFailure(t *testing.T) {
	// Arrange: two tag-8 values, explicit UID values in encounter order.
	input, _ := hex.DecodeString("000000080000000c00000065000000040000007b000000080000000c00000065000000040000007c")
	// Act.
	got, e := ParseBinNet(context.Background(), input)
	// Assert.
	if e != nil || len(got.Mentions) != 2 || *got.Mentions[0].UID != 123 || *got.Mentions[1].UID != 124 || got.UnsupportedFields != 0 {
		t.Fatal("mention order lost", e)
	}
	owned := got.Mentions[0].UID
	got.Clear()
	if got.Mentions != nil || *owned != 0 {
		t.Fatal("mentions retained")
	}
	// Arrange: valid mention followed by wrong width or duplicate known scalar.
	for _, suffix := range []string{"000000080000000b0000006500000003000001", "000000080000001800000065000000040000007b00000065000000040000007c"} {
		data, _ := hex.DecodeString("000000080000000c00000065000000040000007b" + suffix)
		// Act / Assert: valid prefix must not survive.
		result, e := ParseBinNet(context.Background(), data)
		if e == nil || result.Mentions != nil || result.Quote != nil {
			t.Fatal("partial mention stream")
		}
	}
}
