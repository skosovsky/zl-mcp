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
		*v = AttachmentValue{}
	}
	for _, p := range []*int32{a.CategoryID, a.ID, a.ChildNumber} {
		if p != nil {
			*p = 0
		}
	}
	*a = Attachment{}
}

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
		case 40:
			text = &result.Type
		case 41:
			scalar = &result.CategoryID
		case 42:
			scalar = &result.ID
		case 43:
			text = &result.ExtInfo
		case 44:
			scalar = &result.ChildNumber
		case 45:
			text = &result.Action
		case 46:
			text = &result.Params
		case 47:
			text = &result.Title
		case 48:
			text = &result.Href
		case 49:
			text = &result.Thumb
		case 50:
			text = &result.Description
		case 66:
			text = &result.Remains
		case 67:
			text = &result.ZInstantData
		case 68:
			text = &result.ZInstantMessage
		default:
			result.UnsupportedFields++
			continue
		}
		if scalar != nil {
			if *scalar != nil || len(f.Value) != 4 {
				return result, ErrTLV
			}
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
