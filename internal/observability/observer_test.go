package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/metry"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// Borrowed SDK is confined to the synchronous test fixture. Production chooses
// one service-owned metry.New provider; this fixture owns both SDK shutdowns.
func memoryObserver(t *testing.T) (*Observer, *sdkmetric.ManualReader, *tracetest.InMemoryExporter) {
	t.Helper()
	exporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(exporter))
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	p, err := metry.FromSDK(metry.SDKDependencies{TracerProvider: tp, MeterProvider: mp, Propagator: propagation.TraceContext{}})
	if err != nil {
		t.Fatal(err)
	}
	o, err := New(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()); _ = mp.Shutdown(context.Background()) })
	return o, reader, exporter
}

type privateFailure struct{ formatted atomic.Int64 }

func (e *privateFailure) Error() string  { e.formatted.Add(1); return "PRIVATE_SENTINEL_ERROR" }
func (e *privateFailure) String() string { e.formatted.Add(1); return "PRIVATE_SENTINEL_PANIC" }

func metrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var result metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func count(t *testing.T, data metricdata.ResourceMetrics, name string, attributes map[string]string) int64 {
	t.Helper()
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok {
				t.Fatalf("%s is not counter", name)
			}
			for _, point := range sum.DataPoints {
				match := true
				for k, v := range attributes {
					a, ok := point.Attributes.Value(attribute.Key(k))
					if !ok || a.AsString() != v {
						match = false
					}
				}
				if match {
					return point.Value
				}
			}
		}
	}
	return 0
}

func TestTechnicalAndDomainOutcomesRemainIndependentAndPrivate(t *testing.T) {
	// Arrange: hostile formatters must never run, even during panic observation.
	o, reader, exporter := memoryObserver(t)
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	secret := &privateFailure{}
	ctx := context.Background()
	tool := "zalo_send_direct_message"
	outcomes := []domain.Outcome{domain.OutcomeSuccess, domain.OutcomeError, domain.OutcomePartial, domain.OutcomeUnknown, domain.OutcomeCancelled}
	var calls int
	// Act: domain outcomes accompany nil Go errors; original error/panic survive.
	for _, outcome := range outcomes {
		if err := o.Execute(ctx, tool, func(context.Context) (domain.Outcome, error) { calls++; return outcome, nil }); err != nil {
			t.Fatal("domain observation manufactured a Go error")
		}
	}
	got := o.Execute(ctx, tool, func(context.Context) (domain.Outcome, error) { calls++; return domain.OutcomeSuccess, secret })
	if got != secret {
		t.Fatal("original error identity changed")
	}
	func() {
		defer func() {
			if recover() != secret {
				t.Error("original panic identity changed")
			}
		}()
		_ = o.Execute(ctx, tool, func(context.Context) (domain.Outcome, error) { calls++; panic(secret) })
	}()
	cancelled := o.Execute(ctx, tool, func(context.Context) (domain.Outcome, error) { calls++; return domain.OutcomeSuccess, context.Canceled })
	data := metrics(t, reader)
	// Assert: technical success includes the five nil-error business outcomes.
	if calls != 8 || cancelled != context.Canceled {
		t.Fatal("observation changed invocation count/result")
	}
	for status, want := range map[string]int64{"success": 5, "error": 2, "panic": 1} {
		if got := count(t, data, "executor_operation_calls", map[string]string{"operation": tool, "status": status}); got != want {
			t.Errorf("technical %s: got %d want %d", status, got, want)
		}
	}
	for outcome, want := range map[string]int64{"success": 1, "error": 2, "partial": 1, "unknown": 1, "cancelled": 2, "panic": 1} {
		if got := count(t, data, "zl_mcp_domain_calls", map[string]string{"tool": tool, "outcome": outcome}); got != want {
			t.Errorf("domain %s: got %d want %d", outcome, got, want)
		}
	}
	spans := exporter.GetSpans()
	if len(spans) != 16 {
		t.Errorf("got %d spans for8calls; want8hosts+8executors", len(spans))
	}
	payload, _ := json.Marshal(struct {
		Metrics metricdata.ResourceMetrics
		Spans   tracetest.SpanStubs
	}{data, spans})
	if strings.Contains(string(payload)+logs.String(), "PRIVATE_SENTINEL") || secret.formatted.Load() != 0 {
		t.Fatal("private formatter/content reached telemetry")
	}
}

func TestSignalsAndOpaqueWorkerCorrelation(t *testing.T) {
	// Arrange.
	o, reader, exporter := memoryObserver(t)
	ctx := context.Background()
	secret := "PRIVATE_SENTINEL_OPERATION"
	// Act: capture the execution reference, then work with an independent context.
	_ = o.Execute(ctx, "zalo_join_group", func(ctx context.Context) (domain.Outcome, error) {
		o.Remember(ctx, domain.WorkJoin, secret)
		return domain.OutcomeUnknown, nil
	})
	var workerCalls, upstreamCalls int
	err := o.Worker(ctx, domain.WorkJoin, secret, func(ctx context.Context) error {
		workerCalls++
		return o.Upstream(ctx, domain.WorkJoin, func(context.Context) error { upstreamCalls++; return nil })
	})
	o.ResponseBytes(ctx, "zalo_join_group", domain.TransportHTTP, 1234)
	o.Transition(ctx, domain.WorkJoin, domain.StateRunning, domain.StateJoined, observedAge(2*time.Second))
	o.Queue(ctx, domain.WorkJoin, 3)
	// Unknown tool/state/kind/transport input creates only bounded other/unknown.
	_ = o.Execute(ctx, secret, func(context.Context) (domain.Outcome, error) { return domain.Outcome(255), nil })
	o.ResponseBytes(ctx, secret, domain.Transport(255), 100)
	o.Transition(ctx, domain.WorkKind(255), domain.OperationState(255), domain.OperationState(254), observedAge(-time.Second))
	o.Queue(ctx, domain.WorkKind(255), 100)
	data := metrics(t, reader)
	spans := exporter.GetSpans()
	// Assert: all eight signals exist, and the worker links to the execution.
	if err != nil || workerCalls != 1 || upstreamCalls != 1 {
		t.Fatal("worker observation changed behavior")
	}
	found := map[string]bool{}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			found[m.Name] = true
		}
	}
	for _, name := range []string{"executor_operation_calls", "executor_operation_duration", "zl_mcp_domain_calls", "zl_mcp_response_bytes", "zl_mcp_upstream_calls", "zl_mcp_operation_transitions", "zl_mcp_worker_queue_depth", "zl_mcp_operation_age_seconds"} {
		if !found[name] {
			t.Errorf("missing signal %s", name)
		}
	}
	var linked bool
	for _, s := range spans {
		if s.Name == "zl_mcp.worker" {
			if len(s.Links) != 1 || s.Links[0].SpanContext.TraceID() == s.SpanContext.TraceID() {
				t.Fatal("worker must link retained execution without reusing operation ID as trace ID")
			}
			for _, parent := range spans {
				if parent.Name == "zalo_join_group" && parent.SpanContext.TraceID() == s.Links[0].SpanContext.TraceID() && parent.SpanContext.SpanID() == s.Links[0].SpanContext.SpanID() {
					linked = true
				}
			}
		}
	}
	if !linked {
		t.Fatal("worker execution link missing")
	}
	payload, _ := json.Marshal(struct {
		Metrics metricdata.ResourceMetrics
		Spans   tracetest.SpanStubs
	}{data, spans})
	if strings.Contains(string(payload), secret) {
		t.Fatal("opaque operation identity leaked")
	}
	if got := count(t, data, "zl_mcp_domain_calls", map[string]string{"tool": "other", "outcome": "unknown"}); got != 1 {
		t.Fatal("unknown input not collapsed")
	}
}

func TestTraceCacheBoundAndExpiry(t *testing.T) {
	// Arrange.
	o, _, _ := memoryObserver(t)
	now := time.Now()
	o.now = func() time.Time { return now }
	var remembered context.Context
	// Act.
	_ = o.Execute(context.Background(), "zalo_send_direct_message", func(ctx context.Context) (domain.Outcome, error) { remembered = ctx; return domain.OutcomeSuccess, nil })
	for i := 0; i < traceCacheLimit+10; i++ {
		o.Remember(remembered, domain.WorkSend, fmt.Sprint(i))
	}
	// Assert: bounded without modifying durable operation state.
	if len(o.references) != traceCacheLimit {
		t.Fatalf("cache size %d", len(o.references))
	}
	now = now.Add(traceCacheTTL)
	_ = o.Worker(context.Background(), domain.WorkSend, "no-reference", func(context.Context) error { return nil })
	if len(o.references) != 0 {
		t.Fatal("expired opaque references retained")
	}
}

func TestNilObserverPreservesOriginalCallsAndErrors(t *testing.T) {
	// Arrange.
	o, err := New(nil)
	if err != nil || o != nil {
		t.Fatal("nil provider not disabled")
	}
	original := errors.New("private business failure")
	calls := 0
	// Act.
	err = o.Execute(context.Background(), "anything", func(context.Context) (domain.Outcome, error) { calls++; return domain.OutcomeUnknown, original })
	upstreamErr := o.Upstream(context.Background(), domain.WorkSend, func(context.Context) error { calls++; return original })
	workerErr := o.Worker(context.Background(), domain.WorkSend, "opaque", func(context.Context) error { calls++; return original })
	o.ResponseBytes(context.Background(), "anything", domain.TransportResource, 3)
	o.Queue(context.Background(), domain.WorkHistory, 2)
	o.Transition(context.Background(), domain.WorkHistory, domain.StateQueued, domain.StateRunning, observedAge(time.Second))
	o.Remember(context.Background(), domain.WorkHistory, "opaque")
	// Assert.
	if calls != 3 || err != original || upstreamErr != original || workerErr != original {
		t.Fatal("disabled observer changed business behavior")
	}
}

func observedAge(value time.Duration) *time.Duration { return &value }

func TestUnavailableNativeCreationOmitsAgeWithoutInventingZero(t *testing.T) {
	// Arrange: native state remains observable even when no creation timestamp survived.
	o, reader, _ := memoryObserver(t)
	// Act.
	o.Transition(context.Background(), domain.WorkEvents, domain.StatePending, domain.StateSending, nil)
	data := metrics(t, reader)
	// Assert: the transition is counted; an absent timestamp cannot become a zero age sample.
	if count(t, data, "zl_mcp_operation_transitions", map[string]string{"kind": "events", "from": "pending", "to": "sending"}) != 1 {
		t.Fatal("native transition was dropped")
	}
	for _, scope := range data.ScopeMetrics {
		for _, m := range scope.Metrics {
			if m.Name == "zl_mcp_operation_age_seconds" {
				t.Fatal("missing native timestamp manufactured age")
			}
		}
	}
}

type callbackFailure struct{ calls atomic.Int64 }

func (*callbackFailure) Error() string   { panic("PRIVATE_SENTINEL_FORMATTER") }
func (e *callbackFailure) Is(error) bool { e.calls.Add(1); panic("PRIVATE_SENTINEL_IS_CALLBACK") }
func TestObservationDoesNotInvokeArbitraryErrorClassificationCallbacks(t *testing.T) {
	// Arrange: classification hooks are user code, not trusted diagnostic fields.
	o, _, _ := memoryObserver(t)
	original := &callbackFailure{}
	// Act.
	err := o.Execute(context.Background(), "zalo_get_status", func(context.Context) (domain.Outcome, error) { return domain.OutcomeSuccess, original })
	// Assert: the original business error survives without callback execution.
	if err != original || original.calls.Load() != 0 {
		t.Fatal("telemetry ran arbitrary error classification code")
	}
}
