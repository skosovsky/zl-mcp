// Package evalfixture provides an offline MCP environment for model evaluations.
// It has no Zalo credentials and its upstream cannot perform network requests.
package evalfixture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

const InviteURL = "https://zalo.me/g/evalapproved"

type Options struct {
	Fixtures    string
	SkillDir    string
	Empty       bool
	Stopped     bool
	JoinTimeout bool
	// ApprovalFile simulates a prior trusted human approval in the synthetic
	// environment. No real invite is accepted; this is never used by zl-mcp.
	ApprovalFile string
	// ApprovalTokenFile is a synthetic harness-issued token known before client
	// startup. It never grants permissions for a live Zalo account.
	ApprovalTokenFile string
}

type Fixture struct {
	Dir           string
	Store         *storage.Store
	Server        *mcp.Server
	Upstream      *Upstream
	joins         *collector.JoinManager
	http          *http.Server
	cancel        context.CancelFunc
	done          chan error
	heartbeatDone chan struct{}
}

type Upstream struct {
	mu      sync.Mutex
	groups  []domain.Group
	joined  bool
	calls   int
	timeout bool
}

func (*Upstream) AccountID() string { return "synthetic-eval-account" }
func (u *Upstream) Groups(context.Context) ([]domain.Group, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	result := append([]domain.Group(nil), u.groups...)
	if u.joined {
		result = append(result, domain.Group{ID: "g-join", Name: "Synthetic join target"})
	}
	return result, nil
}
func (u *Upstream) Group(ctx context.Context, id string) (domain.Group, *string, error) {
	groups, _ := u.Groups(ctx)
	for _, g := range groups {
		if g.ID == id {
			return g, nil, nil
		}
	}
	return domain.Group{}, nil, domain.Invalid("Unknown synthetic group ID.")
}
func (*Upstream) Inspect(_ context.Context, link string) (domain.Invite, error) {
	if link != InviteURL {
		return domain.Invite{}, domain.Invalid("Only the fixed synthetic invitation is supported.")
	}
	return domain.Invite{Group: domain.Group{ID: "g-join", Name: "Synthetic join target"}}, nil
}
func (u *Upstream) Join(_ context.Context, link string) error {
	if link != InviteURL {
		return domain.Invalid("Only the fixed synthetic invitation is supported.")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls++
	// A timeout deliberately remains unknown: absence of confirmation must not
	// make the model submit another request.
	if u.timeout {
		return context.DeadlineExceeded
	}
	u.joined = true
	return nil
}
func (u *Upstream) JoinCalls() int { u.mu.Lock(); defer u.mu.Unlock(); return u.calls }

func New(ctx context.Context, opts Options) (_ *Fixture, err error) {
	data, err := os.ReadFile(opts.Fixtures)
	if err != nil {
		return nil, err
	}
	var corpus struct {
		Groups   []domain.Group   `json:"groups"`
		Messages []domain.Message `json:"messages"`
	}
	if err = json.Unmarshal(data, &corpus); err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "zl-eval-")
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	f := &Fixture{Dir: dir, cancel: cancel, done: make(chan error, 1)}
	defer func() {
		if err != nil {
			f.Close()
		}
	}()
	allowed := []string{"g-join"}
	for _, group := range corpus.Groups {
		allowed = append(allowed, group.ID)
	}
	f.Store, err = storage.Open(runCtx, filepath.Join(dir, "messages.sqlite"), allowed, 90)
	if err != nil {
		return nil, err
	}
	if err = f.Store.BindAccount(runCtx, "synthetic-eval-account"); err != nil {
		return nil, err
	}
	if err = f.Store.ReplaceCatalog(runCtx, corpus.Groups); err != nil {
		return nil, err
	}
	if !opts.Empty {
		for _, message := range corpus.Messages {
			message.Source = "live"
			if err = f.Store.Put(runCtx, message); err != nil {
				return nil, err
			}
		}
	}
	if err = f.Store.BeginGap(runCtx, "synthetic_missing_history"); err != nil {
		return nil, err
	}
	state := map[string]any{"authenticated": true, "collector_state": "connected", "last_connected_at": time.Now().UTC().Format(time.RFC3339Nano), "last_event_at": nil, "last_persisted_at": nil, "last_error": nil}
	if opts.Stopped {
		state["collector_state"] = "stopped"
	}
	if err = f.Store.SetState(runCtx, state); err != nil {
		return nil, err
	}
	f.Upstream = &Upstream{groups: corpus.Groups, timeout: opts.JoinTimeout}
	f.joins = collector.NewJoin(runCtx, f.Store, f.Upstream, true)
	if opts.ApprovalFile != "" && opts.ApprovalTokenFile != "" {
		return nil, fmt.Errorf("use only one synthetic approval mode")
	}
	if opts.ApprovalFile != "" || opts.ApprovalTokenFile != "" {
		preview, e := f.joins.Inspect(runCtx, InviteURL)
		if e != nil {
			return nil, e
		}
		token, e := f.joins.Approve(runCtx, *preview["preview_id"].(*string))
		if e != nil {
			return nil, e
		}
		if opts.ApprovalTokenFile != "" {
			provided, readErr := os.ReadFile(opts.ApprovalTokenFile)
			if readErr != nil {
				return nil, readErr
			}
			seed := strings.TrimSpace(string(provided))
			if _, decodeErr := hex.DecodeString(seed); decodeErr != nil || len(seed) != 64 {
				return nil, fmt.Errorf("synthetic token must be 64 hex characters")
			}
			oldHash, newHash := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(seed))
			// Replace only this fixture's fresh approval hash. This keeps the
			// real approval/preview ledger and lets harness supply the token
			// as trusted user input before Codex starts its MCP subprocess.
			if _, e = f.Store.DB.ExecContext(runCtx, "UPDATE join_approvals SET token_hash=? WHERE token_hash=?", hex.EncodeToString(newHash[:]), hex.EncodeToString(oldHash[:])); e != nil {
				return nil, e
			}
		}
		if opts.ApprovalFile != "" {
			approval, e := json.Marshal(map[string]any{"synthetic_only": true, "invite_url": InviteURL, "preview": preview, "plan_token": token})
			if e != nil {
				return nil, e
			}
			// Never overwrite an existing file (including a symlink).
			file, e := os.OpenFile(opts.ApprovalFile, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				return nil, e
			}
			_, writeErr := file.Write(approval)
			closeErr := file.Close()
			if writeErr != nil {
				return nil, writeErr
			}
			if closeErr != nil {
				return nil, closeErr
			}
		}
	}
	sock, err := local.SocketPath(dir)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("unix", sock)
	if err != nil {
		return nil, err
	}
	if err = os.Chmod(sock, 0600); err != nil {
		listener.Close()
		return nil, err
	}
	f.http = &http.Server{Handler: collector.ControlHandler(f.joins), ReadHeaderTimeout: 5 * time.Second}
	go func() { f.done <- f.http.Serve(listener) }()
	f.Server, err = mcpserver.New(f.Store, dir)
	if err != nil {
		return nil, err
	}
	// Expose only the three fixed skill documents so evaluations can record
	// actual reference reads without giving the model filesystem access.
	if opts.SkillDir != "" {
		for _, name := range []string{"SKILL.md", "reference/report-format.md", "reference/eval-cases.md"} {
			contents, readErr := os.ReadFile(filepath.Join(opts.SkillDir, name))
			if readErr != nil {
				return nil, readErr
			}
			uri := "eval://skills/researching-zalo-groups/" + name
			body := string(contents)
			f.Server.AddResource(&mcp.Resource{URI: uri, Name: name, MIMEType: "text/markdown"}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
				return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "text/markdown", Text: body}}}, nil
			})
		}
	}
	// Keep synthetic connected status current throughout long model turns.
	f.heartbeatDone = make(chan struct{})
	go func() {
		defer close(f.heartbeatDone)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-runCtx.Done():
				return
			case <-ticker.C:
				_ = f.Store.SetState(runCtx, state)
			}
		}
	}()
	return f, nil
}

func (f *Fixture) Close() error {
	f.cancel()
	if f.heartbeatDone != nil {
		<-f.heartbeatDone
	}
	if f.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := f.http.Shutdown(ctx)
		cancel()
		if err != nil {
			_ = f.http.Close()
		}
		<-f.done
	}
	if f.joins != nil {
		f.joins.Wait()
	}
	if f.Store != nil {
		_ = f.Store.Close()
	}
	if err := os.RemoveAll(f.Dir); err != nil {
		return fmt.Errorf("remove synthetic state: %w", err)
	}
	return nil
}
