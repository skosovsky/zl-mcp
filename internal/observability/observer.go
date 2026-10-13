// Package observability owns the bounded, content-free telemetry boundary.
package observability

import (
	"context"
	"errors"
	"slices"
	"sync"
	"time"

	"github.com/skosovsky/metry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

// Names are a fixed allowlist: neither caller input nor native identities can
// introduce a telemetry dimension. Unknown capabilities collapse to other.
//
//nolint:gochecknoglobals // Private immutable capability allowlist bounds telemetry dimensions.
var toolNames = []string{
	"zalo_search_messages", "zalo_search_conversation_messages",
	"zalo_get_message_context", "zalo_get_conversation_message_context",
	"zalo_get_status", "zalo_list_groups", "zalo_get_group",
	"zalo_get_conversation", "zalo_list_conversations", "zalo_list_conversation_messages",
	"zalo_list_event_subscriptions", "zalo_read_subscription_events", "zalo_list_archive_sources",
	"zalo_get_send_status", "zalo_get_history_import_status", "zalo_get_join_status", "zalo_inspect_invite",
	"zalo_send_direct_message", "zalo_join_group", "zalo_import_conversation_history",
	"zalo_cancel_history_import", "zalo_ack_subscription_events",
}

const traceCacheLimit = 4096
const traceCacheTTL = 24 * time.Hour

type traceKey struct {
	kind domain.WorkKind
	id   string
}
type traceReference struct {
	span    trace.SpanContext
	expires time.Time
}

type Observer struct {
	tracer      trace.Tracer
	executors   map[string]func(context.Context, shared.Invocation) (domain.Outcome, error)
	calls       metric.Int64Counter
	response    metric.Int64Histogram
	upstream    metric.Int64Counter
	transitions metric.Int64Counter
	queue       metric.Int64Gauge
	age         metric.Float64Histogram
	mu          sync.Mutex
	references  map[traceKey]traceReference
	now         func() time.Time
}

var _ shared.TelemetryPort = (*Observer)(nil)

// New borrows the service's single provider. The service retains shutdown.
// A nil provider disables instrumentation without constructing ExecutorWrap.
func New(provider *metry.Provider) (*Observer, error) {
	if provider == nil {
		//nolint:nilnil // A nil provider disables instrumentation without constructing a new owner.
		return nil, nil
	}
	sdk := provider.SDK()
	//nolint:exhaustruct_v5 // Observer initializes only this owner/phase; optional collaborators, buffers and counters keep their zero defaults.
	o := &Observer{
		tracer:     sdk.TracerProvider.Tracer("zl-mcp/host"),
		executors:  make(map[string]func(context.Context, shared.Invocation) (domain.Outcome, error)),
		references: make(map[traceKey]traceReference),
		now:        time.Now,
	}
	meter := sdk.MeterProvider.Meter("zl-mcp/host")
	var err error
	if o.calls, err = meter.Int64Counter("zl_mcp_domain_calls", metric.WithUnit("calls")); err != nil {
		return nil, errors.New("telemetry instrument configuration failed")
	}
	if o.response, err = meter.Int64Histogram("zl_mcp_response_bytes", metric.WithUnit("By")); err != nil {
		return nil, errors.New("telemetry instrument configuration failed")
	}
	if o.upstream, err = meter.Int64Counter("zl_mcp_upstream_calls", metric.WithUnit("calls")); err != nil {
		return nil, errors.New("telemetry instrument configuration failed")
	}
	if o.transitions, err = meter.Int64Counter(
		"zl_mcp_operation_transitions",
		metric.WithUnit("transitions"),
	); err != nil {
		return nil, errors.New("telemetry instrument configuration failed")
	}
	if o.queue, err = meter.Int64Gauge("zl_mcp_worker_queue_depth", metric.WithUnit("operations")); err != nil {
		return nil, errors.New("telemetry instrument configuration failed")
	}
	if o.age, err = meter.Float64Histogram("zl_mcp_operation_age_seconds", metric.WithUnit("s")); err != nil {
		return nil, errors.New("telemetry instrument configuration failed")
	}
	for _, name := range append(append([]string(nil), toolNames...), "other") {
		o.executors[name] = metry.ExecutorWrap(
			provider,
			name,
			func(ctx context.Context, next shared.Invocation) (domain.Outcome, error) { return next(ctx) },
		)
	}
	return o, nil
}

func (o *Observer) tool(name string) string {
	if o.executors == nil {
		if slices.Contains(toolNames, name) {
			return name
		}
		return "other"
	}
	if _, ok := o.executors[name]; ok {
		return name
	}
	return "other"
}
func errorOutcome(err error, outcome domain.Outcome) domain.Outcome {
	// Do not invoke arbitrary Is/Unwrap callbacks merely to observe an error.
	//nolint:errorlint // Observe the exact sentinel without calling untrusted Is or Unwrap callbacks.
	if err == context.Canceled {
		return domain.OutcomeCancelled
	}
	if err != nil {
		return domain.OutcomeError
	}
	return outcome
}

func (o *Observer) Execute(ctx context.Context, name string, next shared.Invocation) error {
	var err error
	if o == nil {
		_, err = next(ctx)
		return err
	}
	name = o.tool(name)
	if o.tracer == nil {
		outcome := domain.OutcomeUnknown
		defer func() {
			p := recover()
			if p != nil {
				outcome = domain.OutcomePanic
			}
			if p != nil {
				panic(p)
			}
		}()
		outcome, err = next(ctx)
		outcome = errorOutcome(err, outcome)
		return err
	}
	//nolint:spancheck // The deferred recovery closure sets outcome and calls End before rethrowing panic.
	ctx, span := o.tracer.Start(ctx, "zl_mcp.request", trace.WithAttributes(attribute.String("tool", name)))
	outcome := domain.OutcomeUnknown
	defer func() {
		p := recover()
		if p != nil {
			outcome = domain.OutcomePanic
		}
		label := outcome.String()
		o.calls.Add(ctx, 1, metric.WithAttributes(attribute.String("tool", name), attribute.String("outcome", label)))
		span.SetAttributes(attribute.String("outcome", label))
		span.End()
		if p != nil {
			panic(p)
		}
	}()
	outcome, err = o.executors[name](ctx, next)
	outcome = errorOutcome(err, outcome)
	//nolint:spancheck // The deferred recovery closure sets outcome and calls End before rethrowing panic.
	return err
}

func (o *Observer) ResponseBytes(ctx context.Context, tool string, transport domain.Transport, size int64) {
	if o == nil || o.tracer == nil || size < 0 {
		return
	}
	o.response.Record(
		ctx,
		size,
		metric.WithAttributes(
			attribute.String("tool", o.tool(tool)),
			attribute.String("transport", transport.String()),
		),
	)
}
func (o *Observer) Upstream(ctx context.Context, kind domain.WorkKind, next func(context.Context) error) error {
	var err error
	if o == nil || o.tracer == nil {
		return next(ctx)
	}
	//nolint:spancheck // The deferred recovery closure sets outcome and calls End before rethrowing panic.
	ctx, span := o.tracer.Start(ctx, "zl_mcp.upstream", trace.WithAttributes(attribute.String("kind", kind.String())))
	outcome := domain.OutcomeUnknown
	defer func() {
		p := recover()
		if p != nil {
			outcome = domain.OutcomePanic
		}
		label := outcome.String()
		o.upstream.Add(
			ctx,
			1,
			metric.WithAttributes(attribute.String("kind", kind.String()), attribute.String("outcome", label)),
		)
		span.SetAttributes(attribute.String("outcome", label))
		span.End()
		if p != nil {
			panic(p)
		}
	}()
	err = next(ctx)
	outcome = errorOutcome(err, domain.OutcomeSuccess)
	//nolint:spancheck // The deferred recovery closure sets outcome and calls End before rethrowing panic.
	return err
}

func (o *Observer) Transition(
	ctx context.Context,
	kind domain.WorkKind,
	from, to domain.OperationState,
	age *time.Duration,
) {
	if o == nil || o.tracer == nil {
		return
	}
	o.transitions.Add(
		ctx,
		1,
		metric.WithAttributes(
			attribute.String("kind", kind.String()),
			attribute.String("from", from.String()),
			attribute.String("to", to.String()),
		),
	)
	if age != nil {
		value := max(*age, time.Duration(0))
		o.age.Record(
			ctx,
			value.Seconds(),
			metric.WithAttributes(attribute.String("kind", kind.String()), attribute.String("state", to.String())),
		)
	}
}
func (o *Observer) Queue(ctx context.Context, kind domain.WorkKind, depth int64) {
	if o == nil || o.tracer == nil || kind > domain.WorkEvents || depth < 0 {
		return
	}
	o.queue.Record(ctx, depth, metric.WithAttributes(attribute.String("queue", kind.String())))
}
func (o *Observer) expire(now time.Time) {
	for key, value := range o.references {
		if !now.Before(value.expires) {
			delete(o.references, key)
		}
	}
}
func (o *Observer) Remember(ctx context.Context, kind domain.WorkKind, id string) {
	if o == nil || o.tracer == nil || id == "" || len(id) > 2048 || kind > domain.WorkEvents {
		return
	}
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	now := o.now()
	o.expire(now)
	key := traceKey{kind, id}
	if _, exists := o.references[key]; !exists && len(o.references) >= traceCacheLimit {
		var oldest traceKey
		var expiry time.Time
		for candidate, value := range o.references {
			if expiry.IsZero() || value.expires.Before(expiry) {
				oldest, expiry = candidate, value.expires
			}
		}
		delete(o.references, oldest)
	}
	o.references[key] = traceReference{sc, now.Add(traceCacheTTL)}
}

func (o *Observer) Worker(
	ctx context.Context,
	kind domain.WorkKind,
	id string,
	next func(context.Context) error,
) error {
	var err error
	if o == nil || o.tracer == nil {
		return next(ctx)
	}
	options := []trace.SpanStartOption{trace.WithAttributes(attribute.String("kind", kind.String()))}
	o.mu.Lock()
	o.expire(o.now())
	ref, ok := o.references[traceKey{kind, id}]
	o.mu.Unlock()
	if ok {
		//nolint:exhaustruct_v5 // Only the span context is linked; optional link attributes remain absent.
		options = append(options, trace.WithLinks(trace.Link{SpanContext: ref.span}))
	}
	//nolint:spancheck // The deferred recovery closure sets outcome and calls End before rethrowing panic.
	ctx, span := o.tracer.Start(ctx, "zl_mcp.worker", options...)
	outcome := domain.OutcomeUnknown
	defer func() {
		p := recover()
		if p != nil {
			outcome = domain.OutcomePanic
		}
		span.SetAttributes(attribute.String("outcome", outcome.String()))
		span.End()
		if p != nil {
			panic(p)
		}
	}()
	err = next(ctx)
	outcome = errorOutcome(err, domain.OutcomeSuccess)
	//nolint:spancheck // The deferred recovery closure sets outcome and calls End before rethrowing panic.
	return err
}

func (o *Observer) Recording() bool { return o != nil && o.tracer != nil }
