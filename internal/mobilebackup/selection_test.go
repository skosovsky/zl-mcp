package mobilebackup

import (
	"context"
	"errors"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func selectionArchive() PlainArchive {
	return PlainArchive{Files: []ArchiveFile{{Name: "9007199254740993.db", Data: []byte("synthetic-a")}, {Name: "group_9007199254740993.db", Data: []byte("synthetic-b")}, {Name: "18446744073709551615.db", Data: []byte("synthetic-c")}}}
}
func TestArchiveSelectionExactTypedIdentity(t *testing.T) {
	// Arrange: direct/group IDs deliberately collide while retaining distinct namespaces.
	a := selectionArchive()
	pairs := []IdentityPair{{Plain: "18446744073709551615", Session: "13"}, {Plain: "9007199254740993", Session: "12", Group: true}, {Plain: "9007199254740993", Session: "12"}}
	// Act.
	request, e := ArchiveIdentityRequest(context.Background(), a)
	direct, de := SelectArchiveIndex(context.Background(), a, pairs, domain.ConversationRef{Type: "direct", ID: "12"})
	group, ge := SelectArchiveIndex(context.Background(), a, pairs, domain.ConversationRef{Type: "group", ID: "12"})
	// Assert: complete exact IDs/order; selection borrows index without mutating data.
	if e != nil || len(request.Direct) != 2 || request.Direct[0] != "9007199254740993" || request.Direct[1] != "18446744073709551615" || len(request.Groups) != 1 || de != nil || ge != nil || direct != 0 || group != 1 || string(a.Files[0].Data) != "synthetic-a" {
		t.Fatal("typed archive selection lost", e, de, ge)
	}
	_, absent := SelectArchiveIndex(context.Background(), a, pairs, domain.ConversationRef{Type: "direct", ID: "9007199254740993"})
	if !errors.Is(absent, ErrSelectedConversationUnavailable) {
		t.Fatal("source-ID fallback invented")
	}
}
func TestArchiveSelectionRejectsPartialExtraAndAmbiguousMapping(t *testing.T) {
	a := selectionArchive()
	valid := []IdentityPair{{Plain: "9007199254740993", Session: "12"}, {Plain: "9007199254740993", Session: "12", Group: true}, {Plain: "18446744073709551615", Session: "13"}}
	for _, bad := range [][]IdentityPair{valid[:2], append(append([]IdentityPair(nil), valid...), IdentityPair{Plain: "1", Session: "1"}), {valid[0], valid[1], valid[0]}, {valid[0], valid[1], {Plain: "18446744073709551615", Session: "12"}}, {valid[0], valid[1], {Plain: "18446744073709551616", Session: "13"}}, {valid[0], valid[1], {Plain: "18446744073709551615", Session: "g13"}}} {
		// Act / Assert: even a selected match cannot conceal invalid whole mapping.
		index, e := SelectArchiveIndex(context.Background(), a, bad, domain.ConversationRef{Type: "direct", ID: "12"})
		if index != -1 || !errors.Is(e, ErrSelection) {
			t.Fatal("partial mapping accepted")
		}
	}
}
func TestArchiveSelectionRejectsInvalidDeclarationsAndCancellation(t *testing.T) {
	for _, name := range []string{"0.db", "01.db", "18446744073709551616.db", "../1.db", "group_g1.db"} {
		// Arrange / Act / Assert: no invalid source identity enters a request.
		got, e := ArchiveIdentityRequest(context.Background(), PlainArchive{Files: []ArchiveFile{{Name: name, Data: []byte("x")}}})
		if e == nil || got.Direct != nil || got.Groups != nil {
			t.Fatal("invalid declaration accepted")
		}
	}
	for _, a := range []PlainArchive{{}, {Files: []ArchiveFile{{Name: "1.db"}}}, {Files: []ArchiveFile{{Name: "1.db", Data: []byte("x")}, {Name: "1.db", Data: []byte("y")}}}} {
		if _, e := ArchiveIdentityRequest(context.Background(), a); e == nil {
			t.Fatal("missing/duplicate file accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e := ArchiveIdentityRequest(ctx, selectionArchive()); e == nil {
		t.Fatal("cancelled selection accepted")
	}
}
