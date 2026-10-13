package mobilebackup

import (
	"context"
	"strconv"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// SnapshotRecord is private until an explicitly bounded transport view is built.
// Its global ID may be absent; its archive ID must never identify a send anchor.
type SnapshotRecord struct {
	ArchiveRowID              string                 `json:"-"`
	GlobalID, SenderID        string                 `json:"-"`
	Conversation              domain.ConversationRef `json:"-"`
	Direction, TextKind, Text string                 `json:"-"`
	TimestampMS               int64                  `json:"-"`
}

func (SnapshotRecord) String() string   { return "archive snapshot record [redacted]" }
func (SnapshotRecord) GoString() string { return "archive snapshot record [redacted]" }

// TransportRecord returns an explicit projection; the public adapter must apply
// collection policy, tombstones, excerpts and resource ownership before delivery.
func (r SnapshotRecord) TransportRecord() map[string]any {
	var global, sender any
	if r.GlobalID != "" {
		global = r.GlobalID
	}
	mapping := "unavailable"
	if r.SenderID != "" {
		sender = r.SenderID
		mapping = "verified"
	}
	return map[string]any{
		"archive_row_id":        r.ArchiveRowID,
		"zalo_message_id":       global,
		"conversation_type":     r.Conversation.Type,
		"conversation_id":       r.Conversation.ID,
		"sender_id":             sender,
		"sender_mapping":        mapping,
		"direction":             r.Direction,
		fieldTimestamp:          time.UnixMilli(r.TimestampMS).UTC().Format(time.RFC3339Nano),
		fieldText:               r.Text,
		"text_kind":             r.TextKind,
		"quote_anchor_eligible": false,
	}
}

//nolint:recvcheck // Value String/GoString must redact formatted values; pointer methods mutate or clear owned state.
type SnapshotPage struct {
	Records                                                         []SnapshotRecord `json:"-"`
	Coverage                                                        SQLiteCoverage   `json:"-"`
	WALMode                                                         bool             `json:"-"`
	UnsupportedMetadataFields, UnresolvedQuotes, UnresolvedMentions int              `json:"-"`
	Examined, Rejected, Expired, UnresolvedSenders                  int              `json:"-"`
	SourceInformation                                               int
	SourceNativeExcluded, SourceRecallRows, SuppressedSource        int            `json:"-"`
	Unsupported                                                     map[string]int `json:"-"`
	HasMore                                                         bool           `json:"-"`
	Next                                                            *SQLiteCursor  `json:"-"`
}

func (SnapshotPage) String() string   { return "archive snapshot page [redacted]" }
func (SnapshotPage) GoString() string { return "archive snapshot page [redacted]" }
func (p *SnapshotPage) Clear() {
	if p != nil {
		clear(p.Records)
		clear(p.Unsupported)
		//nolint:exhaustruct_v5 // Clearing owned state resets every field to its zero value, including nil slices and pointers.
		*p = SnapshotPage{}
	}
}

// ReadSnapshotPage performs only local projection of an authenticated retained
// source. It neither remaps senders on the network nor produces import records.
//
//nolint:nonamedreturns // Deferred cleanup observes the returned result/error to preserve ownership and panic/error semantics.
func (a AccountArchive) ReadSnapshotPage(
	ctx context.Context,
	sourceID string,
	ref domain.ConversationRef,
	scratch string,
	since, until time.Time,
	size int,
	after *SQLiteCursor,
	order string,
	nowMS int64,
) (result SnapshotPage, err error) {
	if ctx == nil || ctx.Err() != nil || sourceID != a.retainedSourceID || !canonicalIdentity(a.ownerAccount) ||
		nowMS <= 0 {
		return result, ErrArchive
	}
	if _, valid := retainedName(sourceID); !valid {
		return result, ErrArchive
	}
	index, err := a.ConversationIndex(ctx, ref)
	if err != nil {
		return result, err
	}
	batch, err := ReadSnapshotSQLitePage(ctx, a.archive.Files[index], scratch, since, until, size, after, order)
	defer batch.Clear()
	if err != nil {
		return result, err
	}
	defer func() {
		if err != nil {
			result.Clear()
		}
	}()
	result.Coverage = batch.Coverage
	result.WALMode = batch.WALMode
	result.Examined = batch.Examined
	result.Rejected = batch.Rejected
	result.SourceInformation = batch.SourceInformation
	result.SourceNativeExcluded = batch.SourceNativeExcluded
	result.SourceRecallRows = batch.SourceRecallRows
	result.HasMore = batch.HasMore
	if batch.Next != nil {
		pageAfter := *batch.Next
		result.Next = &pageAfter
	}
	result.Unsupported = map[string]int{}
	mapped := map[string]string{}
	for _, pair := range a.pairs {
		if !pair.Group {
			mapped[pair.Plain] = pair.Session
		}
	}
	for _, row := range batch.Rows {
		if rowErr := a.appendSnapshotRow(
			ctx,
			row,
			&batch,
			mapped,
			ref,
			sourceID,
			index,
			nowMS,
			&result,
		); rowErr != nil {
			return result, rowErr
		}
	}
	if ctx.Err() != nil {
		return result, ErrArchive
	}
	return result, nil
}

func (a AccountArchive) appendSnapshotRow(
	ctx context.Context,
	row SQLiteRow,
	batch *SQLiteBatch,
	mapped map[string]string,
	ref domain.ConversationRef,
	sourceID string,
	index int,
	nowMS int64,
	result *SnapshotPage,
) error {
	if ctx.Err() != nil {
		return ErrArchive
	}
	if row.Type == 36 || row.Type != 20 && batch.recalls.suppresses(row) {
		result.SuppressedSource++
		return nil
	}
	if row.Type == nativeInformationalRowType {
		return inspectExcludedSnapshotRow(ctx, row, result)
	}
	expiry, declared, e := MessageExpiryMS(row.TimestampMS, row.TTL)
	if e != nil {
		result.Unsupported["invalid_expiry"]++
		//nolint:nilerr // Invalid expiry is a counted row omission, not a failure of the bounded snapshot page.
		return nil
	}
	if declared && nowMS >= expiry {
		result.Expired++
		return nil
	}
	kind, known := mobilePayloadKind(row.Type)
	if !known {
		result.Unsupported["unknown_content_kind"]++
		return nil
	}
	if len(row.BinNet) == 0 {
		result.Unsupported["missing_metadata"]++
		return nil
	}
	text, rich, visible, projectionErr := snapshotRowProjection(ctx, row, kind, result)
	if projectionErr != nil || !visible {
		return projectionErr
	}
	sender := mapped[row.SenderID]
	direction := "unknown"
	if sender != "" {
		if ref.Type == domain.ConversationDirect && sender != a.ownerAccount && sender != ref.ID {
			result.Unsupported["sender_conversation_mismatch"]++
			return nil
		}
		direction = directionIncoming
		if sender == a.ownerAccount {
			direction = directionOutgoing
		}
	} else {
		result.UnresolvedSenders++
	}
	textKind := contentKindPlain
	if rich {
		textKind = "rich"
	}
	identity := "ar:" + sourceID + ":" + strconv.Itoa(index) + ":" + strconv.FormatInt(row.SourceRowID, 10)
	result.Records = append(
		result.Records,
		SnapshotRecord{
			ArchiveRowID: identity,
			GlobalID:     row.MessageID,
			SenderID:     sender,
			Conversation: ref,
			Direction:    direction,
			TextKind:     textKind,
			Text:         text,
			TimestampMS:  row.TimestampMS,
		},
	)
	return nil
}

func inspectExcludedSnapshotRow(ctx context.Context, row SQLiteRow, result *SnapshotPage) error {
	// The native SQL query excludes this format type before conversion.
	// Metadata is only diagnostic; it cannot turn the body into visible text.
	metadata, metadataErr := ParseBinNet(ctx, row.BinNet)
	if ctx.Err() != nil {
		metadata.Clear()
		return ErrArchive
	}
	if metadataErr != nil {
		result.Unsupported["invalid_excluded_metadata"]++
		result.Unsupported["native_excluded"]++
	} else {
		result.UnsupportedMetadataFields += metadata.UnsupportedFields
		if metadata.Quote != nil {
			result.UnresolvedQuotes++
		}
		result.UnresolvedMentions += len(metadata.Mentions)
		if snapshotInformation(ctx, row.BinNet) {
			result.Unsupported["native_information"]++
		} else {
			result.Unsupported["native_excluded"]++
		}
	}
	metadata.Clear()
	return nil
}

func snapshotRowProjection(
	ctx context.Context,
	row SQLiteRow,
	kind string,
	result *SnapshotPage,
) (string, bool, bool, error) {
	metadata, e := ParseBinNet(ctx, row.BinNet)
	if ctx.Err() != nil {
		metadata.Clear()
		return "", false, false, ErrArchive
	}
	if e != nil {
		metadata.Clear()
		result.Unsupported["invalid_metadata"]++
		//nolint:nilerr // Malformed row metadata is counted and omitted; it does not invalidate the complete bounded page.
		return "", false, false, nil
	}
	//nolint:exhaustruct_v5 // PreparedRow keeps absent decoded metadata and zero counters unchanged until this phase populates them.
	text, rich, supported, valid := archiveText(PreparedRow{Row: row, Kind: kind, Metadata: metadata})
	result.UnsupportedMetadataFields += metadata.UnsupportedFields
	if metadata.Quote != nil {
		result.UnresolvedQuotes++
	}
	result.UnresolvedMentions += len(metadata.Mentions)
	metadata.Clear()
	if !valid {
		result.Unsupported["invalid_text_projection"]++
		return "", false, false, nil
	}
	if !supported {
		result.Unsupported["unsupported_content"]++
		return "", false, false, nil
	}
	if text == "" {
		result.Unsupported["empty_text_projection"]++
		return "", false, false, nil
	}
	return text, rich, true, nil
}
