package observability

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func ownedOptions() Options {
	return Options{
		Enabled:         true,
		Exporter:        "none",
		ExportTimeout:   100 * time.Millisecond,
		ShutdownTimeout: 100 * time.Millisecond,
	}
}

type failingTraceExporter struct {
	mu        sync.Mutex
	resources []string
	cause     *privateError
	stall     bool
	shutdowns int
}

func (e *failingTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) error {
	e.mu.Lock()
	for _, s := range spans {
		e.resources = append(e.resources, s.Resource().String())
	}
	e.mu.Unlock()
	if e.stall {
		<-ctx.Done()
	}
	return e.cause
}
func (e *failingTraceExporter) Shutdown(context.Context) error {
	e.mu.Lock()
	e.shutdowns++
	e.mu.Unlock()
	return e.cause
}

type failingMetricExporter struct {
	mu        sync.Mutex
	resources []string
	cause     *privateError
}

func (e *failingMetricExporter) Temporality(sdkmetric.InstrumentKind) metricdata.Temporality {
	return metricdata.CumulativeTemporality
}
func (e *failingMetricExporter) Aggregation(k sdkmetric.InstrumentKind) sdkmetric.Aggregation {
	return sdkmetric.DefaultAggregationSelector(k)
}
func (e *failingMetricExporter) Export(_ context.Context, data *metricdata.ResourceMetrics) error {
	e.mu.Lock()
	e.resources = append(e.resources, data.Resource.String())
	e.mu.Unlock()
	return e.cause
}
func (e *failingMetricExporter) ForceFlush(context.Context) error { return e.cause }
func (e *failingMetricExporter) Shutdown(context.Context) error   { return e.cause }

func TestExporterFailureKeepsOutcomeAndShutdownBounded(t *testing.T) {
	// Arrange: an unavailable exporter stalls until its deadline, with a hostile error.
	cause := &privateError{}
	exporter := &failingTraceExporter{cause: cause, stall: true}
	owner, observer, err := newOwned(context.Background(), ownedOptions(), exporter, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	// Act: a send finishes once before export. Flushing and shutdown use bounded contexts.
	started := time.Now()
	err = observer.Execute(
		context.Background(),
		"zalo_send_direct_message",
		func(context.Context) (domain.Outcome, error) { calls++; return domain.OutcomeUnknown, nil },
	)
	elapsed := time.Since(started)
	flush, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	flushErr := owner.ForceFlush(flush)
	cancel()
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	started = time.Now()
	shutdownErr := owner.Shutdown(cancelled)
	shutdownElapsed := time.Since(started)
	repeatErr := owner.Shutdown(cancelled)
	// Assert: failure is isolated, never formatted, and cannot turn unknown into an error.
	if calls != 1 || err != nil || elapsed > 100*time.Millisecond {
		t.Fatal("export failure blocked/changed/retried business invocation")
	}
	if flushErr == nil || shutdownErr == nil || repeatErr == nil || shutdownElapsed > 500*time.Millisecond {
		t.Fatal("export/shutdown failure not bounded/observable")
	}
	if cause.formatted.Load() != 0 ||
		strings.Contains(flushErr.Error()+shutdownErr.Error()+repeatErr.Error(), "PRIVATE_SENTINEL") {
		t.Fatal("export error leaked")
	}
	select {
	case <-owner.done:
	case <-time.After(time.Second):
		t.Fatal("owned shutdown did not terminate")
	}
	exporter.mu.Lock()
	defer exporter.mu.Unlock()
	if exporter.shutdowns != 1 {
		t.Fatal("owned exporter shutdown repeated")
	}
}

func TestDisabledPipelineDoesNotDuplicateFinalAuditOrConstructSDK(t *testing.T) {
	// Arrange.
	owner, o, err := NewOwned(context.Background(), Options{})
	if err != nil || owner != nil || o == nil || o.tracer != nil {
		t.Fatal("default created an SDK/export dependency")
	}
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)
	cause := &privateError{}
	calls := 0
	// Act.
	result := o.Execute(
		context.Background(),
		"PRIVATE_SENTINEL_TOOL",
		func(context.Context) (domain.Outcome, error) { calls++; return domain.OutcomeUnknown, cause },
	)
	_ = o.Worker(
		context.Background(),
		domain.WorkSend,
		"PRIVATE_SENTINEL_OPERATION",
		func(context.Context) error { return nil },
	)
	o.Remember(context.Background(), domain.WorkSend, "PRIVATE_SENTINEL_OPERATION")
	o.Queue(context.Background(), domain.WorkSend, 1)
	o.Transition(context.Background(), domain.WorkSend, domain.StatePending, domain.StateSent, new(time.Second))
	o.ResponseBytes(context.Background(), "PRIVATE_SENTINEL_TOOL", domain.TransportHTTP, 1)
	_ = owner.Shutdown(context.Background())
	// Assert.
	if calls != 1 || !errors.Is(result, cause) || cause.formatted.Load() != 0 {
		t.Fatal("disabled audit altered business/formatting")
	}
	if strings.Contains(logs.String(), "PRIVATE_SENTINEL") {
		t.Fatal("disabled audit leaked content")
	}
	if logs.Len() != 0 {
		t.Fatal("app execution emitted duplicate final MCP audit")
	}
}

// A fresh process is required because OTel caches resource.Default on first use.
func TestOwnedExportFiltersResourceEnvironment(t *testing.T) {
	if os.Getenv("ZL_TEST_TELEMETRY_RESOURCE_CHILD") != "1" {
		// Arrange/Act: only this synthetic test runs in a subprocess with private env fields.
		cmd := exec.Command(os.Args[0], "-test.run=^TestOwnedExportFiltersResourceEnvironment$")
		cmd.Env = append(
			os.Environ(),
			"ZL_TEST_TELEMETRY_RESOURCE_CHILD=1",
			"OTEL_RESOURCE_ATTRIBUTES=host.private=PRIVATE_SENTINEL_ENV,service.name=PRIVATE_SENTINEL_SERVICE",
		)
		output, err := cmd.CombinedOutput()
		// Assert.
		if err != nil {
			t.Fatalf("resource isolation child failed: %s", output)
		}
		if strings.Contains(string(output), "PRIVATE_SENTINEL") {
			t.Fatal("private exporter error/resource reached process diagnostics")
		}
		return
	}
	// Arrange.
	cause := &privateError{}
	te := &failingTraceExporter{cause: cause}
	me := &failingMetricExporter{cause: cause}
	owner, o, err := newOwned(context.Background(), ownedOptions(), te, me)
	if err != nil {
		t.Fatal(err)
	}
	// Act.
	_ = o.Execute(
		context.Background(),
		"zalo_join_group",
		func(context.Context) (domain.Outcome, error) { return domain.OutcomePartial, nil },
	)
	flush, cancel := context.WithTimeout(context.Background(), time.Second)
	_ = owner.ForceFlush(flush)
	cancel()
	_ = owner.Shutdown(context.Background())
	// Assert: both owned network sinks receive only static service/SDK resources.
	te.mu.Lock()
	traceResources := append([]string(nil), te.resources...)
	te.mu.Unlock()
	me.mu.Lock()
	metricResources := append([]string(nil), me.resources...)
	me.mu.Unlock()
	if len(traceResources) == 0 || len(metricResources) == 0 {
		t.Fatal("owned exporters did not receive telemetry")
	}
	for _, r := range append(traceResources, metricResources...) {
		if strings.Contains(r, "PRIVATE_SENTINEL") || strings.Contains(r, "host.private") ||
			!strings.Contains(r, "service.name=zl-mcp") {
			t.Fatal("resource environment not filtered")
		}
	}
	if cause.formatted.Load() != 0 {
		t.Fatal("owned exporter diagnostics formatted private causes")
	}
}

func TestSafeExporterPreservesFailureCategoryWithoutOriginalCause(t *testing.T) {
	// Arrange.
	original := &privateError{}
	e := safeTraceExporter{&failingTraceExporter{cause: original}}
	// Act.
	err := e.ExportSpans(context.Background(), tracetest.SpanStubs{}.Snapshots())
	// Assert.
	if err == nil || errors.Is(err, original) || err.Error() != "telemetry export failed" ||
		original.formatted.Load() != 0 {
		t.Fatal("private export cause escaped safe boundary")
	}
}

func TestMalformedResourceEnvironmentDoesNotReachDiagnostics(t *testing.T) {
	if os.Getenv("ZL_TEST_MALFORMED_RESOURCE_CHILD") == "1" {
		// Arrange: the parent supplies one malformed private resource value.
		// Act.
		owner, observer, err := NewOwned(context.Background(), ownedOptions())
		// Assert: no SDK or exporter is constructed; diagnostics remain neutral.
		if owner != nil || observer != nil || err == nil || err.Error() != "invalid telemetry resource environment" {
			t.Fatal("malformed resource did not fail neutrally before initialization")
		}
		return
	}
	for _, raw := range []string{"PRIVATE_SENTINEL_MISSING_VALUE", "key=PRIVATE_SENTINEL,", "key=PRIVATE_SENTINEL%ZZ", "key=PRIVATE_SENTINEL%"} {
		// Arrange/Act: a fresh process prevents OTel's cached default masking the bug.
		cmd := exec.Command(os.Args[0], "-test.run=^TestMalformedResourceEnvironmentDoesNotReachDiagnostics$")
		cmd.Env = append(os.Environ(), "ZL_TEST_MALFORMED_RESOURCE_CHILD=1", "OTEL_RESOURCE_ATTRIBUTES="+raw)
		output, err := cmd.CombinedOutput()
		// Assert.
		if err != nil {
			t.Fatalf("neutral resource rejection failed: %s", output)
		}
		if strings.Contains(string(output), "PRIVATE_SENTINEL") {
			t.Fatal("private malformed resource reached process diagnostics")
		}
	}
}

func TestDisabledTelemetryIgnoresMalformedResourceEnvironment(t *testing.T) {
	// Arrange.
	t.Setenv("OTEL_RESOURCE_ATTRIBUTES", "PRIVATE_SENTINEL_MISSING_VALUE")
	// Act.
	owner, observer, err := NewOwned(context.Background(), Options{})
	// Assert.
	if owner != nil || observer == nil || observer.Recording() || err != nil {
		t.Fatal("disabled telemetry changed behavior")
	}
}
