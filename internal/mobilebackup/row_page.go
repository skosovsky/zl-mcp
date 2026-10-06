package mobilebackup

import (
	"context"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type PreparedRow struct {
	Row                       SQLiteRow `json:"-"`
	SenderID, Direction, Kind string    `json:"-"`
	Metadata                  BinNet    `json:"-"`
	ExpiresMS                 int64     `json:"-"`
	ExpiryDeclared            bool      `json:"-"`
	QuoteExpired              bool      `json:"-"`
}

func (PreparedRow) String() string   { return "mobile backup prepared row [redacted]" }
func (PreparedRow) GoString() string { return "mobile backup prepared row [redacted]" }

type PreparedRowPage struct {
	Rows                                                                                                      []PreparedRow                `json:"-"`
	OwnRecalls                                                                                                []domain.MobileHistoryRecall `json:"-"`
	Examined, DeferredControls, UnsupportedTypes, MissingMetadata, InvalidMetadata, UnsupportedMetadataFields int                          `json:"-"`
}

func (PreparedRowPage) String() string   { return "mobile backup prepared row page [redacted]" }
func (PreparedRowPage) GoString() string { return "mobile backup prepared row page [redacted]" }
func (p *PreparedRowPage) Clear() {
	if p == nil {
		return
	}
	for i := range p.Rows {
		clear(p.Rows[i].Row.BinNet)
		p.Rows[i].Metadata.Clear()
		p.Rows[i] = PreparedRow{}
	}
	clear(p.OwnRecalls)
	*p = PreparedRowPage{}
}
func mobilePayloadKind(t int64) (string, bool) {
	switch t {
	case 0:
		return "webchat", true
	case 2:
		return "chat.doodle", true
	case 3, 4, 31:
		return "chat.photo", true
	case 6:
		return "chat.voice", true
	case 10:
		return "chat.sticker", true
	case 12:
		return "chat.recommended", true
	case 15:
		return "chat.list.action", true
	case 18:
		return "chat.location.new", true
	case 19:
		return "chat.video.msg", true
	case 22:
		return "share.file", true
	case 23:
		return "chat.gif", true
	case 24:
		return "chat.webcontent", true
	case 53:
		return "chat.webcontent.v2", true
	}
	return "", false
}
func deferredMobileControl(t int64) bool {
	switch t {
	case 20, 21, 25, 26, 29, 32, 33, 34, 35, 36, 45, 51, 52:
		return true
	}
	return false
}
func PrepareRowPage(ctx context.Context, rows []SQLiteRow, r domain.MobileBackupRequest, account string, mapper domain.MobileIdentitySource) (result PreparedRowPage, err error) {
	if ctx == nil || ctx.Err() != nil || len(rows) > 50 || !canonicalIdentity(account) || mapper == nil {
		return result, ErrSQLite
	}
	normalized, e := r.Normalize()
	if e != nil || !canonicalIdentity(normalized.ConversationID) || len(rows) > normalized.MaxMessages {
		return result, ErrSQLite
	}
	since, _ := time.Parse(time.RFC3339Nano, normalized.Since)
	until, _ := time.Parse(time.RFC3339Nano, normalized.Until)
	from, to, validWindow := millisecondWindow(since, until)
	if !validWindow {
		return result, ErrSQLite
	}
	defer func() {
		if err != nil {
			result.Clear()
		}
	}()
	// Whole-page validation comes before source mapping or row classification.
	owned := make([]SQLiteRow, 0, len(rows))
	retained := 0
	defer func() {
		for i := range owned {
			clear(owned[i].BinNet)
		}
	}()
	for _, row := range rows {
		retained += len(row.Text) + len(row.BinNet)
		if retained > 8<<20 {
			return result, ErrSQLite
		}
		if ctx.Err() != nil {
			return result, ErrSQLite
		}
		verified, ok := sqliteRow([9]any{row.SenderID, row.MessageID, row.ClientID, row.Text, row.TimestampMS, row.TTL, row.Type, row.Status, row.BinNet}, from, to)
		if !ok {
			return result, ErrSQLite
		}
		if _, _, expiryErr := MessageExpiryMS(verified.TimestampMS, verified.TTL); expiryErr != nil {
			clear(verified.BinNet)
			return result, ErrSQLite
		}
		owned = append(owned, verified)
	}
	result.Examined = len(rows)
	request := IdentityRequest{}
	seen := map[string]bool{}
	// Only the own/direct/status-3 transition has paired live archive evidence.
	// Retain no body or BinNet for a recall and keep its deferred-control count.
	controls := map[string][]SQLiteRow{}
	for i, row := range owned {
		if ctx.Err() != nil {
			return result, ErrSQLite
		}
		if deferredMobileControl(row.Type) {
			result.DeferredControls++
			if normalized.ConversationType == domain.ConversationDirect && row.Type == 36 && row.Status == 3 {
				controls[row.SenderID] = append(controls[row.SenderID], row)
				if !seen[row.SenderID] {
					seen[row.SenderID] = true
					request.Direct = append(request.Direct, row.SenderID)
				}
			}
			continue
		}
		kind, known := mobilePayloadKind(row.Type)
		if !known {
			result.UnsupportedTypes++
			continue
		}
		if len(row.BinNet) == 0 {
			result.MissingMetadata++
			continue
		}
		metadata, e := ParseBinNet(ctx, row.BinNet)
		if e != nil {
			if ctx.Err() != nil {
				return result, ErrSQLite
			}
			result.InvalidMetadata++
			continue
		}
		result.UnsupportedMetadataFields += metadata.UnsupportedFields
		expiresMS, expiryDeclared, _ := MessageExpiryMS(row.TimestampMS, row.TTL) // Whole page checked above.
		result.Rows = append(result.Rows, PreparedRow{Row: row, Kind: kind, Metadata: metadata, ExpiresMS: expiresMS, ExpiryDeclared: expiryDeclared})
		owned[i].BinNet = nil // Ownership transferred to the returned candidate.
		if !seen[row.SenderID] {
			seen[row.SenderID] = true
			request.Direct = append(request.Direct, row.SenderID)
		}
	}
	if len(request.Direct) == 0 {
		if ctx.Err() != nil {
			return result, ErrSQLite
		}
		return result, nil
	}
	pairs, e := mapper.MapMobileBackupIdentities(ctx, request)
	defer clear(pairs)
	if e != nil || ctx.Err() != nil || len(pairs) != len(request.Direct) {
		return result, ErrSQLite
	}
	mapped := map[string]string{}
	targets := map[string]bool{}
	for _, pair := range pairs {
		if ctx.Err() != nil || pair.Group || !canonicalIdentity(pair.Plain) || !canonicalIdentity(pair.Session) || !seen[pair.Plain] || mapped[pair.Plain] != "" || targets[pair.Session] {
			return result, ErrSQLite
		}
		mapped[pair.Plain] = pair.Session
		targets[pair.Session] = true
	}
	for i := range result.Rows {
		row := &result.Rows[i]
		row.SenderID = mapped[row.Row.SenderID]
		if row.SenderID == "" || normalized.ConversationType == domain.ConversationDirect && row.SenderID != account && row.SenderID != normalized.ConversationID {
			return result, ErrSQLite
		}
		row.Direction = "incoming"
		if row.SenderID == account {
			row.Direction = "outgoing"
		}
	}
	seenTargets := map[string]bool{}
	// Preserve encounter order; maps are only used for ownership/membership tests.
	for _, source := range request.Direct {
		if len(controls[source]) == 0 {
			continue
		}
		sender := mapped[source]
		if sender != account && sender != normalized.ConversationID {
			return result, ErrSQLite
		}
		if sender != account {
			continue // Received-message recall semantics are not yet verified.
		}
		for _, control := range controls[source] {
			id := control.MessageID
			if seenTargets[id] {
				return result, ErrSQLite
			}
			seenTargets[id] = true
			result.OwnRecalls = append(result.OwnRecalls, domain.MobileHistoryRecall{Conversation: normalized.Ref(), MessageID: id, SenderID: sender, RecordAtMS: control.TimestampMS})
		}
	}
	if ctx.Err() != nil {
		return result, ErrSQLite
	}
	return result, nil
}
