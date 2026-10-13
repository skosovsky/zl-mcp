package observability

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/collector"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/historyimport"
	"github.com/skosovsky/zl-mcp/internal/messaging"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type nativeSender func(context.Context, string, string, *domain.SendQuote) (string, error)

func (f nativeSender) SendDirect(ctx context.Context, peer, text string, quote *domain.SendQuote) (string, error) {
	return f(ctx, peer, text, quote)
}
func nativeStore(t *testing.T) *storage.Store {
	t.Helper()
	s, err := storage.OpenWithPolicy(
		context.Background(),
		filepath.Join(t.TempDir(), "native.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.BindAccount(context.Background(), "PRIVATE_SENTINEL_ACCOUNT"); err != nil {
		t.Fatal(err)
	}
	return s
}

//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestNativeSendRecordsOneAmbiguousEffectWithoutRetry(t *testing.T) {
	// Arrange: real durable owner and in-memory telemetry, with a hostile upstream cause.
	o, reader, exporter := memoryObserver(t)
	s := nativeStore(t)
	cause := &privateError{}
	ctx := shared.WithTelemetry(context.Background(), o)
	var sends atomic.Int64
	manager := messaging.Manager{
		Store:   s,
		Enabled: true,
		Sender: nativeSender(func(context.Context, string, string, *domain.SendQuote) (string, error) {
			sends.Add(1)
			return "", cause
		}),
	}
	args := map[string]any{
		"request_id":   "80000000-0000-4000-8000-000000000008",
		"recipient_id": "PRIVATE_SENTINEL_PEER",
		"text":         "PRIVATE_SENTINEL_TEXT",
	}
	// Act: duplicate UUID observes the same native terminal ledger.
	for range 2 {
		if err := o.Execute(ctx, "zalo_send_direct_message", func(ctx context.Context) (domain.Outcome, error) {
			result, err := manager.Call(ctx, "zalo_send_direct_message", args)
			if err != nil {
				return domain.OutcomeError, err
			}
			if result["status"] != "unknown" {
				t.Error("native ambiguous outcome changed")
			}
			return domain.OutcomeUnknown, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	op, err := s.SendStatus(context.Background(), args["request_id"].(string))
	data := metrics(t, reader)
	// Assert: successful Go return, unknown domain, one upstream mutation and two real transitions.
	if err != nil || op.Status != "unknown" || sends.Load() != 1 || cause.formatted.Load() != 0 {
		t.Fatal("telemetry changed native send contract")
	}
	if count(t, data, "zl_mcp_upstream_calls", map[string]string{"kind": "send", "outcome": "error"}) != 1 {
		t.Fatal("native upstream observation missing")
	}
	for _, states := range [][2]string{{"pending", "sending"}, {"sending", "unknown"}} {
		if count(
			t,
			data,
			"zl_mcp_operation_transitions",
			map[string]string{"kind": "send", "from": states[0], "to": states[1]},
		) != 1 {
			t.Fatal("native send transition missing/repeated")
		}
	}
	if count(
		t,
		data,
		"zl_mcp_domain_calls",
		map[string]string{"tool": "zalo_send_direct_message", "outcome": "unknown"},
	) != 2 {
		t.Fatal("domain unknown changed")
	}
	spans := exporter.GetSpans()
	workers := 0
	for _, span := range spans {
		if span.Name == "zl_mcp.worker" {
			workers++
			if len(span.Links) != 1 {
				t.Fatal("native send worker link missing")
			}
		}
	}
	if workers != 1 {
		t.Fatal("terminal replay started a worker")
	}
	payload, _ := json.Marshal(struct {
		Metrics any `json:"Metrics"`
		Spans   any `json:"Spans"`
	}{data, spans})
	if strings.Contains(string(payload), "PRIVATE_SENTINEL") ||
		strings.Contains(string(payload), args["request_id"].(string)) {
		t.Fatal("native identity/content reached telemetry")
	}
}

type nativeHistorySource struct{ calls atomic.Int64 }

func (s *nativeHistorySource) HistoryPage(
	context.Context,
	domain.ConversationRef,
	string,
	int,
) (domain.HistoryPage, error) {
	s.calls.Add(1)
	more := false
	return domain.HistoryPage{HasMore: &more}, nil
}

//nolint:gocognit // This AAA test keeps arrangement, operation and boundary assertions visible as one ordered scenario.
func TestNativeHistoryWorkerKeepsLedgerAndRequestLink(t *testing.T) {
	// Arrange: the command prepares the real native ledger under an executor span.
	o, reader, exporter := memoryObserver(t)
	s := nativeStore(t)
	base := shared.WithTelemetry(context.Background(), o)
	manager := historyimport.Manager{Store: s}
	args := map[string]any{
		"request_id":        "90000000-0000-4000-8000-000000000009",
		"conversation_type": "group",
		"conversation_id":   "PRIVATE_SENTINEL_GROUP",
		"since":             "2026-09-01T00:00:00Z",
		"until":             "2026-11-01T00:00:00Z",
	}
	var id string
	if err := o.Execute(base, "zalo_import_conversation_history", func(ctx context.Context) (domain.Outcome, error) {
		result, err := manager.Call(ctx, "zalo_import_conversation_history", args)
		if err == nil {
			id = result["operation_id"].(string)
		}
		return domain.OutcomeUnknown, err
	}); err != nil {
		t.Fatal(err)
	}
	source := &nativeHistorySource{}
	ctx, cancel := context.WithCancel(base)
	defer cancel()
	done := make(chan error, 1)
	// Act: the existing worker claims and completes one available source page.
	go func() { done <- historyimport.RunRecovered(ctx, s, source) }()
	deadline := time.After(3 * time.Second)
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		op, err := s.HistoryOperation(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status.State == "completed" {
			break
		}
		select {
		case <-deadline:
			t.Fatal("native history did not complete")
		case <-ticker.C:
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	data := metrics(t, reader)
	spans := exporter.GetSpans()
	// Assert: durable state and one upstream read survive observation; correlation is opaque.
	if source.calls.Load() != 1 {
		t.Fatal("native history source repeated")
	}
	for _, states := range [][2]string{{"queued", "running"}, {"running", "completed"}} {
		if count(
			t,
			data,
			"zl_mcp_operation_transitions",
			map[string]string{"kind": "history", "from": states[0], "to": states[1]},
		) != 1 {
			t.Fatal("native history transition missing")
		}
	}
	if count(t, data, "zl_mcp_upstream_calls", map[string]string{"kind": "history", "outcome": "success"}) != 1 {
		t.Fatal("native history upstream observation missing")
	}
	linked := false
	for _, span := range spans {
		if span.Name == "zl_mcp.worker" && len(span.Links) == 1 {
			linked = true
		}
	}
	if !linked {
		t.Fatal("native history worker lost request reference")
	}
	payload, _ := json.Marshal(struct {
		Metrics any `json:"Metrics"`
		Spans   any `json:"Spans"`
	}{data, spans})
	if strings.Contains(string(payload), "PRIVATE_SENTINEL") || strings.Contains(string(payload), id) {
		t.Fatal("native history identity/content leaked")
	}
}

type nativeJoinAPI struct {
	joined atomic.Bool
	calls  atomic.Int64
}

func (*nativeJoinAPI) AccountID() string { return "PRIVATE_SENTINEL_ACCOUNT" }
func (s *nativeJoinAPI) Groups(context.Context) ([]domain.Group, error) {
	if s.joined.Load() {
		return []domain.Group{{ID: "PRIVATE_SENTINEL_GROUP", Name: "private"}}, nil
	}
	return []domain.Group{}, nil
}
func (*nativeJoinAPI) Group(context.Context, string) (domain.Group, *string, error) {
	return domain.Group{ID: "PRIVATE_SENTINEL_GROUP", Name: "private"}, nil, nil
}
func (*nativeJoinAPI) Inspect(context.Context, string) (domain.Invite, error) {
	return domain.Invite{Group: domain.Group{ID: "PRIVATE_SENTINEL_GROUP", Name: "private"}}, nil
}
func (s *nativeJoinAPI) Join(context.Context, string) error {
	s.calls.Add(1)
	s.joined.Store(true)
	return nil
}
func TestNativeJoinLinksWorkerBeforeCommandReturns(t *testing.T) {
	// Arrange: real approval/operation tables and a synthetic upstream, one service context.
	o, reader, exporter := memoryObserver(t)
	s := nativeStore(t)
	base := shared.WithTelemetry(context.Background(), o)
	api := &nativeJoinAPI{}
	manager := collector.NewJoin(base, s, api, true)
	preview, err := manager.Inspect(base, "https://zalo.me/g/synthetic")
	if err != nil {
		t.Fatal(err)
	}
	token, err := manager.Approve(base, *preview["preview_id"].(*string))
	if err != nil {
		t.Fatal(err)
	}
	var id string
	// Act: start from an executor, wait for the existing worker, then read its ledger.
	err = o.Execute(base, "zalo_join_group", func(ctx context.Context) (domain.Outcome, error) {
		result, startErr := manager.Start(ctx, token, "10000000-0000-4000-8000-000000000001")
		if startErr == nil {
			id = result["operation_id"].(string)
		}
		return domain.OutcomeUnknown, startErr
	})
	if err != nil {
		t.Fatal(err)
	}
	manager.Wait()
	result, err := manager.Operation(base, id)
	data := metrics(t, reader)
	spans := exporter.GetSpans()
	// Assert: exactly one mutation, confirmed native outcome and opaque request link.
	if err != nil || result["status"] != "joined" || api.calls.Load() != 1 {
		t.Fatal("native join behavior changed")
	}
	if count(
		t,
		data,
		"zl_mcp_operation_transitions",
		map[string]string{"kind": "join", "from": "running", "to": "joined"},
	) != 1 {
		t.Fatal("native join transition missing")
	}
	linked := false
	for _, span := range spans {
		if span.Name == "zl_mcp.worker" && len(span.Links) == 1 {
			linked = true
		}
	}
	if !linked {
		t.Fatal("join worker started without command reference")
	}
	payload, _ := json.Marshal(struct {
		Metrics any `json:"Metrics"`
		Spans   any `json:"Spans"`
	}{data, spans})
	if strings.Contains(string(payload), "PRIVATE_SENTINEL") || strings.Contains(string(payload), id) ||
		strings.Contains(string(payload), token) {
		t.Fatal("native join identity/token leaked")
	}
}

type nativeCallbackTransport struct{ calls atomic.Int64 }

func (s *nativeCallbackTransport) RoundTrip(*http.Request) (*http.Response, error) {
	s.calls.Add(1)
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}
func TestNativeEventsObservesExistingDeliveryReceipt(t *testing.T) {
	// Arrange: real event/subscription/delivery owners, callback is entirely synthetic.
	o, reader, exporter := memoryObserver(t)
	s := nativeStore(t)
	base := shared.WithTelemetry(context.Background(), o)
	at := time.Now().UTC()
	revision, err := s.SubscriptionRevision(base, "PRIVATE_SENTINEL_SUB")
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ActivateSubscription(
		base,
		storage.EventSubscription{
			ID:        "PRIVATE_SENTINEL_SUB",
			Principal: "owner",
			GroupID:   "PRIVATE_SENTINEL_GROUP",
			Callback:  "https://callback.example/private",
			Secret:    "whsec_" + base64.StdEncoding.EncodeToString([]byte("synthetic-key-for-webhook-32bytes!")),
		},
		revision,
		at,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Put(
		base,
		domain.Message{
			GroupID:  "PRIVATE_SENTINEL_GROUP",
			ID:       "PRIVATE_SENTINEL_MESSAGE",
			SenderID: "private",
			Text:     "PRIVATE_SENTINEL_TEXT",
			SentAt:   at,
			Source:   "live",
		},
	); err != nil {
		t.Fatal(err)
	}
	worker, err := events.NewWorker(s)
	if err != nil {
		t.Fatal(err)
	}
	transport := &nativeCallbackTransport{}
	worker.Client = &http.Client{Transport: transport}
	var diagnosticLogs bytes.Buffer
	worker.Logger = slog.New(slog.NewJSONHandler(&diagnosticLogs, nil))
	worker.TraceLogger = worker.Logger
	if _, err = s.FanoutEvents(base, at, worker.Policy, worker.Encoder.Encode); err != nil {
		t.Fatal(err)
	}
	// Act: existing native lease and receipt flow executes once.
	handled, err := worker.DeliverOne(base, at)
	again, againErr := worker.DeliverOne(base, at.Add(time.Second))
	data := metrics(t, reader)
	spans := exporter.GetSpans()
	// Assert: no second callback, actual transitions, and no payload/signed headers in sinks.
	if !handled || err != nil || again || againErr != nil || transport.calls.Load() != 1 {
		t.Fatal("event callback/receipt behavior changed")
	}
	for _, states := range [][2]string{{"pending", "sending"}, {"sending", "delivered"}} {
		if count(
			t,
			data,
			"zl_mcp_operation_transitions",
			map[string]string{"kind": "events", "from": states[0], "to": states[1]},
		) != 1 {
			t.Fatal("native Events transition missing")
		}
	}
	if count(t, data, "zl_mcp_upstream_calls", map[string]string{"kind": "events", "outcome": "success"}) != 1 {
		t.Fatal("native Events upstream signal missing")
	}
	payload, _ := json.Marshal(struct {
		Metrics any `json:"Metrics"`
		Spans   any `json:"Spans"`
	}{data, spans})
	if strings.Contains(string(payload)+diagnosticLogs.String(), "PRIVATE_SENTINEL") {
		t.Fatal("native Events identity/content leaked")
	}
}
