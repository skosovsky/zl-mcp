package mobilebackup

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func preparedArchiveFixture(t *testing.T, r domain.MobileBackupRequest) SelectedArchive {
	t.Helper()
	r, err := r.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Date(2026, 9, 15, 0, 0, 0, 0, time.UTC).UnixMilli()
	data := sqliteFixture(t, backupSQLiteSchema, func(db *sql.DB) {
		for i := 1; i <= 5; i++ {
			sender := "901"
			ttl := int64(0)
			kind := int64(0)
			if i == 1 {
				ttl = 1
			}
			if i == 2 {
				sender = "01"
			}
			if i == 3 {
				kind = 33
			}
			metadata := []byte{0, 0, 0, 9, 0, 0, 0, 1, 'x'}
			if _, e := db.Exec("INSERT INTO ChatContent VALUES(?,?,?,?,?,?,?,?,?)", sender, fmt.Sprint(i), fmt.Sprint(i), "synthetic", ts, ttl, kind, 1, metadata); e != nil {
				t.Fatal(e)
			}
		}
	})
	return SelectedArchive{File: ArchiveFile{Name: "902.db", Data: data}, ref: r.Ref(), requestID: r.RequestID, requestFingerprint: r.Fingerprint()}
}
func TestPreparedArchivePagesAdvanceAcrossRejectedExpiredAndControlRows(t *testing.T) {
	// Arrange: first page retains nothing; all rows share one timestamp.
	r := selectedFetchRequest()
	r.MaxMessages = 5
	archive := preparedArchiveFixture(t, r)
	defer archive.Clear()
	original := append([]byte(nil), archive.File.Data...)
	calls := 0
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		calls++
		return []domain.MobileIdentityPair{{Plain: "901", Session: "12"}}, nil
	})
	scratch := t.TempDir()
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC).UnixMilli()
	var cursor *PreparedArchiveCursor
	examined, rejected, expired, controls := 0, 0, 0, 0
	var ids []string
	// Act: three pages, independent of retained rows.
	for pageNo := 0; pageNo < 3; pageNo++ {
		page, e := ReadPreparedArchivePage(context.Background(), archive, r, "10", scratch, now, 2, cursor, mapper)
		if e != nil {
			t.Fatal(e)
		}
		examined += page.Examined
		rejected += page.Rejected
		expired += page.ExpiredMessages
		controls += page.Candidates.DeferredControls
		for _, row := range page.Candidates.Rows {
			ids = append(ids, row.Row.MessageID)
		}
		if pageNo < 2 {
			if page.Next == nil || !page.SourceHasMore || page.BudgetExhausted {
				t.Fatal("continuation lost")
			}
			next := *page.Next
			cursor = &next
		} else if page.Next != nil || page.SourceHasMore {
			t.Fatal("terminal page continued")
		}
		encoded, _ := json.Marshal(page)
		if string(encoded) != "{}" || fmt.Sprintf("%#v", page) != "mobile backup prepared archive page [redacted]" {
			t.Fatal("private page exposed")
		}
		page.Clear()
	}
	// Assert: source bytes unchanged; source gaps and expired rows still count.
	if examined != 5 || rejected != 1 || expired != 1 || controls != 1 || fmt.Sprint(ids) != "[4 5]" || calls != 3 || !bytes.Equal(original, archive.File.Data) {
		t.Fatal("page traversal mismatch")
	}
	files, e := os.ReadDir(scratch)
	if e != nil || len(files) != 0 {
		t.Fatal("private scratch retained")
	}
}
func TestPreparedArchivePageBudgetAndBinding(t *testing.T) {
	// Arrange.
	r := selectedFetchRequest()
	r.MaxMessages = 2
	archive := preparedArchiveFixture(t, r)
	defer archive.Clear()
	calls := 0
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		calls++
		return []domain.MobileIdentityPair{{Plain: "901", Session: "12"}}, nil
	})
	scratch := t.TempDir()
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC).UnixMilli()
	// Act: page size exceeds remaining budget but not per-page limit.
	page, e := ReadPreparedArchivePage(context.Background(), archive, r, "10", scratch, now, 50, nil, mapper)
	// Assert: budget includes expired/rejected rows, source remains incomplete.
	if e != nil || page.Examined != 2 || !page.SourceHasMore || !page.BudgetExhausted || page.Next != nil || len(page.Candidates.Rows) != 0 {
		t.Fatal("budget bypass", e)
	}
	page.Clear()
	changed := r
	changed.ConversationID = "13"
	// Act / Assert: selection mismatch cannot reach source mapping.
	before := calls
	if _, e = ReadPreparedArchivePage(context.Background(), archive, changed, "10", scratch, now, 1, nil, mapper); !errors.Is(e, ErrSQLite) || calls != before {
		t.Fatal("changed request accepted")
	}
}
func TestPreparedArchiveCursorRejectsAccountRequestAndChangedBytes(t *testing.T) {
	// Arrange.
	r := selectedFetchRequest()
	r.MaxMessages = 5
	archive := preparedArchiveFixture(t, r)
	defer archive.Clear()
	calls := 0
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		calls++
		return []domain.MobileIdentityPair{{Plain: "901", Session: "12"}}, nil
	})
	scratch := t.TempDir()
	now := time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC).UnixMilli()
	page, e := ReadPreparedArchivePage(context.Background(), archive, r, "10", scratch, now, 1, nil, mapper)
	if e != nil || page.Next == nil {
		t.Fatal("missing cursor", e)
	}
	next := *page.Next
	page.Clear()
	for _, mode := range []string{"account", "request", "bytes", "cancel", "mapper-error"} {
		t.Run(mode, func(t *testing.T) {
			request := r
			selected := archive
			account := "10"
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			source := mapper
			switch mode {
			case "account":
				account = "11"
			case "request":
				request.RequestID = "00000000-0000-4000-8000-000000000002"
			case "bytes":
				selected.File.Data = append([]byte(nil), archive.File.Data...)
				selected.File.Data[len(selected.File.Data)-1] ^= 1
			case "cancel":
				cancel()
			case "mapper-error":
				source = identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
					return nil, errors.New("private marker")
				})
			}
			before := calls
			// Act: use page size two so valid rows reach mapper in mapper-error case.
			got, e := ReadPreparedArchivePage(ctx, selected, request, account, scratch, now, 3, &next, source)
			// Assert: failed pages have no continuation/candidates or successful prefix.
			if !errors.Is(e, ErrSQLite) || got.Next != nil || got.Candidates.Rows != nil || got.Examined != 0 || calls != before {
				t.Fatal("invalid continuation accepted or prefix retained")
			}
		})
	}
}
