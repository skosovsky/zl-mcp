package mcpserver

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// This harness is a synthetic host adapter, not a cloud-host delivery claim.
// Its action is a local counter authorized solely by these temporary fixtures.
type recoveryReadyHarness struct {
	t                               *testing.T
	dir, path, token, secret        string
	key                             []byte
	store                           *storage.Store
	endpoint, receiver              *httptest.Server
	client                          *mcp.ClientSession
	callbackClient                  *http.Client
	worker                          *events.Worker
	business, generation, companion string
	params                          map[string]any
	mu                              sync.Mutex
	consume                         bool
	actions                         map[string]int
	businessPages                   chan recoveryReadResult
	controls                        chan recoveryControlResult
}
type recoveryReadResult struct {
	page map[string]any
	err  error
}
type recoveryControlResult struct {
	event  map[string]any
	body   []byte
	header http.Header
	page   map[string]any
	err    error
}
type recoveryBearer struct{ token string }

func (b recoveryBearer) RoundTrip(r *http.Request) (*http.Response, error) {
	copyRequest := r.Clone(r.Context())
	copyRequest.Header = r.Header.Clone()
	copyRequest.Header.Set("Authorization", "Bearer "+b.token)
	return http.DefaultTransport.RoundTrip(copyRequest)
}

func newRecoveryReadyHarness(t *testing.T) *recoveryReadyHarness {
	t.Helper()
	dir := t.TempDir()
	key := []byte("synthetic-recovery-key-32-bytes!!")
	h := &recoveryReadyHarness{
		t:             t,
		dir:           dir,
		path:          filepath.Join(dir, "state.sqlite"),
		token:         strings.Repeat("r", 32),
		secret:        "whsec_" + base64.StdEncoding.EncodeToString(key),
		key:           key,
		actions:       map[string]int{},
		businessPages: make(chan recoveryReadResult, 8),
		controls:      make(chan recoveryControlResult, 8),
	}
	h.receiver = httptest.NewTLSServer(http.HandlerFunc(h.receive))
	target, err := url.Parse(h.receiver.URL)
	if err != nil {
		t.Fatal(err)
	}
	h.callbackClient = &http.Client{
		Transport: localReceiverTransport{target: target, base: h.receiver.Client().Transport},
	}
	h.open()
	t.Cleanup(func() { h.stop(); h.receiver.Close() })
	h.params = map[string]any{
		"name": domain.ConversationMessageCreatedV3,
		"arguments": map[string]any{
			"scope":               "conversation",
			"conversation_type":   "direct",
			"conversation_id":     "peer",
			"direction":           "incoming",
			"first_incoming_only": false,
		},
		"delivery": map[string]any{"mode": "webhook", "url": "https://receiver.example/business", "secret": h.secret},
	}
	result := httpRPC(t, h.endpoint.URL+"/mcp", h.token, "events/subscribe", h.params)
	h.business = result["id"].(string)
	listed := h.tool("zalo_list_event_subscriptions", map[string]any{"include_recovery": true})
	subscriptions := listed["subscriptions"].([]any)
	if len(subscriptions) != 1 {
		t.Fatalf("unexpected subscriptions: %v", listed)
	}
	sub := subscriptions[0].(map[string]any)
	h.generation = sub["generation_reference"].(string)
	if sub["recovery_mode"] != "unsupported" {
		t.Fatal("unconfigured continuation must be explicit unsupported")
	}
	return h
}
func (h *recoveryReadyHarness) open() {
	h.t.Helper()
	var err error
	h.store, err = storage.OpenWithPolicy(h.t.Context(), h.path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		h.t.Fatal(err)
	}
	if err = h.store.BindAccount(h.t.Context(), "synthetic-recovery-account"); err != nil {
		h.t.Fatal(err)
	}
	service, err := migrationHTTP(h.t, h.store, h.dir, h.token)
	if err != nil {
		h.t.Fatal(err)
	}
	service.Events.(*events.SubscriptionManager).Client = h.callbackClient
	h.endpoint = httptest.NewServer(service)
	h.client, err = mcp.NewClient(&mcp.Implementation{Name: "synthetic-recovery-host", Version: "1"}, nil).
		Connect(h.t.Context(), &mcp.StreamableClientTransport{Endpoint: h.endpoint.URL + "/mcp", HTTPClient: &http.Client{Transport: recoveryBearer{token: h.token}}, DisableStandaloneSSE: true}, nil)
	if err != nil {
		h.t.Fatal(err)
	}
	h.worker, err = events.NewWorker(h.store)
	if err != nil {
		h.t.Fatal(err)
	}
	h.worker.Client = h.callbackClient
}
func (h *recoveryReadyHarness) stop() {
	h.t.Helper()
	if h.client != nil {
		if err := h.client.Close(); err != nil {
			h.t.Error(err)
		}
		h.client = nil
	}
	if h.endpoint != nil {
		h.endpoint.Close()
		h.endpoint = nil
	}
	if h.store != nil {
		if err := h.store.Close(); err != nil {
			h.t.Error(err)
		}
		h.store = nil
	}
}
func (h *recoveryReadyHarness) tool(name string, args map[string]any) map[string]any {
	h.t.Helper()
	page, err := h.call(name, args)
	if err != nil {
		h.t.Fatal(err)
	}
	return page
}
func (h *recoveryReadyHarness) call(name string, args map[string]any) (map[string]any, error) {
	result, err := h.client.CallTool(h.t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return nil, err
	}
	if result.IsError {
		return nil, fmt.Errorf("synthetic %s failed", name)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		return nil, err
	}
	var page map[string]any
	err = json.Unmarshal(raw, &page)
	return page, err
}
func (h *recoveryReadyHarness) read() (map[string]any, error) {
	return h.call(
		"zalo_read_subscription_events",
		map[string]any{"subscription_id": h.business, "include_diagnostics": true, "limit": 20},
	)
}
func (h *recoveryReadyHarness) companionParams() map[string]any {
	return map[string]any{
		"name":      domain.JournalReadyV1,
		"arguments": map[string]any{"subscription_id": h.business, "generation_reference": h.generation},
		"delivery":  map[string]any{"mode": "webhook", "url": "https://receiver.example/control", "secret": h.secret},
	}
}
func (h *recoveryReadyHarness) enable() {
	h.t.Helper()
	result := httpRPC(h.t, h.endpoint.URL+"/mcp", h.token, "events/subscribe", h.companionParams())
	h.companion = result["id"].(string)
}
func (h *recoveryReadyHarness) put(id string) {
	h.t.Helper()
	if err := h.store.Put(
		h.t.Context(),
		domain.Message{
			Conversation: domain.ConversationRef{Type: domain.ConversationDirect, ID: "peer"},
			ID:           id,
			SenderID:     "peer",
			Direction:    "incoming",
			Text:         "synthetic business " + id,
			SentAt:       time.Now(),
			Source:       "live",
		},
	); err != nil {
		h.t.Fatal(err)
	}
	if _, err := h.store.FanoutProfileEvents(
		h.t.Context(),
		time.Now(),
		h.worker.Policy,
		h.worker.Encoder.EncodeProfile,
	); err != nil {
		h.t.Fatal(err)
	}
}
func (h *recoveryReadyHarness) receive(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if !h.signature(r.Header, body) {
		h.t.Error("invalid synthetic receiver HMAC")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	var event map[string]any
	if err = json.Unmarshal(body, &event); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	if event["type"] == "verification" {
		if err = json.NewEncoder(w).Encode(map[string]any{"challenge": event["challenge"]}); err != nil {
			h.t.Error(err)
		}
		return
	}
	if event["name"] == domain.JournalReadyV1 {
		h.receiveControl(w, r, event, body)
		return
	}
	page, err := h.read()
	h.businessPages <- recoveryReadResult{page: page, err: err}
	w.WriteHeader(http.StatusNoContent)
}
func (h *recoveryReadyHarness) signature(header http.Header, body []byte) bool {
	mac := hmac.New(sha256.New, h.key)
	_, _ = mac.Write([]byte(header.Get("Webhook-Id") + "." + header.Get("Webhook-Timestamp") + "."))
	_, _ = mac.Write(body)
	signature, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(header.Get("Webhook-Signature"), "v1,"))
	return err == nil && hmac.Equal(signature, mac.Sum(nil))
}

func (h *recoveryReadyHarness) receiveControl(
	w http.ResponseWriter,
	r *http.Request,
	event map[string]any,
	body []byte,
) {
	page, err := h.read()
	h.mu.Lock()
	consume := h.consume
	h.mu.Unlock()
	if err == nil && consume {
		err = h.process(page)
	}
	h.controls <- recoveryControlResult{event: event, body: append([]byte(nil), body...), header: r.Header.Clone(), page: page, err: err}
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (h *recoveryReadyHarness) process(page map[string]any) error {
	for _, value := range page["events"].([]any) {
		entry := value.(map[string]any)
		if entry["event"] != nil {
			event := entry["event"].(map[string]any)
			id := event["eventId"].(string)
			h.mu.Lock()
			if h.actions[id] == 0 {
				h.actions[id]++
			}
			h.mu.Unlock()
		}
		if _, err := h.call(
			"zalo_ack_subscription_events",
			map[string]any{"subscription_id": h.business, "receipt": entry["receipt"]},
		); err != nil {
			return err
		}
	}
	return nil
}
func (h *recoveryReadyHarness) takeBusiness() map[string]any {
	h.t.Helper()
	select {
	case got := <-h.businessPages:
		if got.err != nil {
			h.t.Fatal(got.err)
		}
		return got.page
	case <-time.After(5 * time.Second):
		h.t.Fatal("synthetic business callback timeout")
		return nil
	}
}
func (h *recoveryReadyHarness) takeControl() recoveryControlResult {
	h.t.Helper()
	select {
	case got := <-h.controls:
		if got.err != nil {
			h.t.Fatal(got.err)
		}
		return got
	case <-time.After(5 * time.Second):
		h.t.Fatal("synthetic control callback timeout")
		return recoveryControlResult{}
	}
}
func (h *recoveryReadyHarness) deliverControl(at time.Time, want bool) {
	h.t.Helper()
	got, err := h.worker.DeliverRecoveryOne(h.t.Context(), at)
	if err != nil || got != want {
		h.t.Fatalf("control handled=%v want=%v error=%v", got, want, err)
	}
}
func (h *recoveryReadyHarness) assertNoNativeActions() {
	h.t.Helper()
	for _, table := range []string{"message_action_operations", "send_operations", "join_requests"} {
		var count int
		if err := h.store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			h.t.Fatalf("unexpected native operation in %s count=%d error=%v", table, count, err)
		}
	}
}

// Embedding forwards every recovery method to the same DB owner; only the
// business completion commit is paused, after the real receiver returned2xx.
type recoveryFinishBarrier struct {
	*storage.Store

	entered, release chan struct{}
}

func (q *recoveryFinishBarrier) FinishDelivery(
	ctx context.Context,
	d storage.EventDelivery,
	outcome storage.DeliveryOutcome,
	at time.Time,
) error {
	close(q.entered)
	select {
	case <-q.release:
	case <-ctx.Done():
		return ctx.Err()
	}
	return q.Store.FinishDelivery(ctx, d, outcome, at)
}

//nolint:gocognit // Ordered synthetic-host AAA scenario spans both race windows, virtual long blocking, restart, reminders and ordered ack.
func TestSyntheticHostRecoveryActualSDKClosesBothRacesAfterRestart(t *testing.T) {
	// Arrange: temporary account/state, HTTP MCP SDK, real TLS receiver and signed control profile.
	h := newRecoveryReadyHarness(t)
	h.assertNoNativeActions()
	h.enable()
	h.put("business-only")
	barrier := &recoveryFinishBarrier{Store: h.store, entered: make(chan struct{}), release: make(chan struct{})}
	h.worker.Queue = barrier
	t.Cleanup(func() {
		select {
		case <-barrier.release:
		default:
			close(barrier.release)
		}
	})
	finished := make(chan error, 1)
	go func() {
		handled, err := h.worker.DeliverOne(t.Context(), time.Now())
		if err == nil && !handled {
			err = errors.New("business not claimed")
		}
		finished <- err
	}()
	// Act/assert: callback's tools/read before returning2xx is blocked, not empty-success.
	before := h.takeBusiness()
	if before["blocked_on_delivery"] != true || len(before["events"].([]any)) != 0 {
		t.Fatal("before2xx did not expose blocked journal")
	}
	select {
	case <-barrier.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("business2xx did not reach commit barrier")
	}
	between, err := h.read()
	if err != nil || between["blocked_on_delivery"] != true {
		t.Fatal("after2xx-beforecommit must stay blocked", err)
	}
	// Virtual45seconds exceeds the documented30second client retry budget; no sleep or new insertion.
	later := time.Now().Add(45 * time.Second)
	h.deliverControl(later, false)
	blocked, err := h.read()
	if err != nil || blocked["blocked_on_delivery"] != true || blocked["recovery_mode"] != "companion" {
		t.Fatal("longblock lost durable continuation", err)
	}
	close(barrier.release)
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	// Restart the actual temporary SQLite owner, service, SDK connection and worker.
	h.stop()
	h.open()
	h.deliverControl(later.Add(time.Second), true)
	first := h.takeControl()
	if len(first.page["events"].([]any)) != 1 {
		t.Fatal("ready callback did not recover exact business event")
	}
	recovered := first.page["events"].([]any)[0].(map[string]any)["event"].(map[string]any)
	if recovered["data"].(map[string]any)["message_id"] != "business-only" {
		t.Fatal("wrong event recovered")
	}
	// The accepted control cannot immediately spin another callback without a bounded interval.
	h.deliverControl(time.Now(), false)
	// First control2xx intentionally did not ack. Restart again; a fresh bounded reminder must survive.
	h.stop()
	h.open()
	h.mu.Lock()
	h.consume = true
	h.mu.Unlock()
	h.deliverControl(later.Add(40*time.Second), true)
	second := h.takeControl()
	if first.header.Get("Webhook-Id") == second.header.Get("Webhook-Id") {
		t.Fatal("reminder needs fresh transport ID")
	}
	firstData := first.event["data"].(map[string]any)
	secondData := second.event["data"].(map[string]any)
	if firstData["recovery_reference"] != secondData["recovery_reference"] ||
		firstData["generation_reference"] != h.generation {
		t.Fatal("restart changed recovery episode/generation")
	}
	// Replay an accepted control: journal checkpoint suppresses a second synthetic action.
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		h.receiver.URL+"/control",
		bytes.NewReader(second.body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header = second.header.Clone()
	response, err := h.receiver.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	duplicate := h.takeControl()
	if len(duplicate.page["events"].([]any)) != 0 {
		t.Fatal("duplicate woke already processed event")
	}
	page, err := h.read()
	if err != nil || len(page["events"].([]any)) != 0 {
		t.Fatal("ack did not advance durable checkpoint", err)
	}
	h.deliverControl(later.Add(3*time.Minute), false)
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.actions) != 1 || h.actions[recovered["eventId"].(string)] != 1 {
		t.Fatal("synthetic action duplicated or missing")
	}
	h.assertNoNativeActions()
}

func TestSyntheticHostRecoveryUnsupportedModeIsVisibleWithoutCompanion(t *testing.T) {
	// Arrange: a business subscription alone must not claim autonomous recovery.
	h := newRecoveryReadyHarness(t)
	h.assertNoNativeActions()
	h.put("without-control")
	// Act: signed callback starts an actualSDK read while business state is sending.
	handled, err := h.worker.DeliverOne(t.Context(), time.Now())
	if err != nil || !handled {
		t.Fatal("business delivery failed", err)
	}
	blocked := h.takeBusiness()
	if blocked["blocked_on_delivery"] != true || blocked["recovery_mode"] != "unsupported" ||
		blocked["recovery_state"] != "unsupported" {
		t.Fatal("unsupported configuration was hidden")
	}
	// Assert: this mode has no fabricated wakeup guarantee, even after a long virtual wait.
	h.deliverControl(time.Now().Add(time.Hour), false)
	page, err := h.read()
	if err != nil || page["recovery_mode"] != "unsupported" || len(page["events"].([]any)) != 1 {
		t.Fatal("business record or explicit unsupported status lost", err)
	}
	h.assertNoNativeActions()
}

//nolint:gocognit // Paired AAA scenarios differ only in whether cancellation also replaces business generation.
func TestSyntheticHostRecoveryCancellationAndGenerationSuppressClaimedControls(t *testing.T) {
	for _, replaceBusiness := range []bool{false, true} {
		t.Run(fmt.Sprintf("replace-business-%v", replaceBusiness), func(t *testing.T) {
			// Arrange: a signed business callback commits, leaving one unprocessed record.
			h := newRecoveryReadyHarness(t)
			h.enable()
			h.put("never-process-stale-control")
			handled, err := h.worker.DeliverOne(t.Context(), time.Now())
			if err != nil || !handled {
				t.Fatal("business delivery", err)
			}
			h.takeBusiness()
			at := time.Now()
			claim, err := h.store.ClaimRecoveryDelivery(t.Context(), at, h.worker.Policy)
			if err != nil || claim == nil {
				t.Fatal("ready control not claimed", err)
			}
			// Act: cancel control, or replace its exact business generation through Events HTTP.
			params := h.companionParams()
			if replaceBusiness {
				params = h.params
			}
			delivery := params["delivery"].(map[string]any)
			cancelParams := map[string]any{
				"name":      params["name"],
				"arguments": params["arguments"],
				"delivery":  map[string]any{"mode": delivery["mode"], "url": delivery["url"]},
			}
			httpRPC(t, h.endpoint.URL+"/mcp", h.token, "events/unsubscribe", cancelParams)
			if replaceBusiness {
				result := httpRPC(t, h.endpoint.URL+"/mcp", h.token, "events/subscribe", h.params)
				if result["id"] != h.business {
					t.Fatal("canonical business identity changed")
				}
				listed := h.tool("zalo_list_event_subscriptions", map[string]any{"include_recovery": true})
				sub := listed["subscriptions"].([]any)[0].(map[string]any)
				if sub["generation_reference"] == h.generation || sub["recovery_mode"] != "unsupported" {
					t.Fatal("replacement inherited old recovery generation")
				}
			}
			// Assert: even a previously claimed callback cannot dispatch for obsolete authority.
			allowed, err := h.store.RecoveryDispatchAllowed(t.Context(), *claim)
			if err != nil || allowed {
				t.Fatal("stale control still dispatchable", err)
			}
			h.deliverControl(at.Add(time.Minute), false)
			select {
			case <-h.controls:
				t.Fatal("cancelled/stale callback delivered")
			default:
			}
			h.mu.Lock()
			actions := len(h.actions)
			h.mu.Unlock()
			if actions != 0 {
				t.Fatal("stale control performed synthetic action")
			}
			h.assertNoNativeActions()
		})
	}
}

func TestSyntheticHostRecoveryWakesExplicitTerminalGapWithoutInventingMessage(t *testing.T) {
	// Arrange: one temporary business record with a terminal receiver rejection.
	h := newRecoveryReadyHarness(t)
	h.enable()
	h.put("terminal-rejection")
	at := time.Now()
	delivery, err := h.store.ClaimDelivery(t.Context(), at, h.worker.Policy)
	if err != nil || delivery == nil {
		t.Fatal("business claim", err)
	}
	if err = h.store.FinishDelivery(
		t.Context(),
		*delivery,
		storage.DeliveryOutcome{State: "failed", Reason: "http_rejected"},
		at,
	); err != nil {
		t.Fatal(err)
	}
	// Act: control wakes the realSDK journal workflow without new Zalo records.
	h.deliverControl(at.Add(time.Second), true)
	control := h.takeControl()
	entries := control.page["events"].([]any)
	// Assert: exact gap and safe reason remain visible; no inferred event/action or automatic ack.
	if len(entries) != 1 {
		t.Fatal("terminal gap was lost")
	}
	entry := entries[0].(map[string]any)
	if entry["event"] != nil || entry["gap_reason"] != "delivery_failed" ||
		entry["delivery_reason"] != "http_rejected" ||
		entry["receipt"] == "" {
		t.Fatal("terminal error was misrepresented as a message")
	}
	page, err := h.read()
	if err != nil || len(page["events"].([]any)) != 1 {
		t.Fatal("unhandled gap was silently acknowledged", err)
	}
	h.mu.Lock()
	actions := len(h.actions)
	h.mu.Unlock()
	if actions != 0 {
		t.Fatal("invented action for terminal gap")
	}
	h.assertNoNativeActions()
}
