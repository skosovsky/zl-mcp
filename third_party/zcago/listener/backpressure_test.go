package listener

import (
	"context"
	"testing"
	"time"
)

func TestDurableEmissionPreservesBurstOrder(t *testing.T) {
	// Arrange: a burst larger than the bounded consumer buffer.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ch := make(chan int, 1)
	var counters diagnosticCounters
	done := make(chan bool, 1)
	// Act: producers must wait for consumption without evicting older values.
	go func() {
		for n := range 1000 {
			if !emitDurable(ctx, ch, n, &counters) {
				done <- false
				return
			}
		}
		done <- true
	}()
	// Assert: every value reaches the receiver in its original order.
	for want := range 1000 {
		select {
		case got := <-ch:
			if got != want {
				t.Fatalf("received %d, want %d", got, want)
			}
		case <-ctx.Done():
			t.Fatal("producer or consumer stalled")
		}
	}
	if !<-done {
		t.Fatal("burst was cancelled")
	}
}

func TestDurableEmissionCancellationRetainsQueuedValue(t *testing.T) {
	// Arrange: no receiver is available and the queue already holds one value.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	ch := make(chan int, 1)
	ch <- 1
	var counters diagnosticCounters
	// Act: cancellation releases the waiting producer.
	ok := emitDurable(ctx, ch, 2, &counters)
	// Assert: overflow is visible and does not overwrite the existing message.
	if ok || <-ch != 1 || counters.backpressure.Load() != 1 || counters.cancelledEmissions.Load() != 1 {
		t.Fatal("incorrect cancellation or hidden eviction")
	}
}
