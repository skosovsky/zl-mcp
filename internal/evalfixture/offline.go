package evalfixture

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/messaging"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
	"github.com/skosovsky/zl-mcp/internal/service"
)

// Effect is recorded by the upstream independently of the MCP response.
// Payload is deliberately absent; this is synthetic dispatch accounting.
type Effect struct {
	Kind string `json:"kind"`
}
type offlineControl struct {
	fixture            *Fixture
	sender             *offlineSender
	library            *mobilebackup.RetainedArchiveStore
	archive            *service.ArchiveReader
	effects            string
	fixtureSendEnabled bool
	historyCancel      context.CancelFunc
	historyDone        chan error
}
type offlineSender struct {
	mu      sync.Mutex
	path    string
	timeout bool
}

func recordEffect(path, kind string) error {
	if path == "" {
		return fmt.Errorf("synthetic effect journal required")
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	if err = json.NewEncoder(file).Encode(Effect{Kind: kind}); err != nil {
		return err
	}
	return file.Sync()
}
func (s *offlineSender) SendDirect(_ context.Context, _ string, _ string, _ *domain.SendQuote) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := recordEffect(s.path, "send"); err != nil {
		return "", err
	}
	if s.timeout {
		return "", context.DeadlineExceeded
	}
	return "9007199254740993", nil
}
func newOffline(ctx context.Context, f *Fixture, opts Options) (*offlineControl, error) {
	c := &offlineControl{fixture: f, effects: opts.EffectsFile, sender: &offlineSender{path: opts.EffectsFile, timeout: opts.SendTimeout}}
	library, err := mobilebackup.NewArchiveLibraryStore(filepath.Join(f.Dir, "library"), 8<<20)
	if err != nil {
		return nil, err
	}
	c.library = library
	c.archive = service.NewArchiveReader(f.Store, library, f.Dir)
	f.Upstream.effectPath = opts.EffectsFile
	if opts.ArchiveVector != "" {
		if err = c.prepareArchive(ctx, opts.ArchiveVector); err != nil {
			library.Close()
			return nil, err
		}
	}
	if !opts.Empty {
		for _, kind := range []string{"direct", "group"} {
			if err := f.Store.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: kind, ID: "9007199254740993"}, ID: "9007199254740995", SenderID: "peer", Text: "precision unicode Cà phê 🍀 " + kind, SentAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), Source: "live"}); err != nil {
				library.Close()
				return nil, err
			}
		}
		if err := f.Store.Put(ctx, domain.Message{Conversation: domain.ConversationRef{Type: "direct", ID: "other-peer"}, ID: "foreign-quote", SenderID: "other-peer", Text: "foreign quote", SentAt: time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC), Source: "live", QuoteMetadata: &domain.QuoteMetadata{}}); err != nil {
			library.Close()
			return nil, err
		}
	}
	if err := f.Store.RecoverInterruptedSends(ctx); err != nil {
		library.Close()
		return nil, err
	}
	if err := f.Store.RecoverInterruptedHistory(ctx); err != nil {
		library.Close()
		return nil, err
	}
	c.fixtureSendEnabled = !opts.DenySend
	return c, nil
}
func (c *offlineControl) Call(ctx context.Context, method string, args any) (map[string]any, error) {
	switch method {
	case "zalo_send_direct_message", "zalo_get_send_status":
		return (&messaging.Manager{Store: c.fixture.Store, Sender: c.sender, Enabled: c.fixtureSendEnabled, Recipients: map[string]bool{"peer": true}}).Call(ctx, method, args)
	case "zalo_import_conversation_history", "zalo_get_history_import_status", "zalo_cancel_history_import":
		return (&historyimport.Manager{Store: c.fixture.Store}).Call(ctx, method, args)
	case "zalo_list_archive_sources", "zalo_list_conversations", "zalo_list_conversation_messages", "read_archive_resource":
		return c.archive.Call(ctx, method, args)
	default:
		return c.fixture.joins.Call(ctx, method, args)
	}
}
func (c *offlineControl) close() error {
	var workerErr error
	if c.historyCancel != nil {
		c.historyCancel()
		workerErr = <-c.historyDone
		if errors.Is(workerErr, context.Canceled) {
			workerErr = nil
		}
	}
	return errors.Join(workerErr, c.library.Close())
}

// ApproveSyntheticJoin represents prior trusted host approval, never an MCP tool.
func (f *Fixture) ApproveSyntheticJoin(ctx context.Context) (string, error) {
	preview, err := f.joins.Inspect(ctx, InviteURL)
	if err != nil {
		return "", err
	}
	return f.joins.Approve(ctx, *preview["preview_id"].(*string))
}

type archiveSource struct{ data []byte }

func (s *archiveSource) ConsumeMobileArchive(ctx context.Context, _ *http.Request, _ *http.Client, consume func(context.Context, *http.Response) ([]byte, error)) ([]byte, error) {
	return consume(ctx, &http.Response{StatusCode: 200, ContentLength: int64(len(s.data)), Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(s.data))})
}
func (*archiveSource) MapMobileBackupIdentities(_ context.Context, r domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
	var pairs []domain.MobileIdentityPair
	for _, id := range r.Direct {
		target := "12"
		if id == "903" {
			target = "13"
		}
		pairs = append(pairs, domain.MobileIdentityPair{Plain: id, Session: target})
	}
	for _, id := range r.Groups {
		pairs = append(pairs, domain.MobileIdentityPair{Plain: id, Session: "12", Group: true})
	}
	return pairs, nil
}
func (c *offlineControl) prepareArchive(ctx context.Context, path string) error {
	binding, err := c.fixture.Store.ArchiveAccountKey(ctx)
	if err != nil {
		return err
	}
	sources, err := c.library.Sources(ctx, binding)
	if err != nil {
		return err
	}
	if len(sources) > 0 {
		return nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var vector struct{ Ciphertext string }
	if err = json.Unmarshal(raw, &vector); err != nil {
		return err
	}
	data, err := hex.DecodeString(vector.Ciphertext)
	if err != nil {
		return err
	}
	source := &archiveSource{data: data}
	d, err := mobilebackup.NewDownloader([]string{"archive.example.com"})
	if err != nil {
		return err
	}
	d, err = d.WithSession(source)
	if err != nil {
		return err
	}
	a, err := mobilebackup.FetchAccountArchive(ctx, d, domain.MobileBackupOffer{URL: "https://archive.example.com/synthetic", KeyText: strings.Repeat("0123456789abcdef", 4), FileSize: uint64(len(data))}, 8<<20, source)
	if err != nil {
		return err
	}
	defer a.Clear()
	capture, err := mobilebackup.NewRetainedArchiveStore(filepath.Join(c.fixture.Dir, "capture"), 8<<20)
	if err != nil {
		return err
	}
	defer capture.Close()
	const id = "00000000-0000-4000-8000-000000000001"
	if _, err = capture.Save(ctx, id, "10", a, time.Hour); err != nil {
		return err
	}
	_, err = c.library.PreserveFrom(ctx, capture, id, binding)
	return err
}
