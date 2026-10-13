package mobilebackup

import (
	"context"
	"unicode/utf8"
)

// QuoteValue retains explicit string presence and owns raw, valid UTF-8 bytes.
type QuoteValue struct {
	Present bool   `json:"-"`
	Bytes   []byte `json:"-"`
}

func (QuoteValue) String() string   { return "mobile backup quote value [redacted]" }
func (QuoteValue) GoString() string { return "mobile backup quote value [redacted]" }

// Quote is a partial native metadata decode, never a validated corpus quote.
//
//nolint:recvcheck // Value String/GoString must redact formatted values; pointer methods mutate or clear owned state.
type Quote struct {
	Scalars                            QuoteScalars `json:"-"`
	Message, Attachment, FromD, Status QuoteValue   `json:"-"`
	UnsupportedFields                  int          `json:"-"`
}

func (Quote) String() string   { return "mobile backup quote [redacted]" }
func (Quote) GoString() string { return "mobile backup quote [redacted]" }
func (q *Quote) Clear() {
	if q == nil {
		return
	}
	q.Scalars.Clear()
	for _, f := range []*QuoteValue{&q.Message, &q.Attachment, &q.FromD, &q.Status} {
		clear(f.Bytes)
		//nolint:exhaustruct_v5 // Clearing owned state resets every field to its zero value, including nil slices and pointers.
		*f = QuoteValue{}
	}
	//nolint:exhaustruct_v5 // Clearing owned state resets every field to its zero value, including nil slices and pointers.
	*q = Quote{}
}

//nolint:nonamedreturns // Deferred cleanup observes the returned result/error to preserve ownership and panic/error semantics.
func ParseQuote(ctx context.Context, data []byte) (result Quote, err error) {
	result.Scalars, err = ParseQuoteScalars(ctx, data)
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			result.Clear()
		}
	}()
	fields, err := ParseTLV(ctx, data)
	if err != nil {
		return result, err
	}
	defer fields.Clear()
	for _, f := range fields.Fields {
		if ctx.Err() != nil {
			return result, ErrTLV
		}
		if f.Tag >= quoteOwnerIDTag && f.Tag <= quoteTTLTag {
			continue
		}
		var target *QuoteValue
		switch f.Tag {
		case quoteMessageTag:
			target = &result.Message
		case quoteAttachmentTag:
			target = &result.Attachment
		case quoteFromDTag:
			target = &result.FromD
		case quoteStatusTag:
			target = &result.Status
		default:
			result.UnsupportedFields++
			continue
		}
		if target.Present || !utf8.Valid(f.Value) {
			return result, ErrTLV
		}
		target.Present = true
		target.Bytes = append([]byte(nil), f.Value...)
	}
	if ctx.Err() != nil {
		return result, ErrTLV
	}
	return result, nil
}
