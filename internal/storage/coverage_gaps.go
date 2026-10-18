package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

const (
	coverageCursorLifetime = 24 * time.Hour
	defaultCoverageLimit   = 50
	maxCoverageLimit       = 100
)

type coverageCursor struct {
	Kind     string                 `json:"kind"`
	Ref      domain.ConversationRef `json:"ref"`
	High     int64                  `json:"high"`
	Last     int64                  `json:"last"`
	Count    int                    `json:"count"`
	Digest   string                 `json:"digest"`
	Observed string                 `json:"observed"`
}

// gapSnapshot hashes stable ordered rows, without retaining unbounded gap arrays.
// Appends after High are excluded; updates/deletion invalidate continuation.
func (s *Store) gapSnapshot(ctx context.Context, ref domain.ConversationRef, high int64) (int, string, error) {
	rows, err := s.DB.QueryContext(
		ctx,
		"SELECT id,started_at,ended_at,reason FROM collection_gaps WHERE conversation_type=? AND group_id=? AND id<=? ORDER BY id",
		ref.Type,
		ref.ID,
		high,
	)
	if err != nil {
		return 0, "", err
	}
	defer rows.Close()
	h := sha256.New()
	count := 0
	for rows.Next() {
		var id int64
		var from, reason string
		var to sql.NullString
		if err = rows.Scan(&id, &from, &to, &reason); err != nil {
			return 0, "", err
		}
		b, encodeErr := json.Marshal([]any{id, from, to, reason})
		if encodeErr != nil {
			return 0, "", encodeErr
		}
		_, _ = h.Write(b)
		count++
	}
	return count, hex.EncodeToString(h.Sum(nil)), rows.Err()
}
func (s *Store) newCoverageCursor(ctx context.Context, ref domain.ConversationRef) (coverageCursor, error) {
	c := coverageCursor{Kind: "coverage-gaps-v1", Ref: ref, Observed: now(), High: 0, Last: 0, Count: 0, Digest: ""}
	if err := s.DB.QueryRowContext(ctx, "SELECT coalesce(max(id),0) FROM collection_gaps WHERE conversation_type=? AND group_id=?", ref.Type, ref.ID).
		Scan(&c.High); err != nil {
		return c, err
	}
	var err error
	c.Count, c.Digest, err = s.gapSnapshot(ctx, ref, c.High)
	return c, err
}
func (s *Store) compactCoverage(ctx context.Context, ref domain.ConversationRef) (*domain.CoverageDetails, error) {
	c, err := s.newCoverageCursor(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := &domain.CoverageDetails{
		Mode:           "compact",
		TotalKnownGaps: c.Count,
		GapsTruncated:  c.Count > 0,
		SnapshotAt:     c.Observed,
		GapsCursor:     nil,
	}
	if c.Count > 0 {
		token, encodeErr := s.EncodeCursor(c)
		if encodeErr != nil {
			return nil, encodeErr
		}
		out.GapsCursor = &token
	}
	return out, nil
}
func coverageExpired() error {
	return &domain.Error{
		Code:      "CURSOR_EXPIRED",
		Retryable: false,
		Message:   "Coverage snapshot changed or expired.",
		NextAction: domain.NextAction{
			Tool:        nil,
			Arguments:   nil,
			Instruction: "Request compact coverage or start zalo_list_coverage_gaps without a cursor to observe current coverage.",
		},
		Details: map[string]any{},
	}
}

// CoverageGaps checks current policy on every page and provides a verifiable
// append-stable snapshot. Changed/deleted rows expire the cursor explicitly.
func (s *Store) CoverageGaps(
	ctx context.Context,
	ref domain.ConversationRef,
	limit int,
	token string,
) (domain.CoverageGaps, error) {
	out := domain.CoverageGaps{
		ConversationType: ref.Type,
		ConversationID:   ref.ID,
		Gaps:             []domain.Gap{},
		TotalKnownGaps:   0,
		HasMore:          false,
		NextCursor:       nil,
		SnapshotAt:       "",
	}
	if !ref.Valid() {
		return out, domain.Invalid("Typed conversation required.")
	}
	if !s.AllowsConversation(ref) {
		return out, subscriptionPermission("Conversation is outside collection policy.")
	}
	if limit == 0 {
		limit = defaultCoverageLimit
	}
	if limit < 1 || limit > maxCoverageLimit {
		return out, domain.Invalid("Use gap limit 1–100.")
	}
	c, err := s.readCoverageCursor(ctx, ref, token)
	if err != nil {
		return out, err
	}
	out.Gaps, out.HasMore, c.Last, err = s.readCoverageGapPage(ctx, c, limit)
	if err != nil {
		return out, err
	}
	// Check again after collecting rows to reject any mixed snapshot.
	if err = s.verifyCoverageSnapshot(ctx, c); err != nil {
		return out, err
	}
	if out.HasMore {
		next, encodeErr := s.EncodeCursor(c)
		if encodeErr != nil {
			return out, encodeErr
		}
		out.NextCursor = &next
	}
	out.TotalKnownGaps = c.Count
	out.SnapshotAt = c.Observed
	return out, nil
}

func (s *Store) readCoverageCursor(
	ctx context.Context,
	ref domain.ConversationRef,
	token string,
) (coverageCursor, error) {
	if token == "" {
		return s.newCoverageCursor(ctx, ref)
	}
	var c coverageCursor
	if err := s.DecodeCursor(token, &c); err != nil {
		return c, err
	}
	if c.Kind != "coverage-gaps-v1" || c.Ref != ref || c.High < 0 || c.Last < 0 || c.Last > c.High || c.Count < 0 {
		return c, domain.Invalid("Coverage cursor target differs or is invalid.")
	}
	at, err := time.Parse(time.RFC3339Nano, c.Observed)
	if err != nil || time.Since(at) > coverageCursorLifetime || at.After(time.Now().Add(time.Minute)) {
		return c, coverageExpired()
	}
	return c, s.verifyCoverageSnapshot(ctx, c)
}
func (s *Store) verifyCoverageSnapshot(ctx context.Context, c coverageCursor) error {
	count, digest, err := s.gapSnapshot(ctx, c.Ref, c.High)
	if err != nil {
		return err
	}
	if count != c.Count || digest != c.Digest {
		return coverageExpired()
	}
	return nil
}

func (s *Store) readCoverageGapPage(
	ctx context.Context,
	c coverageCursor,
	limit int,
) ([]domain.Gap, bool, int64, error) {
	rows, err := s.DB.QueryContext(
		ctx,
		"SELECT id,started_at,ended_at,reason FROM collection_gaps WHERE conversation_type=? AND group_id=? AND id>? AND id<=? ORDER BY id LIMIT ?",
		c.Ref.Type,
		c.Ref.ID,
		c.Last,
		c.High,
		limit+1,
	)
	if err != nil {
		return nil, false, c.Last, err
	}
	defer rows.Close()
	gaps := []domain.Gap{}
	for rows.Next() {
		var id int64
		var from string
		var to sql.NullString
		var gap domain.Gap
		if err = rows.Scan(&id, &from, &to, &gap.Reason); err != nil {
			return nil, false, c.Last, err
		}
		if len(gaps) == limit {
			return gaps, true, c.Last, nil
		}
		gap.From, err = time.Parse(time.RFC3339Nano, from)
		if err != nil {
			return nil, false, c.Last, err
		}
		if to.Valid {
			at, parseErr := time.Parse(time.RFC3339Nano, to.String)
			if parseErr != nil {
				return nil, false, c.Last, parseErr
			}
			gap.To = &at
		}
		gaps = append(gaps, gap)
		c.Last = id
	}
	return gaps, false, c.Last, rows.Err()
}
