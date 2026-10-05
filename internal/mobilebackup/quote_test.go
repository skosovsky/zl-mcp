package mobilebackup

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
)

func TestQuoteScalarsExactSignedGolden(t *testing.T) {
	// Arrange: literal big-endian fields; client ID > 2^53, global ID MaxInt64.
	input, _ := hex.DecodeString("0000005000000004ffffffff0000005100000008002000000000000100000052000000087fffffffffffffff00000053000000040000000000000054000000080000018d4a5100000000005500000008fffffffffffffffe0000005600000003616263")
	// Act.
	got, err := ParseQuoteScalars(context.Background(), input)
	// Assert: raw signed values, zero presence and exact integers; no serialization.
	if err != nil || got.OwnerID == nil || *got.OwnerID != -1 || got.ClientMessageID == nil || *got.ClientMessageID != 9007199254740993 || got.GlobalMessageID == nil || *got.GlobalMessageID != 9223372036854775807 || got.MessageType == nil || *got.MessageType != 0 || got.Timestamp == nil || *got.Timestamp != 1706348838912 || got.TTL == nil || *got.TTL != -2 {
		t.Fatal("quote scalar mismatch", err)
	}
	b, _ := json.Marshal(got)
	if string(b) != "{}" || fmt.Sprintf("%#v", got) != "mobile backup quote scalars [redacted]" {
		t.Fatal("quote exposed")
	}
	owned := got.ClientMessageID
	got.Clear()
	if got.ClientMessageID != nil || *owned != 0 {
		t.Fatal("quote retained")
	}
}
func TestQuoteScalarsRejectPartialDuplicateAndWidth(t *testing.T) {
	// Arrange: valid scalar followed by malformed/duplicate fields.
	prefix := "00000051000000080020000000000001"
	for _, value := range []string{prefix + prefix, prefix + "0000005000000003000000", prefix + "0000005400000009000000000000000000", prefix + "00"} {
		input, _ := hex.DecodeString(value)
		// Act.
		got, err := ParseQuoteScalars(context.Background(), input)
		// Assert.
		if err == nil || got.ClientMessageID != nil {
			t.Fatal("invalid quote accepted")
		}
	}
	input, _ := hex.DecodeString("000000560000000178")
	got, err := ParseQuoteScalars(context.Background(), input)
	if err != nil || got.OwnerID != nil || got.ClientMessageID != nil {
		t.Fatal("absent scalar invented")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ParseQuoteScalars(ctx, input); err == nil {
		t.Fatal("cancelled quote accepted")
	}
}
