package mobilebackup

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func conversionPage(t *testing.T) (PreparedArchivePage, domain.MobileBackupRequest, int64) {
	t.Helper()
	r := selectedFetchRequest()
	r.MaxMessages = 5
	normalized, _ := r.Normalize()
	ts := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC).UnixMilli()
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for _, i := range []int{1, 2, 4, 5} {
			sender := "901"
			ttl := int64(0)
			if i == 1 {
				ttl = 1
			}
			if i == 2 {
				sender = "01"
			}
			if _, e := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", sender, fmt.Sprint(i), fmt.Sprint(i), "synthetic", ts, ttl, 0, 1, []byte{0, 0, 0, 9, 0, 0, 0, 1, 'x'}); e != nil {
				t.Fatal(e)
			}
		}
	})
	archive := SelectedArchive{File: ArchiveFile{Name: "902.db", Data: data}, ref: normalized.Ref(), requestID: normalized.RequestID, requestFingerprint: normalized.Fingerprint()}
	defer archive.Clear()
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC).UnixMilli()
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		return []domain.MobileIdentityPair{{Plain: "901", Session: "12"}}, nil
	})
	page, e := ReadPreparedArchivePage(context.Background(), archive, r, "10", t.TempDir(), now, 50, nil, mapper)
	if e != nil {
		t.Fatal(e)
	}
	return page, r, now
}

func TestMobileRichTextProjectionBoundaries(t *testing.T) {
	for _, mode := range []string{"title", "empty", "absent", "different-action", "multiple", "invalid-utf8", "oversized", "unknown-tag", "wal", "control"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange: owned candidates; rich title must not overwrite the source row.
			page, request, now := conversionPage(t)
			defer page.Clear()
			a := Attachment{Action: AttachmentValue{Present: true, Bytes: []byte("rtf")}, Title: AttachmentValue{Present: true, Bytes: []byte("Visible formatted text")}}
			switch mode {
			case "empty":
				a.Title.Bytes = nil
			case "absent":
				a.Title = AttachmentValue{}
			case "different-action":
				a.Action.Bytes = []byte("ecard")
			case "invalid-utf8":
				a.Title.Bytes = []byte{0xff}
			case "oversized":
				a.Title.Bytes = []byte(strings.Repeat("x", (1<<20)+1))
			}
			row := &page.Candidates.Rows[0]
			original := row.Row.Text
			row.Metadata.Attachments = []Attachment{a}
			if mode == "multiple" {
				row.Metadata.Attachments = append(row.Metadata.Attachments, a)
			}
			if mode == "unknown-tag" {
				row.Metadata.UnsupportedTags = []uint32{6}
			}
			if mode == "wal" {
				page.SourceWAL = true
			}
			if mode == "control" {
				page.SourceControls = 1
			}
			// Act.
			got, err := ConvertPreparedArchivePage(context.Background(), page, request, "10", now)
			defer got.Clear()
			// Assert: native fallback, explicit gaps, or whole-page rejection with no prefix.
			if mode == "invalid-utf8" || mode == "oversized" || mode == "wal" || mode == "control" {
				if err == nil || len(got.Records) != 0 {
					t.Fatal("invalid rich text yielded records")
				}
				return
			}
			if err != nil || row.Row.Text != original {
				t.Fatal("source mutation or conversion failure", err)
			}
			if mode == "multiple" || mode == "different-action" || mode == "unknown-tag" {
				if got.UnsupportedContent != 1 || len(got.Records) != 1 {
					t.Fatal("ambiguous attachment accepted")
				}
				return
			}
			want := "Visible formatted text"
			if mode == "empty" || mode == "absent" {
				want = original
			}
			if len(got.Records) != 2 || got.Records[0].Message.Text != want || len(got.Records[0].Message.AttachmentTypes) != 1 || got.Records[0].Message.AttachmentTypes[0] != "rtf" {
				t.Fatal("native rich-text rule lost")
			}
		})
	}
}
func TestMobileMessageConversionPreservesExactIDsAndOriginalMetadata(t *testing.T) {
	// Arrange: fully selected/mapped/expiry-checked page with explicit source gaps.
	page, r, now := conversionPage(t)
	defer page.Clear()
	page.Candidates.Rows[0].Row.MessageID = "18446744073709551615"
	page.Candidates.Rows[0].Row.ClientID = "9007199254740993"
	// Act.
	got, e := ConvertPreparedArchivePage(context.Background(), page, r, "10", now)
	defer got.Clear()
	// Assert: IDs never cross float, and silent ingestion policy is not assigned here.
	if e != nil || len(got.Records) != 2 {
		t.Fatal("conversion failed", e)
	}
	m := got.Records[0].Message
	if m.ID != "18446744073709551615" || m.QuoteMetadata.ClientMessageID != "9007199254740993" || m.Conversation != r.Ref() || m.SenderID != "12" || m.Direction != "incoming" || m.Source != "" || m.FirstIncoming != nil || m.ReplyTo != nil || m.QuoteMetadata.Timestamp != fmt.Sprint(page.Candidates.Rows[0].Row.TimestampMS) || !m.SentAt.Equal(time.UnixMilli(page.Candidates.Rows[0].Row.TimestampMS)) {
		t.Fatal("conversion invented/lost metadata")
	}
	b, _ := json.Marshal(got)
	if string(b) != "{}" || fmt.Sprintf("%#v", got) != "converted mobile archive page [redacted]" {
		t.Fatal("private records serialized")
	}
	if page.Candidates.DeferredControls != 0 || page.Rejected != 1 || page.ExpiredMessages != 1 {
		t.Fatal("source gaps changed")
	}
}
func TestMobileMessageConversionExpiryContentAndMetadataGaps(t *testing.T) {
	// Arrange: nontext content and unresolved quote/mention metadata cannot be flattened.
	page, r, now := conversionPage(t)
	defer page.Clear()
	row := &page.Candidates.Rows[0]
	row.Kind = "chat.photo"
	row.Row.Type = 3
	page.Candidates.Rows[1].Metadata.Quote = &Quote{}
	page.Candidates.Rows[1].Metadata.Mentions = []Mention{{}}
	// Act.
	got, e := ConvertPreparedArchivePage(context.Background(), page, r, "10", now)
	defer got.Clear()
	// Assert: exact text of the supported row only, with gaps rather than fabricated attachment/reply.
	if e != nil || len(got.Records) != 1 || got.UnsupportedContent != 1 || got.UnresolvedQuotes != 1 || got.UnresolvedMentions != 1 || got.Records[0].Message.ReplyTo != nil {
		t.Fatal("content gaps lost", e)
	}
	row.Kind = "webchat"
	row.Row.Type = 0
	row.Metadata.UnsupportedTags = []uint32{6}
	got.Clear()
	got, e = ConvertPreparedArchivePage(context.Background(), page, r, "10", now)
	if e != nil || got.UnsupportedContent != 1 {
		t.Fatal("undecoded rich-text attachment accepted", e)
	}
	row.Metadata.UnsupportedTags = nil
	row.Row.TTL = 1
	row.ExpiryDeclared = true
	row.ExpiresMS = row.Row.TimestampMS + 1
	got.Clear()
	got, e = ConvertPreparedArchivePage(context.Background(), page, r, "10", now)
	if e != nil || got.Expired != 1 || len(got.Records) != 1 {
		t.Fatal("late expired candidate restored", e)
	}
}
func TestMobileMessageConversionRejectsBindingAndWholePageMutation(t *testing.T) {
	for _, mode := range []string{"request", "account", "direction", "expiry", "invalid-later-row", "controls", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			page, r, now := conversionPage(t)
			defer page.Clear()
			account := "10"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			switch mode {
			case "request":
				r.RequestID = "00000000-0000-4000-8000-000000000002"
			case "account":
				account = "11"
			case "direction":
				page.Candidates.Rows[0].Direction = "outgoing"
			case "expiry":
				page.Candidates.Rows[0].ExpiresMS = 1
			case "invalid-later-row":
				page.Candidates.Rows[1].Row.ClientID = "01"
			case "controls":
				page.SourceControls = 1
			case "cancel":
				cancel()
			}
			// Act / Assert: no valid first-record prefix escapes invalid second-record checks.
			got, e := ConvertPreparedArchivePage(ctx, page, r, account, now)
			if e == nil || len(got.Records) != 0 {
				t.Fatal("invalid conversion yielded prefix")
			}
		})
	}
}

func TestMobileInspectionValidatesBlockedSnapshotWithoutReleasingRecords(t *testing.T) {
	// Arrange: import is gated by both source properties, with one nontext row.
	page, request, now := conversionPage(t)
	defer page.Clear()
	page.SourceWAL, page.SourceControls = true, 1
	page.Candidates.Rows[0].Kind = "chat.photo"
	page.Candidates.Rows[0].Row.Type = 3
	page.Candidates.Rows[1].Metadata.Quote = &Quote{}
	page.Candidates.Rows[1].Metadata.Mentions = []Mention{{}}
	// Act: inspect privately and separately try the storage-record conversion.
	got, err := InspectPreparedArchivePage(context.Background(), page, request, "10", now)
	converted, convertErr := ConvertPreparedArchivePage(context.Background(), page, request, "10", now)
	defer converted.Clear()
	// Assert: content evidence survives the gates, which still prohibit records.
	if err != nil || got.TextCandidates != 1 || got.UnsupportedContent != 1 || got.UnresolvedQuotes != 1 || got.UnresolvedMentions != 1 || len(got.BlockReasons) != 2 || got.BlockReasons[0] != "unverified_wal_snapshot" || got.BlockReasons[1] != "unverified_source_controls" || convertErr == nil || len(converted.Records) != 0 {
		t.Fatal("blocked snapshot evidence or persistence boundary lost", err)
	}
	// Act: a malformed later row must not be hidden by the same WAL gate.
	page.Candidates.Rows[1].Row.ClientID = "01"
	got, err = InspectPreparedArchivePage(context.Background(), page, request, "10", now)
	// Assert: no prefix counts or misleading safety-only result escapes.
	if err == nil || got.TextCandidates != 0 || len(got.BlockReasons) != 0 {
		t.Fatal("source gate masked an invalid candidate")
	}
}

func TestMobileControlsOutsideIntervalPreventConversion(t *testing.T) {
	// Arrange: a deletion record precedes the requested interval, so paging sees no rows.
	r := selectedFetchRequest()
	r.MaxMessages = 5
	r.Since = "2026-09-16T00:00:00Z"
	archive := preparedArchiveFixture(t, r)
	defer archive.Clear()
	calls := 0
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		calls++
		return nil, nil
	})
	now := time.Date(2026, 9, 17, 0, 0, 0, 0, time.UTC).UnixMilli()
	page, e := ReadPreparedArchivePage(context.Background(), archive, r, "10", t.TempDir(), now, 2, nil, mapper)
	if e != nil {
		t.Fatal(e)
	}
	defer page.Clear()
	// Act.
	got, e := ConvertPreparedArchivePage(context.Background(), page, r, "10", now)
	// Assert: absence of in-window control rows is not treated as control compatibility.
	if page.Examined != 0 || page.SourceControls != 1 || calls != 0 || e == nil || len(got.Records) != 0 {
		t.Fatal("out-of-window deletion bypassed preflight")
	}
}

func TestWALSnapshotCannotBecomeImportRecords(t *testing.T) {
	// Arrange: main-file checkpoint completeness is not verified by the reader.
	page := PreparedArchivePage{SourceWAL: true}
	// Act.
	result, err := ConvertPreparedArchivePage(context.Background(), page, domain.MobileBackupRequest{}, "10", 1)
	// Assert: inspection alone must not authorize persistence of an unverified snapshot.
	if err == nil || len(result.Records) != 0 {
		t.Fatal("WAL snapshot became import records")
	}
}
