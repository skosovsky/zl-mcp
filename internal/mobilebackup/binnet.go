package mobilebackup

import "context"

// BinNet retains partial quote/mention/attachment decoding and unsupported field coverage.
// Recognition of all encountered tags does not validate message semantics.
type BinNet struct {
	Mentions          []Mention    `json:"-"`
	Quote             *Quote       `json:"-"`
	Attachments       []Attachment `json:"-"`
	UnsupportedTags   []uint32     `json:"-"`
	UnsupportedFields int          `json:"-"`
}

func (BinNet) String() string   { return "mobile backup BinNet [redacted]" }
func (BinNet) GoString() string { return "mobile backup BinNet [redacted]" }
func (b *BinNet) Clear() {
	if b == nil {
		return
	}
	if b.Quote != nil {
		b.Quote.Clear()
	}
	for i := range b.Attachments {
		b.Attachments[i].Clear()
	}
	for i := range b.Mentions {
		b.Mentions[i].Clear()
	}
	clear(b.UnsupportedTags)
	*b = BinNet{}
}
func ParseBinNet(ctx context.Context, data []byte) (result BinNet, err error) {
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
		if f.Tag == 6 {
			attachment, e := ParseAttachment(ctx, f.Value)
			if e != nil {
				return result, e
			}
			result.Attachments = append(result.Attachments, attachment)
			result.UnsupportedFields += attachment.UnsupportedFields
			continue
		}
		if f.Tag == 8 {
			mention, e := ParseMention(ctx, f.Value)
			if e != nil {
				return result, e
			}
			result.Mentions = append(result.Mentions, mention)
			result.UnsupportedFields += mention.UnsupportedFields
			continue
		}
		if f.Tag != 7 {
			result.UnsupportedTags = append(result.UnsupportedTags, f.Tag)
			result.UnsupportedFields++
			continue
		}
		if result.Quote != nil {
			return result, ErrTLV
		}
		quote, e := ParseQuote(ctx, f.Value)
		if e != nil {
			return result, e
		}
		result.Quote = &quote
		result.UnsupportedFields += quote.UnsupportedFields
	}
	if ctx.Err() != nil {
		return result, ErrTLV
	}
	return result, nil
}
