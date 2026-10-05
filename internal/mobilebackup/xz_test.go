package mobilebackup

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"hash/crc32"
	"os"
	"testing"
)

type xzVector struct{ Name, Plaintext, Compressed string }

func xzVectors(t *testing.T) []xzVector {
	t.Helper()
	b, err := os.ReadFile("testdata/xz-vectors.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors []xzVector
	if err := json.Unmarshal(b, &vectors); err != nil {
		t.Fatal(err)
	}
	return vectors
}
func xzHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestDecompressXZIndependentVectors(t *testing.T) {
	for _, v := range xzVectors(t) {
		t.Run(v.Name, func(t *testing.T) {
			// Arrange: Python/liblzma fixtures, not encoded by the Go decoder.
			compressed, plain := xzHex(t, v.Compressed), xzHex(t, v.Plaintext)
			// Act.
			out, err := DecompressXZ(context.Background(), bytes.NewReader(compressed), int64(len(compressed)), int64(len(plain)+1))
			// Assert.
			if err != nil || !bytes.Equal(out, plain) {
				t.Fatalf("decode: %v, length %d", err, len(out))
			}
		})
	}
}

func TestDecompressXZConcatenation(t *testing.T) {
	// Arrange: independent streams separated and followed by valid padding.
	v := xzVectors(t)
	a, b := xzHex(t, v[1].Compressed), xzHex(t, v[2].Compressed)
	compressed := append(append(append(append([]byte{}, a...), make([]byte, 4)...), b...), make([]byte, 8)...)
	expected := append(xzHex(t, v[1].Plaintext), xzHex(t, v[2].Plaintext)...)
	// Act.
	out, err := DecompressXZ(context.Background(), bytes.NewReader(compressed), int64(len(compressed)), int64(len(expected)))
	// Assert.
	if err != nil || !bytes.Equal(out, expected) {
		t.Fatalf("concat: %v", err)
	}
}

func TestXZAllocationDeclarationsRejectedBeforeDecode(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func([]byte)
	}{
		{"large dictionary", func(b []byte) { b[16] = 40 }},
		{"just above dictionary budget", func(b []byte) { b[16] = 29 }},
		{"invalid dictionary", func(b []byte) { b[16] = 41 }},
		{"unsupported filter", func(b []byte) { b[14] = 0x20 }},
		{"multiple filters", func(b []byte) { b[13] = 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: alter a valid block and recompute its CRC, so CRC alone cannot reject it.
			b := xzHex(t, xzVectors(t)[1].Compressed)
			tc.mutate(b)
			headerLen := (int(b[12]) + 1) * 4
			binary.LittleEndian.PutUint32(b[12+headerLen-4:], crc32.ChecksumIEEE(b[12:12+headerLen-4]))
			// Act: preflight only; dictionary allocation must never run.
			_, err := inspectXZ(context.Background(), b, 1<<20)
			// Assert.
			if err != ErrXZ {
				t.Fatalf("unexpected preflight result: %v", err)
			}
		})
	}
}

func TestDecompressXZCorruptionAndTruncation(t *testing.T) {
	// Arrange: valid text stream and one-byte corruption at every position.
	v := xzVectors(t)[1]
	base := xzHex(t, v.Compressed)
	expected := xzHex(t, v.Plaintext)
	for i := range base {
		b := bytes.Clone(base)
		b[i] ^= 0x80
		// Act.
		out, err := DecompressXZ(context.Background(), bytes.NewReader(b), int64(len(b)), 1<<20)
		// Assert: reject integrity failure or accept only identical decoded content.
		// A changed compressed representation need not change its decoded bytes.
		if err == nil {
			if !bytes.Equal(out, expected) {
				t.Fatalf("changed content at %d escaped", i)
			}
		} else if err != ErrXZ || out != nil {
			t.Fatalf("partial output at %d escaped", i)
		}
	}
	for i := 0; i < len(base); i++ {
		out, err := DecompressXZ(context.Background(), bytes.NewReader(base[:i]), int64(len(base)), 1<<20)
		if err != ErrXZ || out != nil {
			t.Fatalf("truncation at %d escaped", i)
		}
	}
}

func TestDecompressXZLimitsAndCancellation(t *testing.T) {
	// Arrange.
	v := xzVectors(t)[1]
	b := xzHex(t, v.Compressed)
	plain := xzHex(t, v.Plaintext)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, tc := range []struct {
		name    string
		ctx     context.Context
		in, out int64
	}{
		{"input", context.Background(), int64(len(b) - 1), 1 << 20},
		{"output", context.Background(), int64(len(b)), int64(len(plain) - 1)},
		{"cancel", cancelled, int64(len(b)), 1 << 20},
		{"nil context", nil, int64(len(b)), 1 << 20},
		{"zero", context.Background(), 0, 1 << 20},
		{"policy", context.Background(), int64(MaxTotalBytes) + 1, 1 << 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Act.
			out, err := DecompressXZ(tc.ctx, bytes.NewReader(b), tc.in, tc.out)
			// Assert.
			if err != ErrXZ || out != nil {
				t.Fatalf("budget escaped: %v", err)
			}
		})
	}
}

func TestXZStreamAndBlockCountLimits(t *testing.T) {
	// Arrange: many independently valid streams; no decoder allocations are needed.
	b := xzHex(t, xzVectors(t)[1].Compressed)
	many := bytes.Repeat(b, maxXZStreams+1)
	// Act.
	_, err := inspectXZ(context.Background(), many, MaxTotalBytes)
	// Assert.
	if err != ErrXZ {
		t.Fatal("stream count accepted")
	}
	// Arrange: a checksummed index declaring 1001 records, with no block body.
	// Count rejection must precede record parsing or dictionary allocation.
	index := []byte{0, 0xe9, 0x07, 0}
	index = binary.LittleEndian.AppendUint32(index, crc32.ChecksumIEEE(index))
	footer := make([]byte, 12)
	binary.LittleEndian.PutUint32(footer[4:8], uint32(len(index)/4-1))
	footer[9] = b[7]
	copy(footer[10:], "YZ")
	binary.LittleEndian.PutUint32(footer[:4], crc32.ChecksumIEEE(footer[4:10]))
	bad := append(append(bytes.Clone(b[:12]), index...), footer...)
	// Act.
	_, err = inspectXZ(context.Background(), bad, MaxTotalBytes)
	// Assert.
	if err != ErrXZ {
		t.Fatal("block count accepted")
	}
}

func TestXZCanonicalIntegers(t *testing.T) {
	for _, b := range [][]byte{{0x80, 0}, {0x81, 0}, bytes.Repeat([]byte{0x80}, 9)} {
		// Arrange.
		input := bytes.Clone(b)
		// Act.
		_, ok := xzVLI(&input)
		// Assert.
		if ok {
			t.Fatal("noncanonical integer accepted")
		}
	}
}

func FuzzInspectXZ(f *testing.F) {
	b, err := os.ReadFile("testdata/xz-vectors.json")
	if err != nil {
		f.Fatal(err)
	}
	var vectors []xzVector
	if err := json.Unmarshal(b, &vectors); err != nil {
		f.Fatal(err)
	}
	for _, v := range vectors {
		b, err := hex.DecodeString(v.Compressed)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(b)
	}
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		// Arrange: arbitrary compressed declarations, bounded independently of decoder.
		if len(b) > 1<<20 {
			t.Skip()
		}
		// Act.
		declared, err := inspectXZ(context.Background(), b, 1<<20)
		// Assert: no panic, nonclosed error or budget escape.
		if err != nil && err != ErrXZ {
			t.Fatal("nonclosed error")
		}
		if err == nil && declared > 1<<20 {
			t.Fatal("output budget escaped")
		}
	})
}
