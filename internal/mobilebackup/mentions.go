package mobilebackup

import (
	"context"
	"encoding/binary"
)

//nolint:recvcheck // Value String/GoString must redact formatted values; pointer methods mutate or clear owned state.
type Mention struct {
	Type, UID, Position, Length *int32 `json:"-"`
	UnsupportedFields           int    `json:"-"`
}

func (Mention) String() string   { return "mobile backup mention [redacted]" }
func (Mention) GoString() string { return "mobile backup mention [redacted]" }
func (m *Mention) Clear() {
	if m == nil {
		return
	}
	for _, p := range []*int32{m.Type, m.UID, m.Position, m.Length} {
		if p != nil {
			*p = 0
		}
	}
	*m = Mention{}
}

//nolint:nonamedreturns // Deferred cleanup observes the returned result/error to preserve ownership and panic/error semantics.
func ParseMention(ctx context.Context, data []byte) (result Mention, err error) {
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
	for _, f := range fields.Fields {
		if ctx.Err() != nil {
			return result, ErrTLV
		}
		var target **int32
		switch f.Tag {
		case 100:
			target = &result.Type
		case 101:
			target = &result.UID
		case 102:
			target = &result.Position
		case 103:
			target = &result.Length
		default:
			result.UnsupportedFields++
			continue
		}
		if *target != nil || len(f.Value) != 4 {
			return result, ErrTLV
		}
		//nolint:gosec // G115: reconstruct the signed two's-complement int32 wire value without changing bits.
		v := int32(binary.BigEndian.Uint32(f.Value))
		*target = &v
	}
	if ctx.Err() != nil {
		return result, ErrTLV
	}
	return result, nil
}
