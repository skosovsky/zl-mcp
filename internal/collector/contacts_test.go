package collector

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type contactPages func(context.Context, int, int) ([]domain.Contact, error)

func (f contactPages) ContactsPage(ctx context.Context, page, limit int) ([]domain.Contact, error) {
	return f(ctx, page, limit)
}

func TestContactsRefreshBoundsAndPartialRecords(t *testing.T) {
	for _, test := range []struct {
		name           string
		mode           string
		pages          int
		status, reason string
		count          int
		wantErr        bool
	}{
		{"short", "short", 2, "exhausted", "short_page", 201, false},
		{"repeat", "repeat", 2, "partial", "repeated_page", 200, false},
		{"limit", "limit", 20, "partial", "page_limit", 4000, false},
		{"failure", "failure", 2, "partial", "upstream_unavailable", 200, true},
		{"unsupported", "unsupported", 1, "unsupported", "source_unsupported", 0, true},
		{"oversized", "oversized", 1, "partial", "oversized_page", 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			s, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			pages := 0
			source := contactPages(func(ctx context.Context, page, limit int) ([]domain.Contact, error) {
				pages++
				if limit != 200 {
					t.Fatal("unbounded page")
				}
				if test.mode == "unsupported" {
					return nil, errContactsUnsupported
				}
				if test.mode == "failure" && page == 2 {
					return nil, errors.New("private wire details must not enter status")
				}
				n := 200
				offset := (page - 1) * 200
				if test.mode == "short" && page == 2 {
					n = 1
				}
				if test.mode == "repeat" {
					offset = 0
				}
				if test.mode == "oversized" {
					n = 201
				}
				contacts := make([]domain.Contact, n)
				for i := range contacts {
					contacts[i] = domain.Contact{ID: fmt.Sprintf("peer-%d", offset+i), Name: "Synthetic", Friendship: "unknown"}
				}
				return contacts, nil
			})
			// Act
			err = refreshContacts(ctx, s, source)
			status, e := s.ContactStatus(ctx)
			if e != nil {
				t.Fatal(e)
			}
			// Assert
			if (err != nil) != test.wantErr || pages != test.pages || status["status"] != test.status || status["stop_reason"] != test.reason || status["observed_count"] != float64(test.count) {
				t.Fatalf("result: pages=%d err=%v status=%#v", pages, err, status)
			}
			var n int
			if e = s.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM directory_contacts").Scan(&n); e != nil {
				t.Fatal(e)
			}
			if n != test.count {
				t.Fatal("partial records lost")
			}
			if (status["last_success_at"] != nil) != (test.status == "exhausted") {
				t.Fatal("partial claimed success")
			}
		})
	}
}

func TestContactsRefreshCancellation(t *testing.T) {
	// Arrange
	ctx, cancel := context.WithCancel(context.Background())
	s, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	source := contactPages(func(ctx context.Context, page, limit int) ([]domain.Contact, error) {
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	})
	// Act
	err = refreshContacts(ctx, s, source)
	status, e := s.ContactStatus(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	// Assert
	if !errors.Is(err, context.Canceled) || status["status"] != "partial" || status["stop_reason"] != "cancelled_or_deadline" {
		t.Fatalf("cancel: %v %#v", err, status)
	}
}
