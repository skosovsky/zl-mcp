package mobilebackup

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
)

func TestBinNetQuoteAndExplicitPartialCoverage(t *testing.T) {
	// Arrange: quote with exact large client ID/text/unknown field, repeated unknown fields.
	input, _ := hex.DecodeString("00000007000000220000005100000008002000000000000100000056000000017800000059000000017a0000000a00000001610000000a0000000162000000090000000163")
	// Act.
	got, err := ParseBinNet(context.Background(), input)
	// Assert: nested quote kept, unsupported occurrences never silently lost.
	if err != nil || got.Quote == nil || got.Quote.Scalars.ClientMessageID == nil || *got.Quote.Scalars.ClientMessageID != 9007199254740993 || string(got.Quote.Message.Bytes) != "x" || got.UnsupportedFields != 4 || len(got.UnsupportedTags) != 3 || got.UnsupportedTags[0] != 10 || got.UnsupportedTags[1] != 10 || got.UnsupportedTags[2] != 9 {
		t.Fatal("BinNet coverage mismatch", err)
	}
	b, _ := json.Marshal(got)
	if string(b) != "{}" || fmt.Sprintf("%#v", got) != "mobile backup BinNet [redacted]" {
		t.Fatal("BinNet exposed")
	}
	quote := got.Quote
	tags := got.UnsupportedTags
	got.Clear()
	if got.Quote != nil || quote.Scalars.ClientMessageID != nil || quote.Message.Present || tags[0] != 0 {
		t.Fatal("BinNet retained")
	}
}
func TestBinNetWholeResultFailureAndUnknownOnly(t *testing.T) {
	quote := "000000070000001000000051000000080020000000000001"
	// Arrange: duplicate quote, malformed nested field, malformed suffix, empty quote.
	for _, inputHex := range []string{quote + quote, quote + "00", "00000006000000017800000007000000090000005600000001ff", "0000000700000000"} {
		input, _ := hex.DecodeString(inputHex)
		// Act.
		got, err := ParseBinNet(context.Background(), input)
		// Assert: no partial metadata survives.
		if err == nil || got.Quote != nil || got.Attachments != nil || got.Mentions != nil || got.UnsupportedTags != nil || got.UnsupportedFields != 0 {
			t.Fatal("partial BinNet accepted")
		}
	}
	input, _ := hex.DecodeString("ffffffff0000000178")
	got, err := ParseBinNet(context.Background(), input)
	if err != nil || got.Quote != nil || got.UnsupportedFields != 1 || got.UnsupportedTags[0] != 0xffffffff {
		t.Fatal("unknown metadata misreported")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ParseBinNet(ctx, input); err == nil {
		t.Fatal("cancelled BinNet accepted")
	}
}
func FuzzBinNetWholeResult(f *testing.F) {
	f.Add([]byte{0, 0, 0, 6, 0, 0, 0, 1, 'x'})
	f.Fuzz(func(t *testing.T, data []byte) {
		got, err := ParseBinNet(context.Background(), data)
		if err != nil {
			if got.Quote != nil || got.Attachments != nil || got.Mentions != nil || got.UnsupportedTags != nil || got.UnsupportedFields != 0 {
				t.Fatal("partial BinNet")
			}
			return
		}
		if len(got.UnsupportedTags) > 1024 || got.UnsupportedFields > len(data)/8 {
			t.Fatal("metadata bound exceeded")
		}
		got.Clear()
	})
}
