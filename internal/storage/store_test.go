package storage

import (
	"context"
	"fmt"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), filepath.Join(t.TempDir(), "messages.sqlite"), []string{"g1", "g2"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func putTest(t *testing.T, s *Store, id, group, text string, at time.Time) {
	t.Helper()
	if err := s.Put(context.Background(), domain.Message{GroupID: group, ID: id, SenderID: "sender", SentAt: at, Text: text, Source: "live"}); err != nil {
		t.Fatal(err)
	}
}
func TestSearchDeduplicationAllowlistAndUnicode(t *testing.T) {
	// Arrange
	ctx := context.Background()
	s := openTest(t)
	at := time.Now().UTC()
	putTest(t, s, "m1", "g1", "Ремонт кондиционера. Cà phê ngon", at)
	putTest(t, s, "m1", "g1", "duplicate delivery", at)
	putTest(t, s, "m2", "forbidden", "Ремонт", at)
	// Act
	hits, _, _, err := s.Search(ctx, Search{Query: "РЕМОНТ"})
	viet, _, _, ve := s.Search(ctx, Search{Query: "ca phe"})
	// Assert
	if err != nil || len(hits) != 1 || hits[0].ID != "m1" {
		t.Fatalf("search: %+v %v", hits, err)
	}
	if ve != nil || len(viet) != 1 {
		t.Fatalf("diacritics: %+v %v", viet, ve)
	}
	if _, e := s.Message(ctx, "forbidden", "m2"); e == nil {
		t.Fatal("allowlist bypass")
	}
}
func TestPaginationSnapshotAndTampering(t *testing.T) {
	// Arrange
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	for i := 0; i < 5; i++ {
		putTest(t, s, fmt.Sprint(i), "g1", "ремонт", at.Add(time.Duration(i)*time.Minute))
	}
	// Act
	first, c, _, err := s.Search(ctx, Search{Query: "ремонт", Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	putTest(t, s, "new-backdated", "g1", "ремонт", at)
	rest, c2, _, err := s.Search(ctx, Search{Query: "ремонт", Limit: 2, Cursor: *c})
	// Assert
	if err != nil || len(first) != 2 || len(rest) != 2 || rest[0].ID != "2" || c2 == nil {
		t.Fatalf("pagination: %+v %+v %v", first, rest, err)
	}
	last, _, _, err := s.Search(ctx, Search{Query: "ремонт", Limit: 2, Cursor: *c2})
	if err != nil || len(last) != 1 || last[0].ID != "0" {
		t.Fatalf("snapshot leaked: %+v %v", last, err)
	}
	if _, _, _, err = s.Search(ctx, Search{Query: "other", Cursor: *c}); err == nil {
		t.Fatal("changed filters accepted")
	}
	if _, _, _, err = s.Search(ctx, Search{Query: "ремонт", Cursor: *c + "x"}); err == nil {
		t.Fatal("tampered cursor accepted")
	}
}
func TestRetentionAndDeleteUpdateFTS(t *testing.T) {
	// Arrange
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC()
	putTest(t, s, "old", "g1", "test retention", at.AddDate(0, 0, -100))
	putTest(t, s, "new", "g1", "test delete", at)
	// Act
	if err := s.Retain(ctx); err != nil {
		t.Fatal(err)
	}
	h, _, _, err := s.Search(ctx, Search{Query: "test"})
	if err != nil || len(h) != 1 || h[0].ID != "new" {
		t.Fatalf("retention: %+v %v", h, err)
	}
	if err = s.Delete(ctx, "g1", "new"); err != nil {
		t.Fatal(err)
	}
	h, _, _, err = s.Search(ctx, Search{Query: "test"})
	// Assert
	if err != nil || len(h) != 0 {
		t.Fatalf("FTS deletion: %+v %v", h, err)
	}
}
func TestDateBoundariesAndPunctuation(t *testing.T) {
	// Arrange
	s := openTest(t)
	ctx := context.Background()
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	putTest(t, s, "first", "g1", "repair service", at)
	putTest(t, s, "second", "g1", "repair service", at.Add(time.Hour))
	// Act
	h, _, _, err := s.Search(ctx, Search{Query: "repair; service!", Since: at.Format(time.RFC3339), Until: at.Add(time.Hour).Format(time.RFC3339)})
	// Assert
	if err != nil || len(h) != 1 || h[0].ID != "first" {
		t.Fatalf("interval: %+v %v", h, err)
	}
	if _, _, _, err = s.Search(ctx, Search{Query: "!!!"}); err == nil {
		t.Fatal("punctuation-only query accepted")
	}
}

func TestRestartAndAccountIsolation(t *testing.T) {
	// Arrange
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "state.sqlite")
	s, err := Open(ctx, path, []string{"g1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BindAccount(ctx, "account-one"); err != nil {
		t.Fatal(err)
	}
	putTest(t, s, "persisted", "g1", "Ремонт", time.Now().UTC())
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	// Act
	s, err = Open(ctx, path, []string{"g1"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	hits, _, _, err := s.Search(ctx, Search{Query: "Ремонт"})
	other := s.BindAccount(ctx, "account-two")
	// Assert
	if err != nil || len(hits) != 1 || hits[0].ID != "persisted" {
		t.Fatalf("restart: %+v %v", hits, err)
	}
	if other == nil {
		t.Fatal("cross-account state accepted")
	}
}
