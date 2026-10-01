package storage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestCrashGapStartsAtLastHeartbeatAndPreservesOpenGap(t *testing.T) {
	// Arrange: last successful heartbeat predates an unclean process stop.
	s := openTest(t)
	ctx := context.Background()
	state := map[string]any{"authenticated": true, "collector_state": "connected", "last_connected_at": nil, "last_event_at": nil, "last_persisted_at": nil, "last_error": nil}
	if err := s.SetState(ctx, state); err != nil {
		t.Fatal(err)
	}
	previous := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	if _, err := s.DB.ExecContext(ctx, "UPDATE collector_state SET heartbeat=?", previous.Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	// Act
	observed, err := s.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.BeginGap(ctx, "collector_start_or_restart"); err != nil {
		t.Fatal(err)
	}
	if err = s.BeginGap(ctx, "listener_disconnected"); err != nil {
		t.Fatal(err)
	}
	coverage, err := s.Coverage(ctx, "g1")
	// Assert: repeated failures retain the earliest open gap, covering the crash interval.
	if err != nil || observed["collector_state"] != "stopped" || len(coverage.KnownGaps) != 1 || !coverage.KnownGaps[0].From.Equal(previous) || coverage.KnownGaps[0].To != nil {
		t.Fatalf("state=%+v coverage=%+v error=%v", observed, coverage, err)
	}
	// Act/Assert: a connection closes gaps in every enabled group; absent corpus remains explicit.
	if err = s.EndGaps(ctx); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"g1", "g2"} {
		v, err := s.Coverage(ctx, id)
		if err != nil || len(v.KnownGaps) != 1 || v.KnownGaps[0].To == nil || v.CollectionStartedAt != nil || v.EarliestStoredAt != nil || v.HistoryComplete {
			t.Fatalf("%s coverage=%+v error=%v", id, v, err)
		}
	}
}

func TestContextTimestampTiesReplyAndSenderFilter(t *testing.T) {
	// Arrange: same timestamps must use opaque message IDs as a stable tie-breaker.
	s := openTest(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Millisecond)
	for _, id := range []string{"a", "b", "c", "d"} {
		sender := "first"
		if id == "c" {
			sender = "second"
		}
		m := domain.Message{GroupID: "g1", ID: id, SenderID: sender, SentAt: at, Text: "ремонт", Source: "live"}
		if id == "b" {
			reply := "d"
			m.ReplyTo = &reply
		}
		if err := s.Put(ctx, m); err != nil {
			t.Fatal(err)
		}
	}
	// Act
	result, err := s.Context(ctx, "g1", "b", 1, 1)
	hits, _, _, searchErr := s.Search(ctx, Search{Query: "ремонт", SenderID: "second", GroupID: "g1"})
	// Assert
	if err != nil || searchErr != nil {
		t.Fatalf("context=%v search=%v", err, searchErr)
	}
	before := result["before"].([]domain.Message)
	after := result["after"].([]domain.Message)
	reply := result["reply_to"].(*domain.Message)
	if len(before) != 1 || before[0].ID != "a" || len(after) != 1 || after[0].ID != "c" || reply == nil || reply.ID != "d" || len(hits) != 1 || hits[0].ID != "c" {
		t.Fatalf("context=%+v hits=%+v", result, hits)
	}
	// Act: quoted message deleted by upstream; absence must be represented as null.
	if err = s.Delete(ctx, "g1", "d"); err != nil {
		t.Fatal(err)
	}
	result, err = s.Context(ctx, "g1", "b", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var wire map[string]any
	if err = json.Unmarshal(encoded, &wire); err != nil {
		t.Fatal(err)
	}
	if wire["reply_to"] != nil {
		t.Fatalf("missing quote invented: %+v", wire)
	}
}

func TestCatalogUnicodeAndRemovedMembership(t *testing.T) {
	// Arrange: decomposed Vietnamese name and duplicate display names with distinct IDs.
	s := openTest(t)
	ctx := context.Background()
	if err := s.ReplaceCatalog(ctx, []domain.Group{{ID: "g1", Name: "Ca\u0300 phe\u0302"}, {ID: "g2", Name: "Команда"}, {ID: "g3", Name: "Команда"}}); err != nil {
		t.Fatal(err)
	}
	// Act
	viet, err := s.Groups(ctx, "CÀ PHÊ", 20, "")
	duplicate, duplicateErr := s.Groups(ctx, "команда", 20, "")
	// Assert
	if err != nil || duplicateErr != nil {
		t.Fatalf("catalog=%v duplicates=%v", err, duplicateErr)
	}
	if len(viet["groups"].([]domain.Group)) != 1 || len(duplicate["groups"].([]domain.Group)) != 2 {
		t.Fatalf("viet=%+v duplicate=%+v", viet, duplicate)
	}
	// Act/Assert: refreshing catalog removes stale memberships but retains cached details.
	if err = s.ReplaceCatalog(ctx, []domain.Group{{ID: "g2", Name: "Команда"}}); err != nil {
		t.Fatal(err)
	}
	groups, err := s.Groups(ctx, "", 20, "")
	old, oldErr := s.Group(ctx, "g1")
	if err != nil || oldErr != nil || len(groups["groups"].([]domain.Group)) != 1 || old["membership"] != "unknown" {
		t.Fatalf("groups=%+v old=%+v errors=%v %v", groups, old, err, oldErr)
	}
}

func TestExpiredHeartbeatPreservesAuthenticationRequired(t *testing.T) {
	// Arrange: the collector has stopped after a verified authentication rejection.
	s := openTest(t)
	ctx := context.Background()
	if err := s.SetState(ctx, map[string]any{"authenticated": false, "collector_state": "auth_required", "last_error": map[string]any{"code": "NOT_AUTHENTICATED", "message": "Run local login."}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, "UPDATE collector_state SET heartbeat=?", time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)); err != nil {
		t.Fatal(err)
	}
	// Act
	state, err := s.State(ctx)
	// Assert: time passing must not hide the required recovery action.
	if err != nil || state["collector_state"] != "auth_required" || state["authenticated"] != false {
		t.Fatalf("state=%+v error=%v", state, err)
	}
}
