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
	return map[string]any{"archive_row_id": r.ArchiveRowID, "zalo_message_id": global, "conversation_type": r.Conversation.Type, "conversation_id": r.Conversation.ID, "sender_id": sender, "sender_mapping": mapping, "direction": r.Direction, "timestamp": time.UnixMilli(r.TimestampMS).UTC().Format(time.RFC3339Nano), "text": r.Text, "text_kind": r.TextKind, "quote_anchor_eligible": false}
}

type SnapshotPage struct {
	Records                                                         []SnapshotRecord `json:"-"`
	Coverage                                                        SQLiteCoverage   `json:"-"`
	WALMode                                                         bool             `json:"-"`
	UnsupportedMetadataFields, UnresolvedQuotes, UnresolvedMentions int              `json:"-"`
	Examined, Rejected, Expired, UnresolvedSenders                  int              `json:"-"`
	SourceInformation                                               int              `json:"-"`
	Unsupported                                                     map[string]int   `json:"-"`
	HasMore                                                         bool             `json:"-"`
	Next                                                            *SQLiteCursor    `json:"-"`
}

func (SnapshotPage) String() string   { return "archive snapshot page [redacted]" }
func (SnapshotPage) GoString() string { return "archive snapshot page [redacted]" }
func (p *SnapshotPage) Clear() {
	if p != nil {
		clear(p.Records)
		clear(p.Unsupported)
		*p = SnapshotPage{}
	}
}

// ReadSnapshotPage performs only local projection of an authenticated retained
// source. It neither remaps senders on the network nor produces import records.
func (a AccountArchive) ReadSnapshotPage(ctx context.Context, sourceID string, ref domain.ConversationRef, scratch string, since, until time.Time, size int, after *SQLiteCursor, order string, nowMS int64) (result SnapshotPage, err error) {
	if ctx == nil || ctx.Err() != nil || sourceID != a.retainedSourceID || !canonicalIdentity(a.ownerAccount) || nowMS <= 0 {
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
	result.HasMore = batch.HasMore
	if batch.Next != nil {
		copy := *batch.Next
		result.Next = &copy
	}
	result.Unsupported = map[string]int{}
	mapped := map[string]string{}
	for _, pair := range a.pairs {
		if !pair.Group {
			mapped[pair.Plain] = pair.Session
		}
	}
	for _, row := range batch.Rows {
		if ctx.Err() != nil {
			return result, ErrArchive
		}
		if row.Type == 20 {
			// Whole-source classification already validated every type-20 action.
			// Advance the ordinary row cursor and report omission, without projecting
			// source text, title, quote, mentions or interactive action parameters.
			metadata, metadataErr := ParseBinNet(ctx, row.BinNet)
			if metadataErr != nil || ctx.Err() != nil {
				metadata.Clear()
				return result, ErrArchive
			}
			result.UnsupportedMetadataFields += metadata.UnsupportedFields
			if metadata.Quote != nil {
				result.UnresolvedQuotes++
			}
			result.UnresolvedMentions += len(metadata.Mentions)
			metadata.Clear()
			result.Unsupported["native_information"]++
			continue
		}
		expiry, declared, e := MessageExpiryMS(row.TimestampMS, row.TTL)
		if e != nil {
			result.Unsupported["invalid_expiry"]++
			continue
		}
		if declared && nowMS >= expiry {
			result.Expired++
			continue
		}
		kind, known := mobilePayloadKind(row.Type)
		if !known {
			result.Unsupported["unknown_content_kind"]++
			continue
		}
		if len(row.BinNet) == 0 {
			result.Unsupported["missing_metadata"]++
			continue
		}
		metadata, e := ParseBinNet(ctx, row.BinNet)
		if ctx.Err() != nil {
			metadata.Clear()
			return result, ErrArchive
		}
		if e != nil {
			metadata.Clear()
			result.Unsupported["invalid_metadata"]++
			continue
		}
		text, rich, supported, valid := archiveText(PreparedRow{Row: row, Kind: kind, Metadata: metadata})
		result.UnsupportedMetadataFields += metadata.UnsupportedFields
		if metadata.Quote != nil {
			result.UnresolvedQuotes++
		}
		result.UnresolvedMentions += len(metadata.Mentions)
		metadata.Clear()
		if !valid {
			result.Unsupported["invalid_text_projection"]++
			continue
		}
		if !supported {
			result.Unsupported["unsupported_content"]++
			continue
		}
		if text == "" {
			result.Unsupported["empty_text_projection"]++
			continue
		}
		sender := mapped[row.SenderID]
		direction := "unknown"
		if sender != "" {
			if ref.Type == domain.ConversationDirect && sender != a.ownerAccount && sender != ref.ID {
				result.Unsupported["sender_conversation_mismatch"]++
				continue
			}
			direction = "incoming"
			if sender == a.ownerAccount {
				direction = "outgoing"
			}
		} else {
			result.UnresolvedSenders++
		}
		textKind := "plain"
		if rich {
			textKind = "rich"
		}
		identity := "ar:" + sourceID + ":" + strconv.Itoa(index) + ":" + strconv.FormatInt(row.SourceRowID, 10)
		result.Records = append(result.Records, SnapshotRecord{ArchiveRowID: identity, GlobalID: row.MessageID, SenderID: sender, Conversation: ref, Direction: direction, TextKind: textKind, Text: text, TimestampMS: row.TimestampMS})
	}
	if ctx.Err() != nil {
		return result, ErrArchive
	}
	return result, nil
}
