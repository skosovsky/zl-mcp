package mobilebackup

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func attachmentField(tag uint32, value []byte) []byte {
	data := make([]byte, 8+len(value))
	binary.BigEndian.PutUint32(data, tag)
	binary.BigEndian.PutUint32(data[4:], uint32(len(value)))
	copy(data[8:], value)
	return data
}
func TestAttachmentNativeFieldsPresenceOwnershipAndRedaction(t *testing.T) {
	// Arrange: all native byte fields, signed/zero scalar values and two unknown occurrences.
	tags := []uint32{40, 43, 45, 46, 47, 48, 49, 50, 66, 67, 68}
	var input []byte
	for _, tag := range tags {
		input = append(input, attachmentField(tag, []byte(fmt.Sprintf("value-%d", tag)))...)
	}
	for _, tag := range []uint32{41, 42, 44} {
		value := []byte{0xff, 0xff, 0xff, 0xff}
		if tag == 42 {
			value = []byte{0, 0, 0, 0}
		}
		if tag == 44 {
			value = []byte{0x01, 0x02, 0x03, 0x04}
		}
		input = append(input, attachmentField(tag, value)...)
	}
	input = append(input, attachmentField(99, []byte("opaque"))...)
	input = append(input, attachmentField(99, nil)...)
	original := bytes.Clone(input)
	// Act.
	got, err := ParseAttachment(context.Background(), input)
	// Assert: exact signed values and owned UTF-8 bytes; unknown occurrences remain visible.
	if err != nil || got.CategoryID == nil || *got.CategoryID != -1 || got.ID == nil || *got.ID != 0 || got.ChildNumber == nil || *got.ChildNumber != 16909060 || got.UnsupportedFields != 2 {
		t.Fatal("native scalars or coverage changed", err)
	}
	values := []*AttachmentValue{&got.Type, &got.ExtInfo, &got.Action, &got.Params, &got.Title, &got.Href, &got.Thumb, &got.Description, &got.Remains, &got.ZInstantData, &got.ZInstantMessage}
	for i, v := range values {
		if !v.Present || string(v.Bytes) != fmt.Sprintf("value-%d", tags[i]) {
			t.Fatal("field mapping lost")
		}
	}
	if !bytes.Equal(input, original) {
		t.Fatal("borrowed input changed")
	}
	clear(input)
	if string(got.Title.Bytes) != "value-47" {
		t.Fatal("result borrowed source bytes")
	}
	for _, v := range []any{got, &got, got.Title, &got.Title} {
		encoded, e := json.Marshal(v)
		if e != nil || string(encoded) != "{}" || bytes.Contains([]byte(fmt.Sprintf("%#v", v)), []byte("value-")) {
			t.Fatal("private metadata exposed")
		}
	}
	title := got.Title.Bytes
	id := got.CategoryID
	got.Clear()
	if !reflect.DeepEqual(got, Attachment{}) || *id != 0 || !bytes.Equal(title, make([]byte, len(title))) {
		t.Fatal("owned data retained")
	}
	empty, err := ParseAttachment(context.Background(), attachmentField(47, nil))
	if err != nil || !empty.Title.Present || empty.Action.Present || empty.ID != nil {
		t.Fatal("empty and absent conflated")
	}
	empty.Clear()
}
func TestAttachmentRejectsMalformedWholeResult(t *testing.T) {
	// Arrange: valid prefix before every malformed tail, including duplicate known values.
	prefix := attachmentField(47, []byte("private-synthetic-marker"))
	for _, tail := range [][]byte{
		{0}, attachmentField(45, []byte{0xff}), attachmentField(41, []byte{1}), attachmentField(42, make([]byte, 8)), attachmentField(47, []byte("second")),
		append(attachmentField(44, make([]byte, 4)), attachmentField(44, make([]byte, 4))...),
	} {
		input := append(bytes.Clone(prefix), tail...)
		// Act.
		got, err := ParseAttachment(context.Background(), input)
		// Assert: no metadata prefix escapes a malformed complete block.
		if err == nil || !reflect.DeepEqual(got, Attachment{}) {
			t.Fatal("partial attachment survived", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, err := ParseAttachment(ctx, prefix); err == nil || !reflect.DeepEqual(got, Attachment{}) {
		t.Fatal("cancelled decode accepted")
	}
}
func TestBinNetAttachmentDecodeDoesNotGrantConversion(t *testing.T) {
	// Arrange: valid rich-text attachment remains a content gap until rendering is verified.
	nested := append(attachmentField(45, []byte("rtf")), attachmentField(47, []byte("synthetic title"))...)
	input := attachmentField(6, nested)
	metadata, err := ParseBinNet(context.Background(), input)
	if err != nil || len(metadata.Attachments) != 1 || string(metadata.Attachments[0].Action.Bytes) != "rtf" || metadata.UnsupportedFields != 0 {
		t.Fatal("nested attachment not decoded", err)
	}
	page, r, now := conversionPage(t)
	defer page.Clear()
	page.Candidates.Rows[0].Metadata.Clear()
	page.Candidates.Rows[0].Metadata = metadata
	// Act.
	converted, err := ConvertPreparedArchivePage(context.Background(), page, r, "10", now)
	defer converted.Clear()
	// Assert: parsed metadata is never silently treated as plain MsgContent.
	if err != nil || converted.UnsupportedContent != 1 || len(converted.Records) != 1 {
		t.Fatal("attachment flattened into plain text", err)
	}
	duplicate := append(bytes.Clone(input), input...)
	got, err := ParseBinNet(context.Background(), duplicate)
	if err != nil || len(got.Attachments) != 2 || string(got.Attachments[0].Title.Bytes) != "synthetic title" || string(got.Attachments[1].Title.Bytes) != "synthetic title" {
		t.Fatal("repeated attachment lost", err)
	}
	got.Clear()
}

func TestBinNetMalformedQuoteClearsPrecedingAttachments(t *testing.T) {
	// Arrange: a valid owned attachment followed by a structurally valid but invalid quote value.
	attachment := attachmentField(6, attachmentField(47, []byte("private-synthetic-title")))
	quote := attachmentField(7, attachmentField(86, []byte{0xff}))
	input := append(bytes.Clone(attachment), quote...)
	original := bytes.Clone(input)
	// Act.
	got, err := ParseBinNet(context.Background(), input)
	// Assert: nested failure discards previously decoded data, without modifying borrowed input.
	if err == nil || got.Attachments != nil || got.Quote != nil || got.Mentions != nil || got.UnsupportedFields != 0 || !bytes.Equal(input, original) {
		t.Fatal("malformed quote exposed attachment prefix", err)
	}
}
func FuzzAttachmentWholeResult(f *testing.F) {
	f.Add(attachmentField(45, []byte("rtf")))
	f.Add(attachmentField(41, []byte{0xff, 0xff, 0xff, 0xff}))
	f.Fuzz(func(t *testing.T, input []byte) {
		got, err := ParseAttachment(context.Background(), input)
		if err != nil && !reflect.DeepEqual(got, Attachment{}) {
			t.Fatal("partial metadata")
		}
		if got.UnsupportedFields > 1024 {
			t.Fatal("field budget exceeded")
		}
		got.Clear()
	})
}
