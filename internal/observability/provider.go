package observability

import (
	"context"
	"errors"
	"net"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/skosovsky/metry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// Options are composition-owned configuration, never business DTOs.
type Options struct {
	Enabled         bool
	Exporter        string
	Endpoint        string
	Headers         map[string]string
	ExportTimeout   time.Duration
	ShutdownTimeout time.Duration
}

// Owner owns exactly one metry.New pipeline. Observer only borrows it.
type Owner struct {
	provider        *metry.Provider
	shutdownTimeout time.Duration
	once            sync.Once
	done            chan struct{}
	shutdownErr     error
}

func NewOwned(ctx context.Context, options Options) (*Owner, *Observer, error) {
	if !options.Enabled {
		return nil, &Observer{}, nil
	}
	if err := validateResourceEnvironment(); err != nil {
		return nil, nil, err
	}
	if options.ExportTimeout < 100*time.Millisecond || options.ExportTimeout > 5*time.Second || options.ShutdownTimeout < 100*time.Millisecond || options.ShutdownTimeout > 5*time.Second {
		return nil, nil, errors.New("invalid telemetry deadlines")
	}
	switch options.Exporter {
	case "none":
		if options.Endpoint != "" || len(options.Headers) != 0 {
			return nil, nil, errors.New("invalid disabled exporter configuration")
		}
		return newOwned(ctx, options, nil, nil)
	case "otlp_http":
		endpoint, err := url.Parse(options.Endpoint)
		if err != nil || endpoint.Hostname() == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
			return nil, nil, errors.New("invalid telemetry endpoint")
		}
		if endpoint.Scheme != "https" && (endpoint.Scheme != "http" || net.ParseIP(endpoint.Hostname()) == nil || !net.ParseIP(endpoint.Hostname()).IsLoopback()) {
			return nil, nil, errors.New("invalid telemetry endpoint")
		}
		for key, value := range options.Headers {
			if strings.ContainsAny(key+value, "\r\n\x00") {
				return nil, nil, errors.New("invalid telemetry headers")
			}
		}
		traceExporter, err := otlptracehttp.New(ctx, otlptracehttp.WithEndpointURL(options.Endpoint), otlptracehttp.WithHeaders(options.Headers), otlptracehttp.WithTimeout(options.ExportTimeout), otlptracehttp.WithRetry(otlptracehttp.RetryConfig{Enabled: false}))
		if err != nil {
			return nil, nil, errors.New("telemetry exporter initialization failed")
		}
		metricExporter, err := otlpmetrichttp.New(ctx, otlpmetrichttp.WithEndpointURL(options.Endpoint), otlpmetrichttp.WithHeaders(options.Headers), otlpmetrichttp.WithTimeout(options.ExportTimeout), otlpmetrichttp.WithRetry(otlpmetrichttp.RetryConfig{Enabled: false}))
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), options.ShutdownTimeout)
			defer cancel()
			_ = traceExporter.Shutdown(cleanup)
			return nil, nil, errors.New("telemetry exporter initialization failed")
		}
		return newOwned(ctx, options, traceExporter, metricExporter)
	default:
		return nil, nil, errors.New("invalid telemetry exporter")
	}
}
func newOwned(ctx context.Context, options Options, traceExporter sdktrace.SpanExporter, metricExporter sdkmetric.Exporter) (*Owner, *Observer, error) {
	if err := validateResourceEnvironment(); err != nil {
		return nil, nil, err
	}
	opts := []metry.Option{metry.WithServiceName("zl-mcp")}
	if traceExporter != nil {
		opts = append(opts, metry.WithExporter(safeTraceExporter{traceExporter}))
	}
	if metricExporter != nil {
		opts = append(opts, metry.WithMetricExporter(safeMetricExporter{metricExporter}))
	}
	p, err := metry.New(ctx, opts...)
	if err != nil {
		return nil, nil, errors.New("telemetry provider initialization failed")
	}
	owner := &Owner{provider: p, shutdownTimeout: options.ShutdownTimeout, done: make(chan struct{})}
	observer, err := New(p)
	if err != nil {
		_ = owner.Shutdown(ctx)
		return nil, nil, err
	}
	return owner, observer, nil
}

// Shutdown uses one fresh bounded shutdown attempt, after workers have joined.
// The caller's cancelled service context cannot prevent flushing owned buffers.
func (o *Owner) Shutdown(ctx context.Context) error {
	if o == nil {
		return nil
	}
	o.once.Do(func() {
		shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), o.shutdownTimeout)
		go func() { defer cancel(); o.shutdownErr = neutral(o.provider.Shutdown(shutdown)); close(o.done) }()
	})
	timer := time.NewTimer(o.shutdownTimeout)
	defer timer.Stop()
	select {
	case <-o.done:
		return o.shutdownErr
	case <-timer.C:
		return errors.New("telemetry shutdown deadline exceeded")
	}
}
func (o *Owner) ForceFlush(ctx context.Context) error {
	if o == nil {
		return nil
	}
	return neutral(o.provider.ForceFlush(ctx))
}
func neutral(err error) error {
	if err != nil {
		return errors.New("telemetry export failed")
	}
	return nil
}
func safeResource() *resource.Resource {
	return resource.NewSchemaless(attribute.String("service.name", "zl-mcp"), attribute.String("telemetry.sdk.language", "go"))
}

type safeSpan struct{ sdktrace.ReadOnlySpan }

func (safeSpan) Resource() *resource.Resource { return safeResource() }

type safeTraceExporter struct{ sdktrace.SpanExporter }

func (e safeTraceExporter) ExportSpans(ctx context.Context, spans []sdktrace.ReadOnlySpan) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("telemetry export failed")
		} else {
			err = neutral(err)
		}
	}()
	safe := make([]sdktrace.ReadOnlySpan, len(spans))
	for i, s := range spans {
		safe[i] = safeSpan{s}
	}
	return e.SpanExporter.ExportSpans(ctx, safe)
}
func (e safeTraceExporter) Shutdown(ctx context.Context) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("telemetry shutdown failed")
		} else {
			err = neutral(err)
		}
	}()
	return e.SpanExporter.Shutdown(ctx)
}

type safeMetricExporter struct{ sdkmetric.Exporter }

func (e safeMetricExporter) Export(ctx context.Context, data *metricdata.ResourceMetrics) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("telemetry export failed")
		} else {
			err = neutral(err)
		}
	}()
	safe := *data
	safe.Resource = safeResource()
	return e.Exporter.Export(ctx, &safe)
}
func (e safeMetricExporter) ForceFlush(ctx context.Context) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("telemetry flush failed")
		} else {
			err = neutral(err)
		}
	}()
	return e.Exporter.ForceFlush(ctx)
}
func (e safeMetricExporter) Shutdown(ctx context.Context) (err error) {
	defer func() {
		if recover() != nil {
			err = errors.New("telemetry shutdown failed")
		} else {
			err = neutral(err)
		}
	}()
	return e.Exporter.Shutdown(ctx)
}

// The pinned OTel detector logs malformed entries before exporter filtering.
// Validate locally without formatting private values or changing global state.
func validateResourceEnvironment() error {
	raw := strings.TrimSpace(os.Getenv("OTEL_RESOURCE_ATTRIBUTES"))
	if raw == "" {
		return nil
	}
	for _, pair := range strings.Split(raw, ",") {
		_, value, found := strings.Cut(pair, "=")
		if !found {
			return errors.New("invalid telemetry resource environment")
		}
		if _, err := url.PathUnescape(strings.TrimSpace(value)); err != nil {
			return errors.New("invalid telemetry resource environment")
		}
	}
	return nil
}
