package service

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
)

// Positions and read claims are private plaintext only inside an AEAD token.
type archivePosition struct {
	Digest      [32]byte `json:"digest"`
	Name        string   `json:"name"`
	SinceMS     int64    `json:"since"`
	UntilMS     int64    `json:"until"`
	TimestampMS int64    `json:"timestamp"`
	RowID       int64    `json:"row"`
	Descending  bool     `json:"descending"`
	Snapshot    bool     `json:"snapshot"`
}
type archiveReadToken struct {
	Version  int                    `json:"v"`
	Kind     string                 `json:"kind"`
	Account  string                 `json:"account"`
	Source   string                 `json:"source"`
	Digest   string                 `json:"digest"`
	Ref      domain.ConversationRef `json:"ref"`
	Since    string                 `json:"since"`
	Until    string                 `json:"until"`
	Order    string                 `json:"order"`
	Position *archivePosition       `json:"position"`
	Target   string                 `json:"target,omitempty"`
	Limit    int                    `json:"limit,omitempty"`
}

func (archivePosition) String() string   { return "archive paging position [redacted]" }
func (archivePosition) GoString() string { return "archive paging position [redacted]" }

func (archiveReadToken) String() string   { return "archive read token [redacted]" }
func (archiveReadToken) GoString() string { return "archive read token [redacted]" }
func archivePagePosition(value *mobilebackup.SQLiteCursor) *archivePosition {
	if value == nil {
		return nil
	}
	return &archivePosition{value.Digest, value.Name, value.SinceMS, value.UntilMS, value.TimestampMS, value.RowID, value.Descending, value.Snapshot}
}
func (value *archivePosition) cursor() *mobilebackup.SQLiteCursor {
	if value == nil {
		return nil
	}
	return &mobilebackup.SQLiteCursor{Digest: value.Digest, Name: value.Name, SinceMS: value.SinceMS, UntilMS: value.UntilMS, TimestampMS: value.TimestampMS, RowID: value.RowID, Descending: value.Descending, Snapshot: value.Snapshot}
}
func (p *membershipPort) sealArchiveRead(ctx context.Context, purpose string, token archiveReadToken) (string, error) {
	body, err := json.Marshal(token)
	if err != nil {
		return "", archiveInvalidCursor()
	}
	defer clear(body)
	return p.library.SealArchiveToken(ctx, purpose, body)
}
func (p *membershipPort) openArchiveRead(ctx context.Context, purpose, sealed string) (archiveReadToken, error) {
	var token archiveReadToken
	body, err := p.library.OpenArchiveToken(ctx, purpose, sealed)
	if err != nil {
		return token, archiveInvalidCursor()
	}
	defer clear(body)
	if json.Unmarshal(body, &token) != nil || token.Version != 1 || token.Kind != purpose || !token.Ref.Valid() {
		return archiveReadToken{}, archiveInvalidCursor()
	}
	return token, nil
}
func archiveDate(ms int64) string { return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano) }
func archiveWindow(token archiveReadToken) (time.Time, time.Time, error) {
	since, e1 := time.Parse(time.RFC3339Nano, token.Since)
	until, e2 := time.Parse(time.RFC3339Nano, token.Until)
	if e1 != nil || e2 != nil || !since.Before(until) {
		return time.Time{}, time.Time{}, domain.Invalid("Archive interval must be an explicit RFC3339 [since, until) window.")
	}
	return since, until, nil
}
func archiveProjectionError(err error) error {
	if errors.Is(err, mobilebackup.ErrSnapshotControls) {
		return &domain.Error{Code: "SOURCE_CONTROLS_UNCLASSIFIED", Message: "This archived conversation contains controls whose visibility semantics are not verified; no message prefix was returned.", Details: map[string]any{}}
	}
	return &domain.Error{Code: "SOURCE_UNSUPPORTED", Message: "The selected archived conversation cannot be safely projected.", Details: map[string]any{}}
}
func (p *membershipPort) snapshotPage(ctx context.Context, source mobilebackup.AccountArchive, token archiveReadToken, limit int) (mobilebackup.SnapshotPage, error) {
	since, until, err := archiveWindow(token)
	if err != nil {
		return mobilebackup.SnapshotPage{}, err
	}
	scratch, err := os.MkdirTemp(p.stateDir, "archive-reading-")
	if err != nil {
		return mobilebackup.SnapshotPage{}, archiveReadUnavailable()
	}
	defer os.RemoveAll(scratch)
	page, err := source.ReadSnapshotPage(ctx, token.Source, token.Ref, scratch, since, until, limit, token.Position.cursor(), token.Order, time.Now().UnixMilli())
	if err != nil {
		page.Clear()
		return mobilebackup.SnapshotPage{}, archiveProjectionError(err)
	}
	return page, nil
}
func (p *membershipPort) archiveMessages(ctx context.Context, args map[string]any, binding string) (map[string]any, error) {
	ref := domain.ConversationRef{Type: archiveString(args, "conversation_type"), ID: archiveString(args, "conversation_id")}
	if !p.store.AllowsConversation(ref) {
		return nil, &domain.Error{Code: "PERMISSION_DENIED", Message: "Conversation is outside collection policy.", Details: map[string]any{}}
	}
	sourceID := archiveString(args, "source_id")
	source, manifest, err := p.library.ReadBound(ctx, sourceID, binding)
	if err != nil {
		return nil, archiveReadUnavailable()
	}
	defer source.Clear()
	receipt, err := p.library.PreservationStatus(ctx, sourceID, binding)
	if err != nil {
		return nil, archiveReadUnavailable()
	}
	order := archiveString(args, "order")
	if order == "" {
		order = "desc"
	}
	token := archiveReadToken{Version: 1, Kind: "cursor", Account: binding, Source: sourceID, Digest: manifest.Digest, Ref: ref, Since: archiveString(args, "since"), Until: archiveString(args, "until"), Order: order}
	since, until, err := archiveWindow(token)
	if err != nil {
		return nil, err
	}
	token.Since, token.Until = since.UTC().Format(time.RFC3339Nano), until.UTC().Format(time.RFC3339Nano)
	if sealed := archiveString(args, "cursor"); sealed != "" {
		saved, err := p.openArchiveRead(ctx, "cursor", sealed)
		if err != nil || saved.Account != token.Account || saved.Source != token.Source || saved.Digest != token.Digest || saved.Ref != token.Ref || saved.Since != token.Since || saved.Until != token.Until || saved.Order != token.Order || saved.Position == nil || saved.Target != "" || saved.Limit != 0 {
			return nil, archiveInvalidCursor()
		}
		token = saved
	}
	limit := archiveLimit(args)
	page, err := p.snapshotPage(ctx, source, token, limit)
	if err != nil {
		return nil, err
	}
	defer page.Clear()
	now := time.Now()
	values := []map[string]any{}
	suppressed := 0
	for _, record := range page.Records {
		hidden, err := p.store.ArchiveMessageSuppressed(ctx, ref, record.GlobalID, now.UnixMilli())
		if err != nil {
			return nil, err
		}
		if hidden {
			suppressed++
			continue
		}
		if len(record.Text) > 1<<20 {
			page.Unsupported["text_size_limit"]++
			continue
		}
		value, err := p.archiveExcerpt(ctx, record, token, limit)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	var next any
	if page.HasMore {
		if page.Next == nil {
			return nil, archiveProjectionError(mobilebackup.ErrSQLite)
		}
		following := token
		following.Position = archivePagePosition(page.Next)
		sealed, err := p.sealArchiveRead(ctx, "cursor", following)
		if err != nil {
			return nil, err
		}
		next = sealed
	}
	var earliest, latest any
	if page.Coverage.HasRange {
		earliest, latest = archiveDate(page.Coverage.EarliestMS), archiveDate(page.Coverage.LatestMS)
	}
	var empty any
	if len(values) == 0 {
		empty = "no_supported_text_in_page"
		if page.Coverage.PeriodRows == 0 {
			empty = "no_records_in_period"
		}
	}
	// The transport owns these counters; clearing the private page must not erase coverage.
	coverage := map[string]any{"conversation_type": ref.Type, "conversation_id": ref.ID, "source_rows": page.Coverage.SourceRows, "source_information_rows": page.SourceInformation, "source_native_excluded_rows": page.SourceNativeExcluded, "source_recall_rows": page.SourceRecallRows, "suppressed_source": page.SuppressedSource, "period_rows": page.Coverage.PeriodRows, "invalid_timestamp_rows": page.Coverage.InvalidTimestamps, "source_earliest_at": earliest, "source_latest_at": latest, "wal_mode": page.WALMode, "history_complete": false, "examined": page.Examined, "rejected": page.Rejected, "expired": page.Expired, "suppressed_live": suppressed, "unresolved_senders": page.UnresolvedSenders, "unsupported_metadata_fields": page.UnsupportedMetadataFields, "unresolved_quotes": page.UnresolvedQuotes, "unresolved_mentions": page.UnresolvedMentions, "unsupported_content": maps.Clone(page.Unsupported), "coverage_scope": "examined_page", "live_visibility": "global_ids_only"}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return map[string]any{"messages": values, "has_more": page.HasMore, "next_cursor": next, "empty_reason": empty, "coverage": coverage, "snapshot_at": manifest.CapturedAt, "coverage_observed_at": now.UTC().Format(time.RFC3339Nano), "requested_since": token.Since, "requested_until": token.Until, "source": archiveDescriptor(receipt)}, nil
}

func (p *membershipPort) archiveExcerpt(ctx context.Context, record mobilebackup.SnapshotRecord, token archiveReadToken, limit int) (map[string]any, error) {
	value := record.TransportRecord()
	runes := []rune(record.Text)
	truncated := len(runes) > 2048
	value["text_truncated"], value["resource_uri"] = truncated, nil
	if truncated {
		value["text"] = string(runes[:2048])
		resource := token
		resource.Kind, resource.Target, resource.Limit = "resource", record.ArchiveRowID, limit
		sealed, err := p.sealArchiveRead(ctx, "resource", resource)
		if err != nil {
			return nil, err
		}
		value["resource_uri"] = "zalo://archives/" + sealed
	}
	return value, nil
}

func (p *membershipPort) archiveResource(ctx context.Context, args any) (map[string]any, error) {
	input, ok := args.(map[string]any)
	if !ok || len(input) != 1 || archiveString(input, "token") == "" || p.library == nil || p.store == nil {
		return nil, archiveReadUnavailable()
	}
	token, err := p.openArchiveRead(ctx, "resource", archiveString(input, "token"))
	if err != nil || token.Target == "" || token.Limit < 1 || token.Limit > 50 {
		return nil, archiveReadUnavailable()
	}
	binding, err := p.store.ArchiveAccountKey(ctx)
	if err != nil || binding != token.Account || !p.store.AllowsConversation(token.Ref) {
		return nil, archiveReadUnavailable()
	}
	source, manifest, err := p.library.ReadBound(ctx, token.Source, binding)
	if err != nil {
		return nil, archiveReadUnavailable()
	}
	defer source.Clear()
	if manifest.Digest != token.Digest {
		return nil, archiveReadUnavailable()
	}
	page, err := p.snapshotPage(ctx, source, token, token.Limit)
	if err != nil {
		return nil, err
	}
	defer page.Clear()
	for _, record := range page.Records {
		if record.ArchiveRowID != token.Target {
			continue
		}
		hidden, err := p.store.ArchiveMessageSuppressed(ctx, token.Ref, record.GlobalID, time.Now().UnixMilli())
		if err != nil || hidden || len(record.Text) > 1<<20 || ctx.Err() != nil {
			return nil, archiveReadUnavailable()
		}
		return record.TransportRecord(), nil
	}
	return nil, archiveReadUnavailable()
}
