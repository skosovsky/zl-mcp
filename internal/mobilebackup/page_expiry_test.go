package mobilebackup

import (
	"bytes"
	"errors"
	"math"
	"testing"
)

func TestPageExpirySeparatesMessageAndQuote(t *testing.T) {
	// Arrange: expired row, non-expiring row with an expired quote, future row.
	old := candidateSQLiteRow("901", 0)
	old.TimestampMS = 1000
	old.TTL = 10
	stable := candidateSQLiteRow("902", 0)
	stable.TimestampMS = 1000
	future := candidateSQLiteRow("903", 0)
	future.TimestampMS = 1000
	future.TTL = 11
	ts, ttl := int64(1000), int64(10)
	quote := &Quote{Scalars: QuoteScalars{Timestamp: &ts, TTL: &ttl}, Message: QuoteValue{Present: true, Bytes: []byte("synthetic quote")}, Attachment: QuoteValue{Present: true, Bytes: []byte("synthetic attachment")}, FromD: QuoteValue{Present: true, Bytes: []byte("synthetic name")}, Status: QuoteValue{Present: true, Bytes: []byte("synthetic status")}}
	p := PreparedRowPage{Examined: 5, DeferredControls: 2, Rows: []PreparedRow{{Row: old}, {Row: stable, Metadata: BinNet{Quote: quote}}, {Row: future}}}
	removedBytes, quoteBytes := old.BinNet, quote.Message.Bytes
	quoteRawBytes := stable.BinNet
	// Act: exact eligibility boundary, not rounded client cleanup time.
	removed, quotes, err := p.ApplyExpiry(1010)
	// Assert: no resurrection or loss of the containing message/coverage metadata.
	if err != nil || removed != 1 || quotes != 1 || len(p.Rows) != 2 || p.Examined != 5 || p.DeferredControls != 2 || p.Rows[0].Row.SenderID != "902" || !p.Rows[1].ExpiryDeclared || p.Rows[1].ExpiresMS != 1011 {
		t.Fatal("wrong eligibility result", err)
	}
	if !bytes.Equal(removedBytes, make([]byte, len(removedBytes))) || !bytes.Equal(quoteBytes, make([]byte, len(quoteBytes))) || !bytes.Equal(quoteRawBytes, make([]byte, len(quoteRawBytes))) || !p.Rows[0].QuoteExpired || p.Rows[0].Row.BinNet != nil || quote.Message.Present || quote.Attachment.Present || quote.FromD.Present || !quote.Status.Present || *quote.Scalars.Timestamp != 1000 {
		t.Fatal("expired owned content retained or unrelated metadata removed")
	}
	// Repeated checks do not remove future messages or resurrect content.
	removed, quotes, err = p.ApplyExpiry(1010)
	if err != nil || removed != 0 || quotes != 0 || len(p.Rows) != 2 {
		t.Fatal("recheck altered survivors")
	}
	p.Clear()
}

func TestPageExpiryValidatesBeforeMutation(t *testing.T) {
	for _, mode := range []string{"negative", "missing-timestamp", "overflow", "invalid-clock"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: an expired first row must survive a failing later validation.
			first := candidateSQLiteRow("901", 0)
			first.TimestampMS = 1000
			first.TTL = 1
			second := candidateSQLiteRow("902", 0)
			ts, ttl := int64(1000), int64(1)
			quote := &Quote{Scalars: QuoteScalars{Timestamp: &ts, TTL: &ttl}}
			clock := int64(1010)
			switch mode {
			case "negative":
				ttl = -1
			case "missing-timestamp":
				quote.Scalars.Timestamp = nil
			case "overflow":
				ts = math.MaxInt64
			case "invalid-clock":
				clock = 0
			}
			p := PreparedRowPage{Rows: []PreparedRow{{Row: first}, {Row: second, Metadata: BinNet{Quote: quote}}}}
			original := append([]byte(nil), first.BinNet...)
			// Act.
			removed, quotes, err := p.ApplyExpiry(clock)
			// Assert: no successful prefix or private-buffer mutation.
			if !errors.Is(err, ErrExpiry) || removed != 0 || quotes != 0 || len(p.Rows) != 2 || !bytes.Equal(first.BinNet, original) {
				t.Fatal("failed page mutated")
			}
			p.Clear()
		})
	}
}
