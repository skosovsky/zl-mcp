package mobilebackup

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func goldenHeader(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/format1-header.hex")
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.TrimSpace(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestChecksumIndependentReferenceVectors(t *testing.T) {
	// Arrange: generated once with upstream xxHash v0.8.3 C, not this package.
	raw, err := os.ReadFile("testdata/checksum-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []struct {
		Length   int
		Checksum uint32
	}
	if err = json.Unmarshal(raw, &vectors); err != nil {
		t.Fatal(err)
	}
	for _, v := range vectors {
		input := make([]byte, v.Length)
		for i := range input {
			input[i] = byte(i*37 + 11)
		}
		// Act / Assert
		if got := checksum32(input); got != v.Checksum {
			t.Fatalf("reference mismatch at length %d: %08x != %08x", v.Length, got, v.Checksum)
		}
	}
}

func TestTableGoldenPreservesExactNames(t *testing.T) {
	// Arrange: the old table-only fixture proves table layout, not a complete archive.
	body := goldenHeader(t)[14:]
	// Act.
	h, consumed, err := readFileTable(body)
	// Assert.
	if err != nil || len(h.Files) != 2 || h.Files[0].Name != "9007199254740993.db" || h.Files[1].Name != "group_9007199254740995.db" || h.TotalBytes != 1536 || consumed != len(body) {
		t.Fatal("table precision incorrect", err)
	}
}

func syntheticHeader(files []File) []byte {
	var body bytes.Buffer
	binary.Write(&body, binary.BigEndian, uint32(len(files)))
	for _, f := range files {
		binary.Write(&body, binary.BigEndian, uint32(len(f.Name)))
		body.WriteString(f.Name)
		binary.Write(&body, binary.BigEndian, f.Size)
	}
	prefix := make([]byte, 14)
	copy(prefix, "ZDB4.0")
	binary.BigEndian.PutUint32(prefix[6:10], uint32(14+body.Len()))
	binary.BigEndian.PutUint32(prefix[10:14], checksum32(body.Bytes()))
	return append(prefix, body.Bytes()...)
}

func TestHeaderRejectsMalformedWholeTable(t *testing.T) {
	for _, files := range [][]File{
		{}, {{Name: "../private-marker.db", Size: 512}}, {{Name: "/1.db", Size: 512}},
		{{Name: "1\\2.db", Size: 512}}, {{Name: "01.db", Size: 512}}, {{Name: "group_.db", Size: 512}},
		{{Name: "１.db", Size: 512}}, {{Name: "1.db", Size: 0}}, {{Name: "1.db", Size: uint32(MaxFileBytes + 1)}},
		{{Name: "1.db", Size: 512}, {Name: "1.db", Size: 512}},
		{{Name: "1.db", Size: uint32(MaxFileBytes)}, {Name: "2.db", Size: uint32(MaxFileBytes)}, {Name: "3.db", Size: 1}},
	} {
		// Arrange / Act
		h, _, err := readFileTable(syntheticHeader(files)[14:])
		// Assert: no successful prefix or private input retained in errors.
		if !errors.Is(err, ErrHeader) || h.Files != nil || h.TotalBytes != 0 || strings.Contains(err.Error(), "private-marker") {
			t.Fatal("invalid table accepted or error leaked")
		}
	}
}

func TestContainerRejectsTruncationCorruptionAndBudgetOverflow(t *testing.T) {
	// Arrange: declared length and checksum include the compressed payload.
	golden := syntheticHeader([]File{{Name: "1.db", Size: 512}})
	golden = append(golden, []byte("synthetic compressed payload")...)
	binary.BigEndian.PutUint32(golden[6:10], uint32(len(golden)))
	binary.BigEndian.PutUint32(golden[10:14], checksum32(golden[14:]))
	for n := 0; n < len(golden); n++ {
		// Act / Assert.
		if _, err := ReadContainer(context.Background(), bytes.NewReader(golden[:n]), uint64(len(golden))); !errors.Is(err, ErrHeader) {
			t.Fatalf("truncation %d accepted", n)
		}
	}
	for _, offset := range []int{18, len(golden) - 1} {
		corrupt := append([]byte(nil), golden...)
		corrupt[offset] ^= 1
		if _, err := ReadContainer(context.Background(), bytes.NewReader(corrupt), uint64(len(corrupt))); !errors.Is(err, ErrHeader) {
			t.Fatal("table/payload corruption accepted")
		}
	}
	reader := bytes.NewReader(golden)
	if _, err := ReadContainer(context.Background(), reader, uint64(len(golden)-1)); !errors.Is(err, ErrHeader) || reader.Len() != len(golden)-14 {
		t.Fatal("budget read beyond prefix")
	}
	extra := []byte("outer framing")
	reader = bytes.NewReader(append(append([]byte(nil), golden...), extra...))
	got, err := ReadContainer(context.Background(), reader, uint64(len(golden)))
	if err != nil || string(got.Compressed) != "synthetic compressed payload" || reader.Len() != len(extra) {
		t.Fatal("container boundary incorrect")
	}
	clear(got.Compressed)
	// A historical table-only fixture is not accepted as a complete container.
	if _, err := ReadContainer(context.Background(), bytes.NewReader(goldenHeader(t)), MaxTotalBytes); !errors.Is(err, ErrHeader) {
		t.Fatal("table-only fixture accepted as complete container")
	}
}
