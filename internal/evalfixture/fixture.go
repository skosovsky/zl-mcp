// Package evalfixture provides an offline MCP environment for model evaluations.
// It has no Zalo credentials and its upstream cannot perform network requests.
package evalfixture

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
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
	"github.com/skosovsky/zl-mcp/internal/control"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

const InviteURL = "https://zalo.me/g/evalapproved"

type Options struct {
	Policy *domain.CollectionPolicy

	// Offline enables extended synthetic scenarios through existing domain owners.
	Offline       bool
	StateDir      string
	EffectsFile   string
	SendTimeout   bool
	DenySend      bool
	KeepState     bool
	ArchiveVector string

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
	offline       *offlineControl
	keepState     bool
}

type Upstream struct {
	mu         sync.Mutex
	groups     []domain.Group
	joined     bool
	calls      int
	timeout    bool
	effectPath string
	accountID  string
}

func (u *Upstream) AccountID() string {
	if u.accountID != "" {
		return u.accountID
	}
	return "synthetic-eval-account"
}
func (u *Upstream) Groups(context.Context) ([]domain.Group, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	result := append([]domain.Group(nil), u.groups...)
	if u.joined {
		//nolint:exhaustruct_v5 // Group preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
		result = append(result, domain.Group{ID: fixtureGJoinID, Name: "Synthetic join target"})
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
	//nolint:exhaustruct_v5 // Invite preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection. Group preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
	return domain.Invite{Group: domain.Group{ID: fixtureGJoinID, Name: "Synthetic join target"}}, nil
}
func (u *Upstream) Join(_ context.Context, link string) error {
	if link != InviteURL {
		return domain.Invalid("Only the fixed synthetic invitation is supported.")
	}
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.effectPath != "" {
		if err := recordEffect(u.effectPath, "join"); err != nil {
			return err
		}
	}
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
	var corpus fixtureCorpus
	if err = json.Unmarshal(data, &corpus); err != nil {
		return nil, err
	}
	dir := opts.StateDir
	if dir == "" {
		dir, err = os.MkdirTemp("", "zl-eval-")
	} else {
		err = os.MkdirAll(dir, 0700)
	}
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	//nolint:exhaustruct_v5 // Fixture fills only this synthetic run/result phase; remaining fields keep their empty/default values.
	f := &Fixture{Dir: dir, cancel: cancel, done: make(chan error, 1), keepState: opts.KeepState}
	defer func() {
		if err != nil {
			//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
			f.Close()
		}
	}()
	f.Store, err = openFixtureStore(runCtx, dir, opts, corpus.Groups)
	if err != nil {
		return nil, err
	}
	account := "synthetic-eval-account"
	if opts.Offline {
		account = "10"
	}
	state, seedErr := seedFixtureStore(runCtx, f.Store, account, corpus, opts)
	if seedErr != nil {
		return nil, seedErr
	}
	//nolint:exhaustruct_v5 // Upstream fills only this synthetic run/result phase; remaining fields keep their empty/default values.
	f.Upstream = &Upstream{groups: corpus.Groups, timeout: opts.JoinTimeout, accountID: account}
	f.joins = collector.NewJoin(runCtx, f.Store, f.Upstream, true)
	if err = f.prepareApproval(runCtx, opts); err != nil {
		return nil, err
	}
	if err = f.startControl(runCtx); err != nil {
		return nil, err
	}
	if opts.Offline {
		f.offline, err = newOffline(runCtx, f, opts)
		if err == nil {
			f.Server, err = mcpserver.New(fixtureReadPorts(f.Store, f.offline))
		}
	} else {
		backend := control.New(dir)
		f.Server, err = mcpserver.New(fixtureReadPorts(f.Store, backend))
	}
	if err != nil {
		return nil, err
	}
	if err = f.addSkillResources(opts.SkillDir); err != nil {
		return nil, err
	}
	// Keep synthetic connected status current throughout long model turns.
	f.heartbeatDone = make(chan struct{})
	go f.keepConnected(runCtx, state)
	return f, nil
}

func (f *Fixture) Close() error {
	var result error
	f.cancel()
	if f.heartbeatDone != nil {
		<-f.heartbeatDone
	}
	if f.http != nil {
		ctx, cancel := context.WithTimeout(context.Background(), fixtureCloseTimeout)
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
	if f.offline != nil {
		result = errors.Join(result, f.offline.close())
	}
	if f.Store != nil {
		result = errors.Join(result, f.Store.Close())
	}

	if f.keepState {
		return result
	}
	if err := os.RemoveAll(f.Dir); err != nil {
		return errors.Join(result, fmt.Errorf("remove synthetic state: %w", err))
	}
	return result
}

func (f *Fixture) prepareApproval(runCtx context.Context, opts Options) error {
	if opts.ApprovalFile != "" && opts.ApprovalTokenFile != "" {
		return errors.New("use only one synthetic approval mode")
	}
	if opts.ApprovalFile == "" && opts.ApprovalTokenFile == "" {
		return nil
	}
	preview, e := f.joins.Inspect(runCtx, InviteURL)
	if e != nil {
		return e
	}
	previewID, ok := preview["preview_id"].(*string)
	if !ok || previewID == nil {
		return errors.New("invalid synthetic approval preview")
	}
	token, e := f.joins.Approve(runCtx, *previewID)
	if e != nil {
		return e
	}
	if opts.ApprovalTokenFile != "" {
		if seedErr := f.seedApprovalToken(runCtx, opts.ApprovalTokenFile, token); seedErr != nil {
			return seedErr
		}
	}
	if opts.ApprovalFile != "" {
		return writeSyntheticApproval(opts.ApprovalFile, preview, token)
	}
	return nil
}

func (f *Fixture) addSkillResources(skillDir string) error {
	// Expose the fixed research skill package, including conditional references,
	// so the harness records reads without providing filesystem access.
	if skillDir != "" {
		for _, name := range []string{
			"SKILL.md",
			"reference/report-format.md",
			"reference/eval-cases.md",
			"reference/service-access.md",
			"reference/corpus-reading.md",
			"reference/archive-reading.md",
			"reference/history-import.md",
			"reference/message-actions.md",
		} {
			contents, readErr := os.ReadFile(filepath.Join(skillDir, name))
			if readErr != nil {
				return readErr
			}
			uri := "eval://skills/researching-zalo-groups/" + name
			body := string(contents)
			f.Server.AddResource(
				//nolint:exhaustruct_v5 // Resource keeps unused optional SDK metadata/representations absent; preserve existing wire defaults.
				&mcp.Resource{URI: uri, Name: name, MIMEType: "text/markdown"},
				func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
					//nolint:exhaustruct_v5 // ReadResourceResult keeps unused optional SDK metadata/representations absent; preserve existing wire defaults.
					return &mcp.ReadResourceResult{
						//nolint:exhaustruct_v5 // Only the selected text/blob resource representation is populated; the other remains absent.
						Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: "text/markdown", Text: body}},
					}, nil
				},
			)
		}
	}
	return nil
}

func (f *Fixture) keepConnected(ctx context.Context, state map[string]any) {
	defer close(f.heartbeatDone)
	ticker := time.NewTicker(fixtureHeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = f.Store.SetState(ctx, state)
		}
	}
}

func (f *Fixture) seedApprovalToken(runCtx context.Context, path, token string) error {
	provided, readErr := os.ReadFile(path)
	if readErr != nil {
		return readErr
	}
	seed := strings.TrimSpace(string(provided))
	if _, decodeErr := hex.DecodeString(seed); decodeErr != nil || len(seed) != 64 {
		return errors.New("synthetic token must be 64 hex characters")
	}
	oldHash, newHash := sha256.Sum256([]byte(token)), sha256.Sum256([]byte(seed))
	// Replace only this fixture's fresh approval hash. This keeps the
	// real approval/preview ledger and lets harness supply the token
	// as trusted user input before Codex starts its MCP subprocess.
	if _, e := f.Store.DB.ExecContext(
		runCtx,
		"UPDATE join_approvals SET token_hash=? WHERE token_hash=?",
		hex.EncodeToString(newHash[:]),
		hex.EncodeToString(oldHash[:]),
	); e != nil {
		return e
	}

	return nil
}

func writeSyntheticApproval(path string, preview map[string]any, token string) error {
	approval, e := json.Marshal(
		map[string]any{
			"synthetic_only": true,
			"invite_url":     InviteURL,
			"preview":        preview,
			"plan_token":     token,
		},
	)
	if e != nil {
		return e
	}
	// Never overwrite an existing file (including a symlink).
	file, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, writeErr := file.Write(approval)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	if closeErr != nil {
		return closeErr
	}

	return nil
}

type fixtureCorpus struct {
	Groups   []domain.Group   `json:"groups"`
	Messages []domain.Message `json:"messages"`
}

func openFixtureStore(ctx context.Context, dir string, opts Options, groups []domain.Group) (*storage.Store, error) {
	var store *storage.Store
	var err error
	allowed := []string{fixtureGJoinID}
	for _, group := range groups {
		allowed = append(allowed, group.ID)
	}
	if opts.Offline {
		//nolint:exhaustruct_v5 // CollectionPolicy preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
		policy := domain.CollectionPolicy{All: true}
		if opts.Policy != nil {
			policy = *opts.Policy
		}
		store, err = storage.OpenWithPolicy(ctx, filepath.Join(dir, "messages.sqlite"), policy, fixtureRetentionDays)
	} else {
		store, err = storage.Open(ctx, filepath.Join(dir, "messages.sqlite"), allowed, fixtureRetentionDays)
	}
	return store, err
}

func seedFixtureStore(
	ctx context.Context,
	store *storage.Store,
	account string,
	corpus fixtureCorpus,
	opts Options,
) (map[string]any, error) {
	var err error
	if err = store.BindAccount(ctx, account); err != nil {
		return nil, err
	}
	if err = store.ReplaceCatalog(ctx, corpus.Groups); err != nil {
		return nil, err
	}
	if !opts.Empty {
		for _, message := range corpus.Messages {
			message.Source = wireLive
			if err = store.Put(ctx, message); err != nil {
				return nil, err
			}
		}
	}
	if err = store.BeginGap(ctx, "synthetic_missing_history"); err != nil {
		return nil, err
	}
	state := map[string]any{
		"authenticated":     true,
		"collector_state":   "connected",
		"last_connected_at": time.Now().UTC().Format(time.RFC3339Nano),
		"last_event_at":     nil,
		"last_persisted_at": nil,
		"last_error":        nil,
	}
	if opts.Stopped {
		state["collector_state"] = "stopped"
	}
	if err = store.SetState(ctx, state); err != nil {
		return nil, err
	}
	return state, nil
}

func (f *Fixture) startControl(ctx context.Context) error {
	sock, err := local.SocketPath(f.Dir)
	if err != nil {
		return err
	}
	//nolint:exhaustruct_v5 // Keep default socket control and keepalive policy; only the configured listener options are supplied.
	listener, err := (&net.ListenConfig{}).Listen(ctx, "unix", sock)
	if err != nil {
		return err
	}
	if err = os.Chmod(sock, 0600); err != nil {
		//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
		listener.Close()
		return err
	}
	f.http = &http.Server{Handler: collector.ControlHandler(f.joins), ReadHeaderTimeout: fixtureHeaderTimeout}
	go func() { f.done <- f.http.Serve(listener) }()
	return nil
}
