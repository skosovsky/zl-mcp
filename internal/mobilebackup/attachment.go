package mobilebackup

import (
	"context"
	"encoding/binary"
	"unicode/utf8"
)

// AttachmentValue distinguishes absent data from an explicitly empty native string.
type AttachmentValue struct {
	Present bool   `json:"-"`
	Bytes   []byte `json:"-"`
}

func (AttachmentValue) String() string   { return "mobile backup attachment value [redacted]" }
func (AttachmentValue) GoString() string { return "mobile backup attachment value [redacted]" }

// Attachment is private metadata, not a rendered message or permission to fetch a URL.
//
//nolint:recvcheck // Value String/GoString must redact formatted values; pointer methods mutate or clear owned state.
type Attachment struct {
	Type, ExtInfo, Action, Params, Title, Href, Thumb, Description AttachmentValue `json:"-"`
	Remains, ZInstantData, ZInstantMessage                         AttachmentValue `json:"-"`
	CategoryID, ID, ChildNumber                                    *int32          `json:"-"`
	UnsupportedFields                                              int             `json:"-"`
}

func (Attachment) String() string   { return "mobile backup attachment [redacted]" }
func (Attachment) GoString() string { return "mobile backup attachment [redacted]" }
func (a *Attachment) Clear() {
	if a == nil {
		return
	}
	for _, v := range []*AttachmentValue{&a.Type, &a.ExtInfo, &a.Action, &a.Params, &a.Title, &a.Href, &a.Thumb, &a.Description, &a.Remains, &a.ZInstantData, &a.ZInstantMessage} {
		clear(v.Bytes)
		//nolint:exhaustruct_v5 // Clearing owned state resets every field to its zero value, including nil slices and pointers.
		*v = AttachmentValue{}
	}
	for _, p := range []*int32{a.CategoryID, a.ID, a.ChildNumber} {
		if p != nil {
			*p = 0
		}
	}
	//nolint:exhaustruct_v5 // Clearing owned state resets every field to its zero value, including nil slices and pointers.
	*a = Attachment{}
}

//nolint:funlen,nonamedreturns // The bounded TLV tag-to-field table retains explicit scalar/text ownership and Clear-on-error in one protocol parser. Returned result/error and deferred cleanup keep the same owner.
func ParseAttachment(ctx context.Context, data []byte) (result Attachment, err error) {
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
		var scalar **int32
		var text *AttachmentValue
		switch f.Tag {
		case attachmentTypeTag:
			text = &result.Type
		case attachmentCategoryIDTag:
			scalar = &result.CategoryID
		case attachmentIDTag:
			scalar = &result.ID
		case attachmentExtInfoTag:
			text = &result.ExtInfo
		case attachmentChildNumberTag:
			scalar = &result.ChildNumber
		case attachmentActionTag:
			text = &result.Action
		case attachmentParamsTag:
			text = &result.Params
		case attachmentTitleTag:
			text = &result.Title
		case attachmentHrefTag:
			text = &result.Href
		case attachmentThumbTag:
			text = &result.Thumb
		case attachmentDescriptionTag:
			text = &result.Description
		case attachmentRemainsTag:
			text = &result.Remains
		case attachmentInstantDataTag:
			text = &result.ZInstantData
		case attachmentInstantMessageTag:
			text = &result.ZInstantMessage
		default:
			result.UnsupportedFields++
			continue
		}
		if scalar != nil {
			if *scalar != nil || len(f.Value) != nativeInt32Bytes {
				return result, ErrTLV
			}
			//nolint:gosec // G115: reconstruct the signed two's-complement int32 wire value without changing bits.
			v := int32(binary.BigEndian.Uint32(f.Value))
			*scalar = &v
		} else {
			if text.Present || !utf8.Valid(f.Value) {
				return result, ErrTLV
			}
			text.Present = true
			text.Bytes = append([]byte(nil), f.Value...)
		}
	}
	if ctx.Err() != nil {
		return result, ErrTLV
	}
	return result, nil
}
