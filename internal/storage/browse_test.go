package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestBrowseBoundariesSnapshotRestartAndNamespace(t *testing.T) {
	// Arrange
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "messages.sqlite")
	s, err := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	if err = s.BindAccount(ctx, "self"); err != nil {
		t.Fatal(err)
	}
	ref := domain.ConversationRef{Type: "direct", ID: "peer"}
	stamp := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	put := func(kind, id, sender string, at time.Time) {
		t.Helper()
		if e := s.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: ref.ID}, ID: id, SenderID: sender, SentAt: at, Text: strings.Repeat("界", 151), Source: "replay"}); e != nil {
			t.Fatal(e)
		}
	}
	put("direct", "a", "peer", stamp)
	put("direct", "b", "self", stamp)
	put("direct", "upper", "peer", stamp.Add(time.Hour))
	put("group", "foreign", "peer", stamp)
	since := "2026-10-03T13:00:00+03:00"
	until := "2026-10-03T14:00:00+03:00"
	// Act
	first, err := s.Browse(ctx, ref, since, until, "asc", 1, "")
	if err != nil {
		t.Fatal(err)
	}
	put("direct", "late-old", "peer", stamp.Add(time.Minute))
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.Browse(ctx, ref, since, until, "asc", 50, first["next_cursor"].(string))
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	a := first["messages"].([]map[string]any)[0]
	b := second["messages"].([]map[string]any)
	if a["message_id"] != "a" || a["direction"] != "incoming" || len(b) != 1 || b[0]["message_id"] != "b" || b[0]["direction"] != "outgoing" || second["has_more"] != false {
		t.Fatalf("wrong boundaries, namespace, order or snapshot: %#v %#v", first, second)
	}
	if first["snapshot_at"] != second["snapshot_at"] || a["text_truncated"] != true || len([]rune(a["excerpt"].(string))) != 150 || a["text_resource_uri"] != ConversationMessageURI(ref, "a") {
		t.Fatal("snapshot or Unicode resource contract lost")
	}
	if second["coverage"].(map[string]any)["history_complete"] != false {
		t.Fatal("browse claimed full history")
	}
	// Act / Assert: descending tie order and exclusive upper bound remain exact.
	desc, err := s.Browse(ctx, ref, since, until, "desc", 50, "")
	if err != nil {
		t.Fatal(err)
	}
	v := desc["messages"].([]map[string]any)
	if len(v) != 3 || v[0]["message_id"] != "late-old" || v[1]["message_id"] != "b" || v[2]["message_id"] != "a" {
		t.Fatal("descending order or exclusive bound wrong")
	}
	for _, changed := range []struct {
		ref   domain.ConversationRef
		order string
	}{{ref, "desc"}, {domain.ConversationRef{Type: "group", ID: ref.ID}, "asc"}} {
		if _, err = s.Browse(ctx, changed.ref, since, until, changed.order, 1, first["next_cursor"].(string)); err == nil {
			t.Fatal("cursor crossed filter namespace/order")
		}
	}
}

func TestBrowseEmptyReasonsPolicyAndInvalidDates(t *testing.T) {
	// Arrange
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref := domain.ConversationRef{Type: "direct", ID: "peer"}
	stamp := time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC)
	// Act / Assert
	unknown, err := s.Browse(ctx, ref, "", "", "", 20, "")
	if err != nil || unknown["empty_reason"] != "unknown_conversation" {
		t.Fatal(err, unknown)
	}
	if err = s.Put(ctx, domain.Message{Conversation: ref, ID: "a", SenderID: "peer", SentAt: stamp, Source: "live"}); err != nil {
		t.Fatal(err)
	}
	period, err := s.Browse(ctx, ref, stamp.Add(time.Hour).Format(time.RFC3339), "", "", 20, "")
	if err != nil || period["empty_reason"] != "no_records_in_period" {
		t.Fatal(err, period)
	}
	if err = s.DeleteConversation(ctx, ref, "a"); err != nil {
		t.Fatal(err)
	}
	empty, err := s.Browse(ctx, ref, "", "", "", 20, "")
	if err != nil || empty["empty_reason"] != "no_collected_data" {
		t.Fatal(err, empty)
	}
	for _, bounds := range [][2]string{{"01/02/2026", ""}, {"2026-10-03T10:00:00Z", "2026-10-03T10:00:00Z"}} {
		if _, err = s.Browse(ctx, ref, bounds[0], bounds[1], "", 20, ""); err == nil {
			t.Fatal("ambiguous or inverted date accepted")
		}
	}
	s.policy = domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{{Type: "group", ID: ref.ID}: true}}
	if _, err = s.Browse(ctx, ref, "", "", "", 20, ""); err == nil {
		t.Fatal("policy bypass")
	}
}
