package mobilebackup

import (
	"context"
	"encoding/binary"
)

// QuoteScalars preserves native signed values and absent fields without float conversion.
// It does not validate identities or convert them into a corpus quote.
type QuoteScalars struct {
	OwnerID         *int32 `json:"-"`
	ClientMessageID *int64 `json:"-"`
	GlobalMessageID *int64 `json:"-"`
	MessageType     *int32 `json:"-"`
	Timestamp       *int64 `json:"-"`
	TTL             *int64 `json:"-"`
}

func (QuoteScalars) String() string   { return "mobile backup quote scalars [redacted]" }
func (QuoteScalars) GoString() string { return "mobile backup quote scalars [redacted]" }
func (q *QuoteScalars) Clear() {
	if q == nil {
		return
	}
	for _, p := range []*int32{q.OwnerID, q.MessageType} {
		if p != nil {
			*p = 0
		}
	}
	for _, p := range []*int64{q.ClientMessageID, q.GlobalMessageID, q.Timestamp, q.TTL} {
		if p != nil {
			*p = 0
		}
	}
	*q = QuoteScalars{}
}
func ParseQuoteScalars(ctx context.Context, data []byte) (result QuoteScalars, err error) {
	fields, err := ParseTLV(ctx, data)
	if err != nil {
		return result, err
	}
	defer fields.Clear()
	defer func() {
		if err != nil {
			result.Clear()
		}
	}()
	seen := make(map[uint32]bool, 6)
	for _, f := range fields.Fields {
		if ctx.Err() != nil {
			return result, ErrTLV
		}
		if f.Tag < 80 || f.Tag > 85 {
			continue
		}
		if seen[f.Tag] {
			return result, ErrTLV
		}
		seen[f.Tag] = true
		width := 8
		if f.Tag == 80 || f.Tag == 83 {
			width = 4
		}
		if len(f.Value) != width {
			return result, ErrTLV
		}
		if width == 4 {
			v := int32(binary.BigEndian.Uint32(f.Value))
			if f.Tag == 80 {
				result.OwnerID = &v
			} else {
				result.MessageType = &v
			}
			continue
		}
		v := int64(binary.BigEndian.Uint64(f.Value))
		switch f.Tag {
		case 81:
			result.ClientMessageID = &v
		case 82:
			result.GlobalMessageID = &v
		case 84:
			result.Timestamp = &v
		case 85:
			result.TTL = &v
		}
	}
	if ctx.Err() != nil {
		return result, ErrTLV
	}
	return result, nil
}
