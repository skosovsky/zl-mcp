package mobilebackup

import (
	"context"
	"encoding/binary"
	"errors"
)

var ErrTLV = errors.New("invalid mobile backup TLV stream")

// TLV owns bounded uninterpreted field values, including repeats and unknown tags.
type TLV struct {
	Fields []TLVField `json:"-"`
}
type TLVField struct {
	Tag   uint32 `json:"-"`
	Value []byte `json:"-"`
}

func (TLV) String() string        { return "mobile backup TLV [redacted]" }
func (TLV) GoString() string      { return "mobile backup TLV [redacted]" }
func (TLVField) String() string   { return "mobile backup TLV field [redacted]" }
func (TLVField) GoString() string { return "mobile backup TLV field [redacted]" }
func (t *TLV) Clear() {
	if t == nil {
		return
	}
	for i := range t.Fields {
		clear(t.Fields[i].Value)
		t.Fields[i] = TLVField{}
	}
	t.Fields = nil
}

func ParseTLV(ctx context.Context, data []byte) (result TLV, err error) {
	if ctx == nil || ctx.Err() != nil || len(data) == 0 || len(data) > 256<<10 {
		return TLV{}, ErrTLV
	}
	defer func() {
		if err != nil {
			result.Clear()
			result = TLV{}
		}
	}()
	for len(data) > 0 {
		if ctx.Err() != nil || len(data) < 8 || len(result.Fields) >= 1024 {
			return result, ErrTLV
		}
		tag := binary.BigEndian.Uint32(data[:4])
		length := binary.BigEndian.Uint32(data[4:8])
		data = data[8:]
		if uint64(length) > uint64(len(data)) {
			return result, ErrTLV
		}
		value := append([]byte(nil), data[:int(length)]...)
		result.Fields = append(result.Fields, TLVField{Tag: tag, Value: value})
		data = data[int(length):]
	}
	if ctx.Err() != nil {
		return result, ErrTLV
	}
	return result, nil
}
