package mobilebackup

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestArchiveWalkCountsGapsAndProtectsCursorFromVisitor(t *testing.T) {
	// Arrange: tied timestamps, expired/rejected/control rows and two usable rows.
	r := selectedFetchRequest()
	r.MaxMessages = 5
	archive := preparedArchiveFixture(t, r)
	defer archive.Clear()
	mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		return []domain.MobileIdentityPair{{Plain: "901", Session: "12"}}, nil
	})
	var ids []string
	var borrowed []byte
	// Act: visitor deliberately modifies its lent cursor, never the walker's copy.
	got, e := WalkPreparedArchive(context.Background(), archive, r, "10", t.TempDir(), time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC).UnixMilli(), 2, 100, mapper, func(_ context.Context, p PreparedArchivePage) error {
		if p.Next != nil {
			*p.Next = PreparedArchiveCursor{}
		}
		for _, row := range p.Candidates.Rows {
			ids = append(ids, row.Row.MessageID)
			borrowed = row.Row.BinNet
		}
		return nil
	})
	// Assert: all source rows counted, empty candidate page progressed and buffers cleared.
	if e != nil || got.Pages != 3 || got.Examined != 5 || got.Rejected != 1 || got.ExpiredMessages != 1 || got.DeferredControls != 1 || got.Candidates != 2 || got.SourceHasMore || got.StopReason != "available_archive_exhausted" || len(ids) != 2 || ids[0] != "4" || ids[1] != "5" {
		t.Fatal("walk coverage mismatch", e, got)
	}
	for _, b := range borrowed {
		if b != 0 {
			t.Fatal("lent bytes retained")
		}
	}
}
func TestArchiveWalkLimitsAndFailure(t *testing.T) {
	for _, mode := range []string{"page", "message", "visitor", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			r := selectedFetchRequest()
			r.MaxMessages = 5
			if mode == "message" {
				r.MaxMessages = 2
			}
			archive := preparedArchiveFixture(t, r)
			defer archive.Clear()
			mapper := identitySourceFunc(func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
				return []domain.MobileIdentityPair{{Plain: "901", Session: "12"}}, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			pages := 100
			if mode == "page" {
				pages = 1
			}
			calls := 0
			// Act.
			got, e := WalkPreparedArchive(ctx, archive, r, "10", t.TempDir(), time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC).UnixMilli(), 2, pages, mapper, func(context.Context, PreparedArchivePage) error {
				calls++
				if mode == "visitor" {
					return errors.New("synthetic private error")
				}
				if mode == "cancel" {
					cancel()
				}
				return nil
			})
			// Assert: no false exhaustion or summary prefix after callback failure.
			switch mode {
			case "page", "message":
				reason := "page_limit"
				if mode == "message" {
					reason = "message_limit"
				}
				if e != nil || !got.SourceHasMore || got.StopReason != reason || got.Pages != 1 || calls != 1 {
					t.Fatal("limit lost", e, got)
				}
			default:
				if !errors.Is(e, ErrSQLite) || got != (ArchiveWalkSummary{}) || calls != 1 {
					t.Fatal("failed walk leaked summary", e, got)
				}
			}
		})
	}
}
