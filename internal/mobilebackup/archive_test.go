package mobilebackup

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
)

func syntheticContainer(files []File, compressed []byte) []byte {
	data := append(syntheticHeader(files), compressed...)
	binary.BigEndian.PutUint32(data[6:10], uint32(len(data)))
	binary.BigEndian.PutUint32(data[10:14], checksum32(data[14:]))
	return data
}

func TestPlainArchiveSplitsCompleteValidatedContainer(t *testing.T) {
	// Arrange: independently generated liblzma output; file bytes remain opaque.
	v := xzVectors(t)[2]
	plain := xzHex(t, v.Plaintext)
	compressed := xzHex(t, v.Compressed)
	if len(plain) < 2 {
		t.Fatal("fixture too short")
	}
	half := len(plain) / 2
	files := []File{{Name: "9007199254740993.db", Size: uint32(half)}, {Name: "group_9007199254740995.db", Size: uint32(len(plain) - half)}}
	input := syntheticContainer(files, compressed)
	// Act.
	archive, err := ReadPlainArchive(context.Background(), bytes.NewReader(input), uint64(len(input)), uint64(len(plain)))
	// Assert: exact boundaries/IDs, redaction and owned-buffer clearing.
	if err != nil || len(archive.Files) != 2 || archive.Files[0].Name != files[0].Name || !bytes.Equal(archive.Files[0].Data, plain[:half]) || !bytes.Equal(archive.Files[1].Data, plain[half:]) {
		t.Fatal("archive split failed", err)
	}
	serialized, _ := json.Marshal(archive)
	if string(serialized) != "{}" || fmt.Sprintf("%#v", archive) != "mobile backup plaintext archive [redacted]" {
		t.Fatal("archive exposed")
	}
	retained := archive.Files[0].Data
	archive.Clear()
	if archive.Files != nil || !bytes.Equal(retained, make([]byte, len(retained))) {
		t.Fatal("owned output retained")
	}
}

func TestPlainArchiveRejectsIncompleteAndAlteredContainer(t *testing.T) {
	v := xzVectors(t)[2]
	plain := xzHex(t, v.Plaintext)
	compressed := xzHex(t, v.Compressed)
	files := []File{{Name: "1.db", Size: uint32(len(plain))}}
	valid := syntheticContainer(files, compressed)
	corrupt := append([]byte(nil), valid...)
	corrupt[len(corrupt)-1] ^= 1
	invalidXZ := append([]byte(nil), compressed...)
	invalidXZ[0] ^= 1
	for _, input := range [][]byte{
		valid[:len(valid)-1], corrupt, append(append([]byte(nil), valid...), 0),
		syntheticContainer([]File{{Name: "1.db", Size: uint32(len(plain) + 1)}}, compressed),
		syntheticContainer(files, invalidXZ),
	} {
		// Act / Assert: no partially split result.
		archive, err := ReadPlainArchive(context.Background(), bytes.NewReader(input), MaxTotalBytes, MaxTotalBytes)
		if !errors.Is(err, ErrArchive) || archive.Files != nil {
			t.Fatal("invalid archive accepted")
		}
	}
	if _, err := ReadPlainArchive(context.Background(), bytes.NewReader(valid), uint64(len(valid)), uint64(len(plain)-1)); !errors.Is(err, ErrArchive) {
		t.Fatal("output budget bypass")
	}
}
