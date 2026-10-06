package mobilebackup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

type format1Vector struct {
	Name, Ciphertext, Plaintext, Filename string
	ContainerBytes                        uint64 `json:"container_bytes"`
	TrailingBytes                         uint64 `json:"trailing_bytes"`
	SHA                                   string `json:"ciphertext_sha256"`
}

func archiveVectors(t *testing.T) []format1Vector {
	t.Helper()
	data, e := os.ReadFile("testdata/format1-archive-vectors.json")
	if e != nil {
		t.Fatal(e)
	}
	var v []format1Vector
	if e = json.Unmarshal(data, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func TestFormat1ArchiveIndependentFullVectors(t *testing.T) {
	for _, v := range archiveVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			// Arrange: OpenSSL CBC + liblzma XZ + official XXH32, independently assembled.
			encrypted, _ := hex.DecodeString(v.Ciphertext)
			want, _ := hex.DecodeString(v.Plaintext)
			digest := sha256.Sum256(encrypted)
			if hex.EncodeToString(digest[:]) != v.SHA {
				t.Fatal("fixture digest changed")
			}
			// Act.
			got, e := ReadFormat1Archive(context.Background(), bytes.NewReader(encrypted), strings.Repeat("0123456789abcdef", 4), uint64(len(encrypted)), uint64(len(want)))
			// Assert: declared region is exact; zero/nonzero tail is counted, not unpadded.
			if e != nil || len(got.Archive.Files) != 1 || got.Archive.Files[0].Name != v.Filename || !bytes.Equal(got.Archive.Files[0].Data, want) || got.CiphertextBytes != uint64(len(encrypted)) || got.ContainerBytes != v.ContainerBytes || got.TrailingBytes != v.TrailingBytes {
				t.Fatal("full archive mismatch", e)
			}
			b, _ := json.Marshal(got)
			if string(b) != "{}" || fmt.Sprintf("%#v", got) != "mobile backup encrypted archive [redacted]" {
				t.Fatal("archive exposed")
			}
			owned := got.Archive.Files[0].Data
			got.Clear()
			if got.Archive.Files != nil || got.ContainerBytes != 0 || !bytes.Equal(owned, make([]byte, len(owned))) {
				t.Fatal("archive retained")
			}
		})
	}
}
func TestFormat1ArchiveFailsWithoutPartialFilesOrCounts(t *testing.T) {
	// Arrange: known valid independent encrypted archive.
	v := archiveVectors(t)[1]
	encrypted, _ := hex.DecodeString(v.Ciphertext)
	for _, tc := range []struct {
		data          []byte
		limit, output uint64
		key           string
	}{
		{encrypted, uint64(len(encrypted) - 1), MaxTotalBytes, strings.Repeat("0123456789abcdef", 4)},
		{encrypted, uint64(len(encrypted)), 1, strings.Repeat("0123456789abcdef", 4)},
		{encrypted[:len(encrypted)-1], MaxTotalBytes, MaxTotalBytes, strings.Repeat("0123456789abcdef", 4)},
		{encrypted, MaxTotalBytes, MaxTotalBytes, strings.Repeat("f", 64)},
	} {
		// Act.
		got, e := ReadFormat1Archive(context.Background(), bytes.NewReader(tc.data), tc.key, tc.limit, tc.output)
		// Assert.
		if e == nil || got.Archive.Files != nil || got.ContainerBytes != 0 || got.CiphertextBytes != 0 || got.TrailingBytes != 0 {
			t.Fatal("invalid archive retained prefix")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, e := ReadFormat1Archive(ctx, bytes.NewReader(encrypted), strings.Repeat("0123456789abcdef", 4), MaxTotalBytes, MaxTotalBytes); e == nil || got.Archive.Files != nil {
		t.Fatal("cancelled archive accepted")
	}
}

func TestFormat1SQLiteIndependentVector(t *testing.T) {
	// Arrange: independently generated SQLite, liblzma, xxHash and OpenSSL archive.
	raw, e := os.ReadFile("testdata/format1-sqlite-vector.json")
	if e != nil {
		t.Fatal(e)
	}
	var v struct {
		Ciphertext, Filename string
		SHA                  string `json:"plaintext_sha256"`
	}
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	data, e := hex.DecodeString(v.Ciphertext)
	if e != nil {
		t.Fatal(e)
	}
	// Act.
	archive, e := ReadFormat1Archive(context.Background(), bytes.NewReader(data), strings.Repeat("0123456789abcdef", 4), MaxTotalBytes, MaxTotalBytes)
	defer archive.Clear()
	// Assert: SQLite payload exact, not merely a compatible encrypt/decrypt roundtrip.
	if e != nil || len(archive.Archive.Files) != 1 {
		t.Fatal("independent SQLite archive rejected", e)
	}
	file := archive.Archive.Files[0]
	digest := sha256.Sum256(file.Data)
	if file.Name != v.Filename || hex.EncodeToString(digest[:]) != v.SHA {
		t.Fatal("SQLite fixture payload mismatch")
	}
}

func TestFormat1ArchivePartialTailOutsideVerifiedContainer(t *testing.T) {
	// Arrange: independently encrypted complete archive followed by 1..15 opaque
	// physical tail bytes. The tail is outside the declared verified container.
	v := archiveVectors(t)[1]
	encrypted, _ := hex.DecodeString(v.Ciphertext)
	want, _ := hex.DecodeString(v.Plaintext)
	for tail := 1; tail < 16; tail++ {
		input := append(append([]byte(nil), encrypted...), bytes.Repeat([]byte{0x93}, tail)...)
		// Act.
		got, err := ReadFormat1Archive(context.Background(), bytes.NewReader(input), strings.Repeat("0123456789abcdef", 4), uint64(len(input)), uint64(len(want)))
		// Assert: exact contents/checksum remain required, physical count includes tail.
		if err != nil || len(got.Archive.Files) != 1 || !bytes.Equal(got.Archive.Files[0].Data, want) || got.CiphertextBytes != uint64(len(input)) || got.TrailingBytes != uint64(len(input))-v.ContainerBytes {
			t.Fatal("bounded physical tail rejected", tail, err)
		}
		got.Clear()
	}
	// Arrange/Act/Assert: an incomplete AES block needed by the container is rejected.
	incomplete := encrypted[:int(v.ContainerBytes)-1]
	got, err := ReadFormat1Archive(context.Background(), bytes.NewReader(incomplete), strings.Repeat("0123456789abcdef", 4), uint64(len(incomplete)), uint64(len(want)))
	if err == nil || len(got.Archive.Files) != 0 {
		t.Fatal("missing container ciphertext accepted")
	}
}
