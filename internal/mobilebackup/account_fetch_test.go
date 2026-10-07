package mobilebackup

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
)

func TestAccountArchiveRetainsEveryMappedFileAndOwnsBuffers(t *testing.T) {
	// Arrange: typed IDs collide across a direct chat and a group.
	decoded := Format1Archive{Archive: selectionArchive(), CiphertextBytes: 123, ContainerBytes: 100, TrailingBytes: 23}
	pairs := []IdentityPair{{Plain: "9007199254740993", Session: "12"}, {Plain: "9007199254740993", Session: "12", Group: true}, {Plain: "18446744073709551615", Session: "13"}}
	buffer := decoded.Archive.Files[2].Data
	// Act.
	got, err := ownAccountArchive(context.Background(), &decoded, pairs)
	clear(pairs)
	// Assert: no selection discarded a file; mapping is independently owned.
	if err != nil || len(got.archive.Files) != 3 || got.directFiles() != 2 || got.groupFiles() != 1 || decoded.Archive.Files != nil || got.pairs[2].Session != "13" || got.trailingBytes != 23 || string(got.archive.Files[2].Data) != "synthetic-c" {
		t.Fatal("account archive lost files or mapping", err)
	}
	b, _ := json.Marshal(got)
	if string(b) != "{}" || fmt.Sprintf("%#v", got) != "mobile account archive [redacted]" {
		t.Fatal("account archive exposed private evidence")
	}
	got.Clear()
	if got.archive.Files != nil || got.pairs != nil || !bytes.Equal(buffer, make([]byte, len(buffer))) {
		t.Fatal("account archive retained owned data")
	}
}

func TestAccountArchiveRejectsIncompleteMappingWithoutOwnershipTransfer(t *testing.T) {
	for _, mode := range []string{"missing", "duplicate", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: a valid first file must not hide an invalid later mapping.
			decoded := Format1Archive{Archive: selectionArchive()}
			defer decoded.Clear()
			pairs := []IdentityPair{{Plain: "9007199254740993", Session: "12"}, {Plain: "9007199254740993", Session: "12", Group: true}, {Plain: "18446744073709551615", Session: "13"}}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "missing" {
				pairs = pairs[:2]
			}
			if mode == "duplicate" {
				pairs[2] = pairs[0]
			}
			if mode == "cancelled" {
				cancel()
			}
			// Act.
			got, err := ownAccountArchive(ctx, &decoded, pairs)
			// Assert: no prefix escapes; the caller still owns cleanup on failure.
			if err == nil || got.archive.Files != nil || got.pairs != nil || len(decoded.Archive.Files) != 3 {
				t.Fatal("partial account archive accepted")
			}
		})
	}
}
