package events

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// Queue keeps delivery scheduling independent of Zalo and of the MCP transport.
type Queue interface {
	FanoutEvents(context.Context, time.Time, storage.DeliveryPolicy, func(string, domain.Message) ([]byte, error)) (int, error)
	ClaimDelivery(context.Context, time.Time, storage.DeliveryPolicy) (*storage.EventDelivery, error)
	FinishDelivery(context.Context, storage.EventDelivery, storage.DeliveryOutcome, time.Time) error
	PruneDeliveries(context.Context, time.Time) error
}

type Worker struct {
	Queue       Queue
	Client      *http.Client
	Policy      storage.DeliveryPolicy
	Encoder     *MessageEncoder
	Logger      *slog.Logger
	TraceLogger *slog.Logger
}

func NewWorker(queue Queue) (*Worker, error) {
	encoder, err := NewMessageEncoder()
	if err != nil {
		return nil, err
	}
	return &Worker{Queue: queue, Client: NewCallbackClient(), Policy: storage.DefaultDeliveryPolicy(), Encoder: encoder}, nil
}

func retryDelay(attempt int) time.Duration {
	delay := time.Second
	for i := 1; i < attempt && delay < time.Hour; i++ {
		delay *= 2
	}
	if delay > time.Hour {
		delay = time.Hour
	}
	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		// +/-20%, bounded without floating point.
		delta := int64(binary.LittleEndian.Uint64(b[:])%400001) - 200000
		delay += time.Duration(int64(delay) * delta / 1000000)
	}
	return delay
}

func retryAfter(raw string, at time.Time) time.Time {
	if seconds, err := strconv.ParseInt(raw, 10, 64); err == nil && seconds >= 0 && seconds <= 7*24*3600 {
		return at.Add(time.Duration(seconds) * time.Second)
	}
	if deadline, err := http.ParseTime(raw); err == nil && deadline.After(at) {
		return deadline
	}
	return at
}

// DeliverOne returns whether a job was handled. SQLite leases recover any request
// whose response was not committed before a process crash.
func (w *Worker) DeliverOne(ctx context.Context, at time.Time) (bool, error) {
	d, err := w.Queue.ClaimDelivery(ctx, at, w.Policy)
	if err != nil || d == nil {
		return false, err
	}
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}
	digest := sha256.Sum256(d.Payload)
	attrs := []any{"event_id", d.EventID, "subscription_id", d.SubscriptionID, "delivery_id", d.ID, "attempt", d.Attempts, "body_bytes", len(d.Payload), "body_sha256", hex.EncodeToString(digest[:])}
	logger.Info("mcp_event_delivery_started", attrs...)
	if w.TraceLogger != nil {
		w.TraceLogger.Info("mcp_event_request", append(append([]any{}, attrs...), "body", json.RawMessage(d.Payload))...)
	}
	started := time.Now()
	statusCode := 0
	requestID, openaiRequestID := "", ""
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	response, requestErr := postCallback(requestCtx, w.Client, d.Callback, d.Secret, d.SubscriptionID, d.EventID, d.Payload)
	outcome := storage.DeliveryOutcome{State: "pending", Reason: "network", RetryAt: at.Add(retryDelay(d.Attempts))}
	if requestErr == nil {
		statusCode = response.StatusCode
		requestID = safeRequestID(response.Header.Get("x-request-id"))
		openaiRequestID = safeRequestID(response.Header.Get("openai-request-id"))
		if w.TraceLogger != nil {
			raw, readErr := io.ReadAll(io.LimitReader(response.Body, 65537))
			truncated := len(raw) > 65536
			if truncated {
				raw = raw[:65536]
			}
			body := strings.ReplaceAll(string(raw), d.Secret, "[redacted]")
			body = strings.ReplaceAll(body, d.Callback, "[redacted]")
			if key, err := DecodeSigningKey(d.Secret); err == nil {
				body = strings.ReplaceAll(body, string(key), "[redacted]")
			}
			w.TraceLogger.Info("mcp_event_response", append(append([]any{}, attrs...), "http_status", statusCode, "request_id", requestID, "openai_request_id", openaiRequestID, "body", body, "body_truncated", truncated, "body_read_failed", readErr != nil)...)
		}
		response.Body.Close()
		switch code := response.StatusCode; {
		case code >= 200 && code < 300:
			outcome.State, outcome.Reason = "delivered", "accepted"
		case code == 410:
			outcome.State, outcome.Reason, outcome.Gone = "failed", "callback_gone", true
		case code == 413:
			outcome.State, outcome.Reason = "failed", "payload_too_large"
		case code == 408 || code == 429 || code >= 500:
			outcome.Reason = "http_transient"
			if code == 429 || code == 503 {
				if retry := retryAfter(response.Header.Get("Retry-After"), at); retry.After(outcome.RetryAt) {
					outcome.RetryAt = retry
				}
			}
		default:
			outcome.State, outcome.Reason = "failed", "http_rejected"
		}
	}
	cancel()
	if outcome.State == "pending" && (d.Attempts >= w.Policy.MaxAttempts || !outcome.RetryAt.Before(d.Deadline)) {
		outcome.State = "failed"
		if d.Attempts >= w.Policy.MaxAttempts {
			outcome.Reason = "attempts"
		} else {
			outcome.Reason = "deadline"
		}
	}
	// Shutdown may have cancelled ctx after a completed HTTP attempt. Persist the
	// outcome with a short independent deadline rather than losing its receipt.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer finishCancel()
	finishErr := w.Queue.FinishDelivery(finishCtx, *d, outcome, time.Now().UTC())
	attrs = append(attrs, "elapsed_ms", time.Since(started).Milliseconds(), "http_status", statusCode, "request_id", requestID, "openai_request_id", openaiRequestID, "outcome", outcome.State, "reason", outcome.Reason, "receipt_persisted", finishErr == nil)
	logger.Info("mcp_event_delivery_finished", attrs...)
	if w.TraceLogger != nil {
		w.TraceLogger.Info("mcp_event_delivery_finished", attrs...)
	}
	return true, finishErr
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.Queue.PruneDeliveries(ctx, time.Now().UTC()); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, 5)
	done := make(chan struct{}, 4)
	for range 4 {
		go func() {
			defer func() { done <- struct{}{} }()
			timer := time.NewTicker(250 * time.Millisecond)
			defer timer.Stop()
			for {
				select {
				case <-runCtx.Done():
					return
				case at := <-timer.C:
					if _, err := w.DeliverOne(runCtx, at.UTC()); err != nil {
						if runCtx.Err() != nil {
							return
						}
						errs <- err
						cancel()
						return
					}
				}
			}
		}()
	}
	defer func() {
		cancel()
		for range 4 {
			<-done
		}
	}()
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	for {
		select {
		case <-runCtx.Done():
			select {
			case err := <-errs:
				return err
			default:
				return nil
			}
		case at := <-timer.C:
			if _, err := w.fanout(runCtx, at.UTC()); err != nil {
				if runCtx.Err() != nil {
					select {
					case err := <-errs:
						return err
					default:
						return nil
					}
				}
				return err
			}
		case at := <-prune.C:
			if err := w.Queue.PruneDeliveries(runCtx, at.UTC()); err != nil {
				return err
			}
		}
	}
}

func (w *Worker) fanout(ctx context.Context, at time.Time) (int, error) {
	if q, ok := w.Queue.(interface {
		FanoutProfileEvents(context.Context, time.Time, storage.DeliveryPolicy, func(string, string, domain.Message) ([]byte, error)) (int, error)
	}); ok {
		return q.FanoutProfileEvents(ctx, at, w.Policy, w.Encoder.EncodeProfile)
	}
	return w.Queue.FanoutEvents(ctx, at, w.Policy, w.Encoder.Encode)
}

// safeRequestID prevents arbitrary receiver content from entering diagnostic logs.
func safeRequestID(value string) string {
	if len(value) > 128 {
		return ""
	}
	for _, ch := range value {
		if !(ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9' || ch == '-' || ch == '_' || ch == '.') {
			return ""
		}
	}
	return value
}
