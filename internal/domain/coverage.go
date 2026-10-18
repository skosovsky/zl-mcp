package domain

import "context"

type compactCoverageKey struct{}

// WithCompactCoverage opts one invocation into the bounded coverage representation.
// It carries no authorization; storage rechecks current collection policy.
func WithCompactCoverage(ctx context.Context) context.Context {
	return context.WithValue(ctx, compactCoverageKey{}, true)
}
func CompactCoverage(ctx context.Context) bool {
	v, _ := ctx.Value(compactCoverageKey{}).(bool)
	return v
}

type CoverageDetails struct {
	Mode           string  `json:"mode"`
	TotalKnownGaps int     `json:"total_known_gaps"`
	GapsTruncated  bool    `json:"gaps_truncated"`
	GapsCursor     *string `json:"gaps_cursor"`
	SnapshotAt     string  `json:"snapshot_at"`
}
type CoverageGaps struct {
	ConversationType string  `json:"conversation_type"`
	ConversationID   string  `json:"conversation_id"`
	Gaps             []Gap   `json:"gaps"`
	TotalKnownGaps   int     `json:"total_known_gaps"`
	HasMore          bool    `json:"has_more"`
	NextCursor       *string `json:"next_cursor"`
	SnapshotAt       string  `json:"snapshot_at"`
}
