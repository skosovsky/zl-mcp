package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type queueObservation struct {
	recording bool
	values    chan domain.WorkKind
}

func (o *queueObservation) Recording() bool { return o.recording }
func (*queueObservation) Execute(ctx context.Context, _ string, next shared.Invocation) error {
	_, err := next(ctx)
	return err
}
func (*queueObservation) ResponseBytes(context.Context, string, domain.Transport, int64) {}
func (*queueObservation) Upstream(ctx context.Context, _ domain.WorkKind, next func(context.Context) error) error {
	return next(ctx)
}
func (*queueObservation) Transition(context.Context, domain.WorkKind, domain.OperationState, domain.OperationState, *time.Duration) {
}
func (o *queueObservation) Queue(_ context.Context, kind domain.WorkKind, _ int64) { o.values <- kind }
func (*queueObservation) Remember(context.Context, domain.WorkKind, string)        {}
func (*queueObservation) Worker(ctx context.Context, _ domain.WorkKind, _ string, next func(context.Context) error) error {
	return next(ctx)
}

type observedQueueOwner struct {
	reads   atomic.Int64
	bounded atomic.Bool
	failure bool
}

func (o *observedQueueOwner) ObservationQueueDepth(ctx context.Context, kind domain.WorkKind) (int64, error) {
	o.reads.Add(1)
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 110*time.Millisecond {
		o.bounded.Store(false)
	}
	if o.failure {
		return 0, errors.New("PRIVATE_SENTINEL_QUEUE_FAILURE")
	}
	return int64(kind) + 1, nil
}
func TestNativeQueueSamplerUsesBoundedBorrowedReadsAndJoins(t *testing.T) {
	// Arrange: the recording observer and native query port are distinct owners.
	observer := &queueObservation{recording: true, values: make(chan domain.WorkKind, 16)}
	owner := &observedQueueOwner{}
	owner.bounded.Store(true)
	ctx, cancel := context.WithCancel(shared.WithTelemetry(context.Background(), observer))
	defer cancel()
	done := make(chan error, 1)
	// Act: service-owned sampler observes all4queues and exits on service cancellation.
	go func() { done <- observeNativeQueues(ctx, owner) }()
	found := map[domain.WorkKind]bool{}
	for range 4 {
		select {
		case kind := <-observer.values:
			found[kind] = true
		case <-time.After(time.Second):
			t.Fatal("sampler did not observe queues")
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("sampler did not join")
	}
	// Assert: no command-path query/cumulative counter; every read is bounded.
	if len(found) != 4 || owner.reads.Load() != 4 || !owner.bounded.Load() {
		t.Fatal("sampler queues/deadlines incorrect")
	}
}
func TestDisabledSamplerAndReadFailuresHaveNoBusinessEffect(t *testing.T) {
	// Arrange.
	observer := &queueObservation{values: make(chan domain.WorkKind, 16)}
	owner := &observedQueueOwner{failure: true}
	owner.bounded.Store(true)
	ctx := shared.WithTelemetry(context.Background(), observer)
	// Act: disabled telemetry never touches native state. Failed enabled reads skip samples.
	sampleNativeQueues(ctx, owner)
	if owner.reads.Load() != 0 {
		t.Fatal("disabled telemetry read the native ledgers")
	}
	observer.recording = true
	sampleNativeQueues(ctx, owner)
	// Assert.
	if owner.reads.Load() != 4 || len(observer.values) != 0 || !owner.bounded.Load() {
		t.Fatal("failed observation manufactured queue samples")
	}
}
