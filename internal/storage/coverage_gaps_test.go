package storage

import (
	"errors"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

//nolint:cyclop,gocognit,gocyclo // Keep the complete Arrange–Act–Assert continuation and boundary scenario together.
func TestCoverageGapsSnapshotAndCurrentPolicy(t *testing.T) {
	// Arrange: many equal-timestamp gaps must retain their distinct identities.
	ctx := t.Context()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref := domain.ConversationRef{Type: domain.ConversationGroup, ID: "g"}
	for i := range 801 {
		if _, err = s.DB.ExecContext(
			ctx,
			"INSERT INTO collection_gaps(conversation_type,group_id,started_at,ended_at,reason) VALUES(?,?,?,?,?)",
			ref.Type,
			ref.ID,
			"2026-10-01T00:00:00Z",
			"2026-10-01T00:01:00Z",
			fmt.Sprintf("reason-%d", i),
		); err != nil {
			t.Fatal(err)
		}
	}
	// Act: explicit compact coverage gives exact count and a fresh continuation.
	full, err := s.Coverage(ctx, "g")
	if err != nil {
		t.Fatal(err)
	}
	compact, err := s.Coverage(domain.WithCompactCoverage(ctx), "g")
	if err != nil {
		t.Fatal(err)
	}
	// Assert: full compatibility, bounded representation and append-stable snapshot.
	if len(full.KnownGaps) != 801 || full.CoverageDetails != nil || len(compact.KnownGaps) != 0 ||
		compact.CoverageDetails.TotalKnownGaps != 801 ||
		!compact.CoverageDetails.GapsTruncated {
		t.Fatalf("unexpected coverage %v %v", full.CoverageDetails, compact.CoverageDetails)
	}
	cursor := compact.CoverageDetails.GapsCursor
	seen := map[string]bool{}
	for page := 0; cursor != nil; page++ {
		got, e := s.CoverageGaps(ctx, ref, 37, *cursor)
		if e != nil {
			t.Fatal(e)
		}
		if got.TotalKnownGaps != 801 || got.SnapshotAt != compact.CoverageDetails.SnapshotAt || len(got.Gaps) > 37 {
			t.Fatalf("invalid page %+v", got)
		}
		for _, gap := range got.Gaps {
			if seen[gap.Reason] {
				t.Fatal("duplicate", gap.Reason)
			}
			seen[gap.Reason] = true
		}
		cursor = got.NextCursor
		if page == 0 {
			if _, e = s.DB.ExecContext(
				ctx,
				"INSERT INTO collection_gaps(conversation_type,group_id,started_at,reason) VALUES(?,?,?,?)",
				ref.Type,
				ref.ID,
				"2026-10-02T00:00:00Z",
				"later",
			); e != nil {
				t.Fatal(e)
			}
		}
	}
	if len(seen) != 801 || seen["later"] {
		t.Fatal("snapshot lost rows or included later append", len(seen))
	}
	// Act/Assert: same signed cursor cannot be transplanted to another identity.
	_, err = s.CoverageGaps(
		ctx,
		domain.ConversationRef{Type: domain.ConversationGroup, ID: "other"},
		10,
		*compact.CoverageDetails.GapsCursor,
	)
	if err == nil {
		t.Fatal("outside-policy target accepted")
	}
	_, err = s.CoverageGaps(ctx, ref, 10, *compact.CoverageDetails.GapsCursor+"x")
	if err == nil {
		t.Fatal("tampered cursor accepted")
	}
	// Act/Assert: even another allowed typed identity cannot reuse this snapshot.
	s.policy = domain.CollectionPolicy{All: true}
	_, err = s.CoverageGaps(
		ctx,
		domain.ConversationRef{Type: domain.ConversationDirect, ID: "g"},
		10,
		*compact.CoverageDetails.GapsCursor,
	)
	var mismatch *domain.Error
	if !errors.As(err, &mismatch) || mismatch.Code != "INVALID_ARGUMENT" {
		t.Fatalf("wrong typed target accepted %v", err)
	}
	// Act/Assert: changing an old gap explicitly expires the immutable snapshot.
	if _, err = s.DB.ExecContext(ctx, "UPDATE collection_gaps SET reason='changed' WHERE id=1"); err != nil {
		t.Fatal(err)
	}
	_, err = s.CoverageGaps(ctx, ref, 10, *compact.CoverageDetails.GapsCursor)
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "CURSOR_EXPIRED" {
		t.Fatalf("changed snapshot error %v", err)
	}
	// Act/Assert: policy is rechecked even for a valid existing cursor.
	s.policy = domain.CollectionPolicy{}
	_, err = s.CoverageGaps(ctx, ref, 10, *compact.CoverageDetails.GapsCursor)
	if !errors.As(err, &de) || de.Code != "PERMISSION_DENIED" {
		t.Fatalf("revoked policy error %v", err)
	}
}

func TestCoverageCursorExpiryAndEmpty(t *testing.T) {
	// Arrange
	ctx := t.Context()
	s, err := Open(ctx, filepath.Join(t.TempDir(), "state.sqlite"), []string{"g"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ref := domain.ConversationRef{Type: domain.ConversationGroup, ID: "g"}
	// Act
	c, err := s.compactCoverage(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	// Assert
	if c.TotalKnownGaps != 0 || c.GapsTruncated || c.GapsCursor != nil {
		t.Fatalf("empty compact %+v", c)
	}
	old, err := s.newCoverageCursor(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	old.Observed = time.Now().Add(-25 * time.Hour).Format(time.RFC3339Nano)
	token, err := s.EncodeCursor(old)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.CoverageGaps(ctx, ref, 5, token)
	var de *domain.Error
	if !errors.As(err, &de) || de.Code != "CURSOR_EXPIRED" {
		t.Fatalf("expired error %v", err)
	}
}
