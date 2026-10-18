package events

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// Queue keeps delivery scheduling independent of Zalo and of the MCP transport.
type Queue interface {
	FanoutEvents(
		context.Context,
		time.Time,
		storage.DeliveryPolicy,
		func(string, domain.Message) ([]byte, error),
	) (int, error)
	ClaimDelivery(context.Context, time.Time, storage.DeliveryPolicy) (*storage.EventDelivery, error)
	FinishDelivery(context.Context, storage.EventDelivery, storage.DeliveryOutcome, time.Time) error
	PruneDeliveries(context.Context, time.Time) error
}

type Worker struct {
	Queue       Queue
	Client      *http.Client
	Callbacks   CallbackPolicy
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
	//nolint:exhaustruct_v5 // Worker preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
	return &Worker{
		Queue:   queue,
		Client:  NewCallbackClient(),
		Policy:  storage.DefaultDeliveryPolicy(),
		Encoder: encoder,
	}, nil
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
		delta := int64(binary.LittleEndian.Uint64(b[:])%retryJitterChoices) - retryJitterCenter
		delay += time.Duration(int64(delay) * delta / retryJitterScale)
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
	var handled bool
	err = shared.ObserveWorker(ctx, domain.WorkEvents, strconv.FormatInt(d.ID, 10), func(ctx context.Context) error {
		var operationErr error
		handled, operationErr = w.deliverClaimed(ctx, at, d)
		return operationErr
	})
	return handled, err
}
func (w *Worker) deliverClaimed(ctx context.Context, at time.Time, d *storage.EventDelivery) (bool, error) {
	return w.deliverClaimedWithFinish(ctx, at, d, w.Queue.FinishDelivery, "business", "")
}

func (w *Worker) deliverClaimedWithFinish(
	ctx context.Context,
	at time.Time,
	d *storage.EventDelivery,
	finish func(context.Context, storage.EventDelivery, storage.DeliveryOutcome, time.Time) error,
	kind, recoveryReference string,
) (bool, error) {
	created := time.Time{}
	if d.CreatedAt != nil {
		created = *d.CreatedAt
	}
	shared.ObserveTransition(ctx, domain.WorkEvents, "pending", "sending", created)
	logger := w.Logger
	if logger == nil {
		logger = slog.Default()
	}
	attrs := []any{
		"correlation_id",
		rand.Text(),
		"attempt",
		d.Attempts,
		"body_bytes",
		len(d.Payload),
		"delivery_kind",
		kind,
	}
	attrs = append(attrs, w.deliveryReferenceAttrs(d.ID, kind, recoveryReference)...)
	logger.InfoContext(ctx, "mcp_event_delivery_started", attrs...)
	if w.TraceLogger != nil {
		w.TraceLogger.InfoContext(ctx, "mcp_event_request", attrs...)
	}
	started := time.Now()
	statusCode := 0
	requestCtx, cancel := context.WithTimeout(ctx, workerdeliverClaimedTimeout)
	defer cancel()
	response, requestErr := shared.ObserveUpstream(
		requestCtx,
		domain.WorkEvents,
		func(ctx context.Context) (*http.Response, error) {
			return postCallbackWithPolicy(
				ctx,
				w.Client,
				d.Callback,
				d.Secret,
				d.SubscriptionID,
				d.EventID,
				d.Payload,
				w.Callbacks,
			)
		},
	)
	//nolint:exhaustruct_v5 // DeliveryOutcome preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
	outcome := storage.DeliveryOutcome{State: "pending", Reason: "network", RetryAt: at.Add(retryDelay(d.Attempts))}
	if requestErr == nil {
		statusCode = response.StatusCode
		if statusCode < 100 || statusCode > 599 {
			statusCode = 0
		}
		if w.TraceLogger != nil {
			w.TraceLogger.InfoContext(
				ctx,
				"mcp_event_response",
				append(append([]any{}, attrs...), "http_status", statusCode)...)
		}
		//nolint:gosec // G104: Cleanup preserves the original setup/operation error.
		response.Body.Close()
		outcome = classifyDeliveryResponse(response, at, outcome)
	}
	cancel()
	if outcome.State == "pending" && (d.Attempts >= w.Policy.MaxAttempts || !outcome.RetryAt.Before(d.Deadline)) {
		outcome.State = stateFailedText
		if d.Attempts >= w.Policy.MaxAttempts {
			outcome.Reason = "attempts"
		} else {
			outcome.Reason = "deadline"
		}
	}
	// Shutdown may have cancelled ctx after a completed HTTP attempt. Persist the
	// outcome with a short independent deadline rather than losing its receipt.
	finishCtx, finishCancel := context.WithTimeout(context.WithoutCancel(ctx), workerdeliverClaimedFinishTimeout)
	defer finishCancel()
	finishErr := finish(finishCtx, *d, outcome, time.Now().UTC())
	if finishErr == nil {
		shared.ObserveTransition(ctx, domain.WorkEvents, "sending", outcome.State, created)
	}
	attrs = append(
		attrs,
		"elapsed_ms",
		time.Since(started).Milliseconds(),
		"http_status",
		statusCode,
		"outcome",
		outcome.State,
		"reason",
		outcome.Reason,
		"receipt_persisted",
		finishErr == nil,
	)
	logger.InfoContext(ctx, "mcp_event_delivery_finished", attrs...)
	if w.TraceLogger != nil {
		w.TraceLogger.InfoContext(ctx, "mcp_event_delivery_finished", attrs...)
	}
	return true, finishErr
}

func (w *Worker) Run(ctx context.Context) error {
	if err := w.Queue.PruneDeliveries(ctx, time.Now().UTC()); err != nil {
		return err
	}
	if err := w.sweepRecovery(ctx, time.Now().UTC()); err != nil {
		return err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, workerErrorBuffer)
	done := make(chan struct{}, workerCompletionBuffer)
	for range 4 {
		go w.runDeliveryWorker(runCtx, cancel, errs, done)
	}
	defer func() {
		cancel()
		for range 4 {
			<-done
		}
	}()
	return w.runFanoutWorker(runCtx, errs)
}

func (w *Worker) fanout(ctx context.Context, at time.Time) (int, error) {
	if q, ok := w.Queue.(interface {
		FanoutProfileEvents(context.Context, time.Time, storage.DeliveryPolicy, func(string, string, domain.Message) ([]byte, error)) (int, error)
	}); ok {
		messages, err := q.FanoutProfileEvents(ctx, at, w.Policy, w.Encoder.EncodeProfile)
		if err != nil {
			return messages, err
		}
		if activities, ok := w.Queue.(interface {
			FanoutActivityEvents(context.Context, time.Time, storage.DeliveryPolicy, func(string, string, domain.Activity) ([]byte, error)) (int, error)
		}); ok {
			n, activityErr := activities.FanoutActivityEvents(ctx, at, w.Policy, w.Encoder.EncodeActivity)
			return messages + n, activityErr
		}
		return messages, nil
	}
	return w.Queue.FanoutEvents(ctx, at, w.Policy, w.Encoder.Encode)
}

func classifyDeliveryResponse(
	response *http.Response,
	at time.Time,
	outcome storage.DeliveryOutcome,
) storage.DeliveryOutcome {
	switch code := response.StatusCode; {
	case code >= 200 && code < 300:
		outcome.State, outcome.Reason = "delivered", "accepted"
	case code == http.StatusGone:
		outcome.State, outcome.Reason, outcome.Gone = stateFailedText, "callback_gone", true
	case code == http.StatusRequestEntityTooLarge:
		outcome.State, outcome.Reason = stateFailedText, "payload_too_large"
	case code == 408 || code == 429 || code >= 500:
		outcome.Reason = "http_transient"
		if code == 429 || code == 503 {
			if retry := retryAfter(response.Header.Get("Retry-After"), at); retry.After(outcome.RetryAt) {
				outcome.RetryAt = retry
			}
		}
	default:
		outcome.State, outcome.Reason = stateFailedText, "http_rejected"
	}
	return outcome
}

func (w *Worker) runDeliveryWorker(
	ctx context.Context,
	cancel context.CancelFunc,
	errs chan<- error,
	done chan<- struct{},
) {
	defer func() { done <- struct{}{} }()
	timer := time.NewTicker(deliveryPollInterval)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case at := <-timer.C:
			if _, err := w.deliverBusinessAndRecovery(ctx, at.UTC()); err != nil {
				if ctx.Err() != nil {
					return
				}
				errs <- err
				cancel()
				return
			}
		}
	}
}

func (w *Worker) runFanoutWorker(ctx context.Context, errs <-chan error) error {
	timer := time.NewTicker(deliveryPollInterval)
	defer timer.Stop()
	prune := time.NewTicker(time.Hour)
	defer prune.Stop()
	for {
		select {
		case <-ctx.Done():
			return pendingWorkerError(errs)
		case at := <-timer.C:
			if err := w.sweepRecovery(ctx, at.UTC()); err != nil {
				return err
			}
			if _, err := w.fanout(ctx, at.UTC()); err != nil {
				if ctx.Err() != nil {
					return pendingWorkerError(errs)
				}
				return err
			}
		case at := <-prune.C:
			if err := w.Queue.PruneDeliveries(ctx, at.UTC()); err != nil {
				return err
			}
		}
	}
}

func pendingWorkerError(errs <-chan error) error {
	select {
	case err := <-errs:
		return err
	default:
		return nil
	}
}

func (w *Worker) deliveryReferenceAttrs(id int64, kind, recoveryReference string) []any {
	var attrs []any
	if references, ok := w.Queue.(interface{ DeliveryReference(int64) string }); ok && kind == "business" {
		attrs = append(attrs, "delivery_reference", references.DeliveryReference(id))
	}
	if references, ok := w.Queue.(interface{ RecoveryDeliveryReference(int64) string }); ok &&
		kind == "journal_recovery" {
		attrs = append(attrs, "delivery_reference", references.RecoveryDeliveryReference(id))
	}
	if kind == "journal_recovery" && recoveryReference != "" {
		attrs = append(attrs, "recovery_reference", recoveryReference)
	}

	return attrs
}
