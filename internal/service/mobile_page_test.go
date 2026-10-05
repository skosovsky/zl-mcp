package service

import (
	"bytes"
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type mobilePageSource struct {
	mobileServiceSource
	data                []byte
	downloads, mappings atomic.Int32
	cancelRead          context.CancelFunc
}

func (*mobilePageSource) AccountID() string { return "10" }
func (s *mobilePageSource) ReceiveMobileBackupOffer(ctx context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
	if _, e := s.mobileServiceSource.ReceiveMobileBackupOffer(ctx, o); e != nil {
		return domain.MobileBackupOffer{}, e
	}
	return domain.MobileBackupOffer{URL: "https://archive.example.com/private", KeyText: strings.Repeat("0123456789abcdef", 4), FileSize: uint64(len(s.data))}, nil
}
func (s *mobilePageSource) ConsumeMobileArchive(ctx context.Context, req *http.Request, client *http.Client, consume func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	s.downloads.Add(1)
	var body io.Reader = bytes.NewReader(s.data)
	if s.cancelRead != nil {
		body = &cancelArchiveReader{Reader: bytes.NewReader(s.data), cancel: s.cancelRead}
	}
	return consume(ctx, &http.Response{StatusCode: 200, ContentLength: int64(len(s.data)), Header: make(http.Header), Body: io.NopCloser(body)})
}
func (s *mobilePageSource) MapMobileBackupIdentities(_ context.Context, r domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
	s.mappings.Add(1)
	pairs := make([]domain.MobileIdentityPair, 0, len(r.Direct))
	for _, id := range r.Direct {
		pairs = append(pairs, domain.MobileIdentityPair{Plain: id, Session: "12"})
	}
	return pairs, nil
}

type cancelArchiveReader struct {
	*bytes.Reader
	cancel context.CancelFunc
}

func (r *cancelArchiveReader) Read(p []byte) (int, error) {
	if len(p) > 16 {
		p = p[:16]
	}
	n, e := r.Reader.Read(p)
	if r.cancel != nil {
		r.cancel()
		r.cancel = nil
	}
	return n, e
}
func TestServiceMobilePageUsesCollectorSessionAndExpiresHistoricalRows(t *testing.T) {
	for _, name := range []string{"complete", "cancel-during-body", "walk"} {
		cancelBody := name == "cancel-during-body"
		t.Run(name, func(t *testing.T) {
			// Arrange: independently encrypted SQLite fixture and production collector guard.
			raw, e := os.ReadFile("../mobilebackup/testdata/format1-sqlite-vector.json")
			if e != nil {
				t.Fatal(e)
			}
			var v struct{ Ciphertext string }
			if e = json.Unmarshal(raw, &v); e != nil {
				t.Fatal(e)
			}
			data, e := hex.DecodeString(v.Ciphertext)
			if e != nil {
				t.Fatal(e)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			store, e := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
			if e != nil {
				t.Fatal(e)
			}
			defer store.Close()
			n := new(big.Int).Lsh(big.NewInt(1), 2047)
			n.Add(n, big.NewInt(1))
			der, _ := x509.MarshalPKIXPublicKey(&rsa.PublicKey{N: n, E: 65537})
			source := &mobilePageSource{mobileServiceSource: mobileServiceSource{historyServiceSource: historyServiceSource{started: make(chan struct{})}, public: base64.StdEncoding.EncodeToString(der)}, data: data}
			port := &membershipPort{store: store, lifecycle: ctx}
			var c config.Config
			c.Collection.Mode = "all"
			var restores atomic.Int32
			done := make(chan error, 1)
			go func() {
				done <- collectWithHeartbeat(ctx, c, store, port, func(context.Context, string) (collector.ListenerUpstream, error) { restores.Add(1); return source, nil }, func(collector.ListenerUpstream) error { return nil }, time.Hour)
			}()
			defer func() {
				cancel()
				if e := <-done; e != nil {
					t.Error(e)
				}
			}()
			select {
			case <-source.started:
			case <-time.After(5 * time.Second):
				t.Fatal("collector not ready")
			}
			request := domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "12", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"}
			attempt, e := store.PrepareMobileBackup(ctx, request)
			if e != nil {
				t.Fatal(e)
			}
			downloader, e := mobilebackup.NewDownloader([]string{"archive.example.com"})
			if e != nil {
				t.Fatal(e)
			}
			scratch := t.TempDir()
			for _, bad := range []struct {
				scratch string
				now     int64
				size    int
			}{{"relative", 1, 1}, {scratch, 0, 1}, {scratch, 1, 51}} {
				if page, e := port.prepareFirstMobilePage(ctx, attempt.OperationID, downloader, bad.scratch, bad.now, bad.size); e == nil || page.Examined != 0 || source.offers.Load() != 0 {
					t.Fatal("invalid local arguments dispatched phone request")
				}
			}
			operation, cancelOperation := context.WithCancel(ctx)
			defer cancelOperation()
			if cancelBody {
				source.cancelRead = cancelOperation
			}
			// Act: offer, scoped download, decryption, mapping, SQLite and expiry in one service call.
			var page mobilebackup.PreparedArchivePage
			mappingsWanted := int32(2)
			if name == "walk" {
				mappingsWanted = 3
				var summary mobilebackup.ArchiveWalkSummary
				summary, e = port.walkPreparedMobilePages(operation, attempt.OperationID, downloader, scratch, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC).UnixMilli(), 1, 100, func(active context.Context, borrowed mobilebackup.PreparedArchivePage) error {
					converted, err := mobilebackup.ConvertPreparedArchivePage(active, borrowed, request, "10", time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC).UnixMilli())
					if err != nil {
						return err
					}
					defer converted.Clear()
					// Synthetic fixture only: prove the silent storage port, not public import acceptance.
					if _, err = store.PutExpiringHistoryPage(active, request.Ref(), converted.Records); err != nil {
						return err
					}
					for _, row := range borrowed.Candidates.Rows {
						// Only scalar evidence is copied out of the borrowed page.
						row.Row.BinNet = nil
						row.Metadata = mobilebackup.BinNet{}
						page.Candidates.Rows = append(page.Candidates.Rows, row)
					}
					return nil
				})
				page.Examined = summary.Examined
				page.ExpiredMessages = summary.ExpiredMessages
				if e == nil && (summary.Pages != 2 || summary.SourceHasMore || summary.StopReason != "available_archive_exhausted") {
					t.Fatal("service traversal lost continuation")
				}
			} else {
				page, e = port.prepareFirstMobilePage(operation, attempt.OperationID, downloader, scratch, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC).UnixMilli(), 2)
			}
			defer page.Clear()
			// Assert: exactly one session, no retained expired row or private scratch file.
			if cancelBody {
				if e == nil || page.Examined != 0 || len(page.Candidates.Rows) != 0 || source.downloads.Load() != 1 || source.mappings.Load() != 0 {
					t.Fatal("cancelled body retained result or reached mapping")
				}
				return
			}
			if e != nil || page.Examined != 2 || page.ExpiredMessages != 1 || len(page.Candidates.Rows) != 1 {
				t.Fatal("archive page mismatch", e, "offers", source.offers.Load(), "downloads", source.downloads.Load(), "mappings", source.mappings.Load())
			}
			row := page.Candidates.Rows[0]
			if row.Row.MessageID != "1001" || row.SenderID != "12" || row.Direction != "incoming" || restores.Load() != 1 || source.offers.Load() != 1 || source.downloads.Load() != 1 || source.mappings.Load() != mappingsWanted {
				t.Fatal("session/page ownership mismatch")
			}
			files, e := os.ReadDir(scratch)
			if e != nil || len(files) != 0 {
				t.Fatal("scratch retained")
			}
			for _, table := range []string{"messages", "message_events", "send_operations", "event_subscriptions"} {
				var count int
				if e := store.DB.QueryRowContext(ctx, "SELECT count(*) FROM "+table).Scan(&count); e != nil {
					t.Fatal(e)
				}
				want := 0
				if name == "walk" && table == "messages" {
					want = 1
				}
				if count != want {
					t.Fatal("synthetic import state mismatch", table, count, want)
				}
			}
		})
	}
}
