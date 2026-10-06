package mobilebackup

import (
	"context"
	"crypto/sha256"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// OwnDirectRecallSet is private whole-source evidence, not a live recall request.
type OwnDirectRecallSet struct {
	requestID, fingerprint, account, filename string
	digest                                    [32]byte
	Recalls                                   []domain.MobileHistoryRecall `json:"-"`
}

func (OwnDirectRecallSet) String() string   { return "mobile backup control set [redacted]" }
func (OwnDirectRecallSet) GoString() string { return "mobile backup control set [redacted]" }
func (p *OwnDirectRecallSet) Clear() {
	if p != nil {
		clear(p.Recalls)
		*p = OwnDirectRecallSet{}
	}
}

// ReadOwnDirectRecallSet classifies all controls before returning any proof. It
// borrows a selected immutable image and the existing session mapper; it never
// writes the corpus, requests the phone or changes persistence eligibility.
func ReadOwnDirectRecallSet(parent context.Context, archive SelectedArchive, request domain.MobileBackupRequest, account, scratch string, maxControls int, mapper domain.MobileIdentitySource) (result OwnDirectRecallSet, err error) {
	r, e := request.Normalize()
	if parent == nil || parent.Err() != nil || e != nil || r.ConversationType != domain.ConversationDirect || !canonicalIdentity(account) || mapper == nil || maxControls < 1 || maxControls > 5000 || archive.ref != r.Ref() || archive.requestID != r.RequestID || archive.requestFingerprint != r.Fingerprint() {
		return result, ErrSQLite
	}
	digest := sha256.Sum256(archive.File.Data)
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	defer func() {
		if err != nil {
			result.Clear()
		}
	}()
	since, until := time.UnixMilli(0), time.UnixMilli(253402300800000)
	var after *SQLiteCursor
	count, examined := -1, 0
	seen := map[string]bool{}
	mapped := map[string]string{}
	plainOwner := ""
	for {
		batch, readErr := readSQLitePage(ctx, archive.File, scratch, since, until, min(50, maxControls-examined), after, true)
		if readErr != nil {
			return result, ErrSQLite
		}
		e = func() error {
			defer batch.Clear()
			if batch.WALMode || batch.SourceControls > maxControls || batch.Rejected != 0 {
				return ErrSnapshotSourceUnsupported
			}
			if count == -1 {
				count = batch.SourceControls
			}
			if count != batch.SourceControls || batch.Examined != len(batch.Rows) || examined+batch.Examined > count {
				return ErrSQLite
			}
			request := domain.MobileIdentityRequest{}
			requested := map[string]bool{}
			for _, row := range batch.Rows {
				if ctx.Err() != nil || row.Type != 36 || row.Status != 3 || seen[row.MessageID] {
					return ErrSnapshotSourceUnsupported
				}
				seen[row.MessageID] = true
				if mapped[row.SenderID] == "" && !requested[row.SenderID] {
					requested[row.SenderID] = true
					request.Direct = append(request.Direct, row.SenderID)
				}
			}
			if len(request.Direct) > 0 {
				pairs, mapErr := mapper.MapMobileBackupIdentities(ctx, request)
				defer clear(pairs)
				if mapErr != nil || ctx.Err() != nil || len(pairs) != len(request.Direct) {
					return ErrSQLite
				}
				for _, pair := range pairs {
					if pair.Group || !requested[pair.Plain] || mapped[pair.Plain] != "" || pair.Session != account || !canonicalIdentity(pair.Plain) || (plainOwner != "" && plainOwner != pair.Plain) {
						return ErrSnapshotSourceUnsupported
					}
					mapped[pair.Plain] = pair.Session
					plainOwner = pair.Plain
				}
			}
			for _, row := range batch.Rows {
				if mapped[row.SenderID] != account {
					return ErrSnapshotSourceUnsupported
				}
				result.Recalls = append(result.Recalls, domain.MobileHistoryRecall{Conversation: r.Ref(), MessageID: row.MessageID, SenderID: account, RecordAtMS: row.TimestampMS})
			}
			examined += batch.Examined
			if batch.HasMore {
				if batch.Next == nil || batch.Examined == 0 || examined >= maxControls || (after != nil && (batch.Next.TimestampMS < after.TimestampMS || batch.Next.TimestampMS == after.TimestampMS && batch.Next.RowID <= after.RowID)) {
					return ErrSQLite
				}
				next := *batch.Next
				after = &next
			} else {
				after = nil
			}
			return nil
		}()
		if e != nil {
			return result, e
		}
		if after == nil {
			break
		}
	}
	if ctx.Err() != nil || examined != count || len(result.Recalls) != count || sha256.Sum256(archive.File.Data) != digest {
		return result, ErrSQLite
	}
	result.requestID, result.fingerprint, result.account, result.filename = r.RequestID, r.Fingerprint(), account, archive.File.Name
	result.digest = digest
	return result, nil
}
