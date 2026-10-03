package websocketx

import (
	"context"
	"testing"
	"time"
)

func TestFrameBackpressurePreservesBurst(t *testing.T) {
	// Arrange: use the same bounded channel as the socket read loop.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c := &client{connCtx: ctx, done: make(chan struct{}), msgChan: make(chan Message, 1)}
	done := make(chan struct{})
	// Act.
	go func() {
		defer close(done)
		for n := range 1000 {
			c.handleMsg(Message{Data: []byte{byte(n >> 8), byte(n)}})
		}
	}()
	// Assert: no frames are evicted by the raw transport queue.
	for want := range 1000 {
		select {
		case msg := <-c.msgChan:
			got := int(msg.Data[0])<<8 | int(msg.Data[1])
			if got != want {
				t.Fatalf("received frame %d, want %d", got, want)
			}
		case <-ctx.Done():
			t.Fatal("raw frame stream stalled")
		}
	}
	<-done
}

func TestFrameBackpressureReleasesOnClose(t *testing.T) {
	// Arrange: a full queue and a closed socket release the reader.
	c := &client{connCtx: context.Background(), done: make(chan struct{}), msgChan: make(chan Message, 1)}
	c.msgChan <- Message{Data: []byte("first")}
	close(c.done)
	// Act.
	c.handleMsg(Message{Data: []byte("second")})
	// Assert.
	if string((<-c.msgChan).Data) != "first" {
		t.Fatal("queued frame overwritten")
	}
}
