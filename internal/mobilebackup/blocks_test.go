package mobilebackup

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
)

func goldenBlocks(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/format1-blocks.hex")
	if err != nil {
		t.Fatal(err)
	}
	b, err := hex.DecodeString(strings.Join(strings.Fields(string(raw)), ""))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestFormat1IndependentOpenSSLChunks(t *testing.T) {
	// Arrange: OpenSSL-encrypted synthetic data crosses the native chunk boundary.
	input := goldenBlocks(t)
	want := make([]byte, len(input))
	for i := range want {
		want[i] = byte(i*37 + 11)
	}
	copy(want, goldenHeader(t))
	// Act
	got, err := DecryptFormat1(context.Background(), bytes.NewReader(input), strings.Repeat("0123456789abcdef", 4), int64(len(input)))
	// Assert: exact bytes, including the first block after IV reset at 65536.
	if err != nil || !bytes.Equal(got, want) {
		t.Fatal("independent cipher vector mismatch", err)
	}
	// This independent block vector contains a table prefix and arbitrary bytes;
	// it is deliberately not asserted to be a complete mobile archive.

}

func TestFormat1RejectsBadKeyAlignmentBudgetAndCancellation(t *testing.T) {
	// Arrange
	input := goldenBlocks(t)
	validKey := strings.Repeat("0123456789abcdef", 4)
	for _, key := range []string{"", strings.Repeat("a", 31), strings.Repeat("a", 33), strings.Repeat("a", 258), strings.Repeat("g", 64), "private-key-marker", strings.Repeat("f", 64)} {
		// Act / Assert: invalid or wrong keys retain no plaintext or input error text.
		got, err := DecryptFormat1(context.Background(), bytes.NewReader(input), key, int64(len(input)))
		if !errors.Is(err, ErrBlocks) || got != nil || strings.Contains(err.Error(), "private-key-marker") {
			t.Fatal("invalid key accepted or leaked")
		}
	}
	for _, n := range []int{0, 1, 15, 17, len(input) - 1} {
		got, err := DecryptFormat1(context.Background(), bytes.NewReader(input[:n]), validKey, int64(len(input)))
		if !errors.Is(err, ErrBlocks) || got != nil {
			t.Fatal("unaligned stream accepted")
		}
	}
	for _, limit := range []int64{-1, 0, 16, int64(len(input) - 1), int64(MaxTotalBytes + 1)} {
		r := bytes.NewReader(input)
		got, err := DecryptFormat1(context.Background(), r, validKey, limit)
		if !errors.Is(err, ErrBlocks) || got != nil {
			t.Fatal("budget bypassed")
		}
		if limit == 16 && len(input)-r.Len() > 17 {
			t.Fatal("read past overflow sentinel")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := bytes.NewReader(input)
	got, err := DecryptFormat1(ctx, r, validKey, int64(len(input)))
	if !errors.Is(err, ErrBlocks) || got != nil || r.Len() != len(input) {
		t.Fatal("cancelled transform consumed bytes")
	}
}
