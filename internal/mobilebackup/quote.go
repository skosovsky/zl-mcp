package mobilebackup

import (
	"context"
	"encoding/binary"
)

// QuoteScalars preserves native signed values and absent fields without float conversion.
// It does not validate identities or convert them into a corpus quote.
//
//nolint:recvcheck // Value String/GoString must redact formatted values; pointer methods mutate or clear owned state.
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
	//nolint:exhaustruct_v5 // Clearing owned state resets every field to its zero value, including nil slices and pointers.
	*q = QuoteScalars{}
}

//nolint:gocognit,nonamedreturns // The closed quote scalar wire alternatives retain their explicit bounds and malformed-input precedence in one parser. Deferred cleanup retains the returned result/error.
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
	seen := make(map[uint32]bool, quoteScalarFieldCount)
	for _, f := range fields.Fields {
		if ctx.Err() != nil {
			return result, ErrTLV
		}
		if f.Tag < quoteOwnerIDTag || f.Tag > quoteTTLTag {
			continue
		}
		if seen[f.Tag] {
			return result, ErrTLV
		}
		seen[f.Tag] = true
		width := nativeInt64Bytes
		if f.Tag == quoteOwnerIDTag || f.Tag == quoteMessageTypeTag {
			width = nativeInt32Bytes
		}
		if len(f.Value) != width {
			return result, ErrTLV
		}
		if width == nativeInt32Bytes {
			//nolint:gosec // G115: reconstruct signed two's-complement wire scalars without changing their bits.
			v := int32(binary.BigEndian.Uint32(f.Value))
			if f.Tag == quoteOwnerIDTag {
				result.OwnerID = &v
			} else {
				result.MessageType = &v
			}
			continue
		}
		//nolint:gosec // G115: reconstruct signed two's-complement wire scalars without changing their bits.
		v := int64(binary.BigEndian.Uint64(f.Value))
		switch f.Tag {
		case quoteClientMessageIDTag:
			result.ClientMessageID = &v
		case quoteGlobalMessageIDTag:
			result.GlobalMessageID = &v
		case quoteTimestampTag:
			result.Timestamp = &v
		case quoteTTLTag:
			result.TTL = &v
		}
	}
	if ctx.Err() != nil {
		return result, ErrTLV
	}
	return result, nil
}
