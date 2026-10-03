package events

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"net/http"
	"strconv"
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
	Queue   Queue
	Client  *http.Client
	Policy  storage.DeliveryPolicy
	Encoder *MessageEncoder
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
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	response, requestErr := postCallback(requestCtx, w.Client, d.Callback, d.Secret, d.SubscriptionID, d.EventID, d.Payload)
	cancel()
	outcome := storage.DeliveryOutcome{State: "pending", Reason: "network", RetryAt: at.Add(retryDelay(d.Attempts))}
	if requestErr == nil {
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
	return true, w.Queue.FinishDelivery(finishCtx, *d, outcome, time.Now().UTC())
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
