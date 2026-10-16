package storage

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestLegacyCallSuppressionDoesNotRetainPayload(t *testing.T) {
	// Arrange: a retained message identity predates the call classifier.
	ctx := t.Context()
	s, err := OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "calls.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	at := time.Now().UTC()
	m := domain.Message{
		GroupID: "g1", ID: "legacy-call", SenderID: "peer", SentAt: at,
		Conversation: domain.ConversationRef{Type: "direct", ID: "g1"},
		Text:         "legacy projection", Source: "live", Direction: "incoming",
	}
	if err = s.Put(ctx, m); err != nil {
		t.Fatal(err)
	}
	a := domain.Activity{
		Family: "call", SchemaVersion: 1, ConversationType: "direct", ConversationID: "g1",
		ActorID: "peer", OccurredAt: at, Direction: "incoming", SourceID: "legacy-source",
		IdentityBasis: "observed_fingerprint", Source: "replay", ProtocolMessageID: m.ID,
		Call: &domain.CallObservation{Status: "unknown", Mode: "unknown"},
	}
	// Act: duplicate replay must keep a silent, content-free tombstone.
	for range 2 {
		if err = s.PutActivity(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	// Assert: no retained activity payload, callback or rollback obstruction.
	var count, expired, deliveries int
	var body string
	if err = s.DB.QueryRowContext(ctx, `SELECT count(*),max(expired),max(payload_json) FROM activity_observations`).
		Scan(&count, &expired, &body); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT count(*) FROM event_deliveries`).Scan(&deliveries); err != nil {
		t.Fatal(err)
	}
	report, err := CheckLegacyRollback(ctx, s.DB)
	if err != nil || !report.Compatible || report.RetainedActivityPayloads != 0 || count != 1 || expired != 1 ||
		body != "" ||
		deliveries != 0 {
		t.Fatal("suppression retained content or changed rollback", report, err, count, expired, deliveries)
	}
}

type rollbackMarkerCase struct {
	name, family, body string
	expired            int
	delivery           bool
	want               string
}

func TestPruneCanonicalizesOnlyLegacyCallSuppressionMarker(t *testing.T) {
	cases := []rollbackMarkerCase{
		{"suppressed", "call", "{}", 1, false, ""},
		{"real expired payload", "call", `{"family":"call"}`, 1, false, `{"family":"call"}`},
		{"reaction", "reaction", "{}", 1, false, "{}"},
		{"active", "call", "{}", 0, false, "{}"},
		{"delivery exists", "call", "{}", 1, true, "{}"},
		{"spaced object", "call", "{ }", 1, false, "{ }"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			testRollbackMarker(t, tc)
		})
	}
}

func testRollbackMarker(t *testing.T, tc rollbackMarkerCase) {
	t.Helper()
	// Arrange: disabled age retention must not disable marker canonicalization.
	s := openTest(t)
	s.retention = 0
	ctx := t.Context()
	at := time.Now().UTC()
	created := at.Format(time.RFC3339Nano)
	seedRollbackMarker(t, s, tc, created)
	// Act: the guard is read-only; pruning is the only canonicalization path.
	before, err := CheckLegacyRollback(ctx, s.DB)
	if err != nil || before.Compatible || before.RetainedActivityPayloads != 1 {
		t.Fatal(before, err)
	}
	var original string
	if err = s.DB.QueryRowContext(ctx, `SELECT payload_json FROM activity_observations`).Scan(&original); err != nil ||
		original != tc.body {
		t.Fatal("guard mutated payload", err)
	}
	for range 2 {
		if err = s.PruneActivities(ctx, at); err != nil {
			t.Fatal(err)
		}
	}
	assertRollbackMarker(t, s, tc, created)
}

func seedRollbackMarker(t *testing.T, s *Store, tc rollbackMarkerCase, created string) {
	t.Helper()
	ctx := t.Context()
	if _, err := s.DB.ExecContext(
		ctx,
		`INSERT INTO activity_observations(seq,event_id,source_key,family,conversation_type,conversation_id,direction,payload_json,created_at,expired) VALUES(1,'event','source',?,'direct','peer','incoming',?,?,?)`,
		tc.family,
		tc.body,
		created,
		tc.expired,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.ExecContext(ctx, `UPDATE activity_fanout SET seq=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	if tc.delivery {
		if _, err := s.DB.ExecContext(
			ctx,
			`INSERT INTO event_deliveries(event_id,subscription_id,generation,payload,state,next_attempt_at,deadline,activity_seq) VALUES('event','absent','generation',X'','delivered','2030-01-01','2030-01-02',1)`,
		); err != nil {
			t.Fatal(err)
		}
	}
}

func assertRollbackMarker(t *testing.T, s *Store, tc rollbackMarkerCase, created string) {
	t.Helper()
	ctx := t.Context()
	var err error
	// Assert: only the exact empty marker changed; identity/order/time survive.
	var seq, watermark int
	var event, key, family, kind, peer, direction, body, gotCreated string
	var expired int
	if err = s.DB.QueryRowContext(ctx, `SELECT seq,event_id,source_key,family,conversation_type,conversation_id,direction,payload_json,created_at,expired FROM activity_observations`).
		Scan(&seq, &event, &key, &family, &kind, &peer, &direction, &body, &gotCreated, &expired); err != nil {
		t.Fatal(err)
	}
	if err = s.DB.QueryRowContext(ctx, `SELECT seq FROM activity_fanout WHERE id=1`).Scan(&watermark); err != nil {
		t.Fatal(err)
	}
	if seq != 1 || watermark != 1 || event != "event" || key != "source" || family != tc.family || kind != "direct" ||
		peer != "peer" ||
		direction != "incoming" ||
		gotCreated != created ||
		expired != tc.expired ||
		body != tc.want {
		t.Fatal("canonicalization modified retained data or identity")
	}
	after, err := CheckLegacyRollback(ctx, s.DB)
	wantCompatible := tc.want == ""
	if err != nil || after.Compatible != wantCompatible {
		t.Fatal(after, err)
	}
}
