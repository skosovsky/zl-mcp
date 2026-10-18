package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestBrowseReductionRetainsInsertionSnapshotAndRechecksPolicy(t *testing.T) {
	// Arrange: retain an invocation-owned page before later inserts or policy changes.
	ctx := t.Context()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref := domain.ConversationRef{Type: domain.ConversationGroup, ID: "g"}
	stamp := time.Now().UTC()
	for i := range 5 {
		if err = s.Put(
			ctx,
			domain.Message{
				GroupID:  "g",
				ID:       fmt.Sprintf("m%d", i),
				SenderID: "sender",
				SentAt:   stamp.Add(time.Duration(i) * time.Second),
				Text:     "short",
				Source:   "live",
			},
		); err != nil {
			t.Fatal(err)
		}
	}
	page, err := s.BrowsePage(ctx, ref, "", "", "asc", 5, "")
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Put(
		ctx,
		domain.Message{GroupID: "g", ID: "late", SenderID: "sender", SentAt: stamp, Text: "late", Source: "live"},
	); err != nil {
		t.Fatal(err)
	}
	// Act: force an actual reduction after a competing insert with an old timestamp.
	if err = page.Shorten(ctx, 2); err != nil {
		t.Fatal(err)
	}
	values := page.Value["messages"].([]map[string]any)
	next, err := s.Browse(ctx, ref, "", "", "asc", 50, page.Value["next_cursor"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// Assert: boundary advances only beyond returned rows within the original snapshot.
	rest := next["messages"].([]map[string]any)
	if len(values) != 2 || values[0]["message_id"] != "m0" || values[1]["message_id"] != "m1" || len(rest) != 3 ||
		rest[0]["message_id"] != "m2" {
		t.Fatalf("lost snapshot %+v %+v", values, rest)
	}
	s.policy = domain.CollectionPolicy{}
	err = page.Shorten(ctx, 1)
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "PERMISSION_DENIED" {
		t.Fatalf("reduction bypassed policy %v", err)
	}
}
