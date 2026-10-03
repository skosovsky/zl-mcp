package listener

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/amrakk/zcago/session"
	"github.com/coder/websocket"

	"github.com/amrakk/zcago/internal/websocketx"
)

type stopTestSocket struct {
	websocketx.Client
	closed bool
}

func (s *stopTestSocket) Close(int, string) { s.closed = true }

func TestStopReleasesClientAfterCancelledWorker(t *testing.T) {
	// Arrange: cancellation wins before the worker consumes a close event.
	ctx, cancel := context.WithCancel(context.Background())
	socket := &stopTestSocket{}
	ln := &listener{client: socket, cancel: cancel, cipherKey: "synthetic", reqID: 7}
	ln.wg.Add(1)
	go func() { defer ln.wg.Done(); <-ctx.Done() }()
	// Act: cleanup must release the client even without a close notification.
	ln.Stop()
	// Assert: a subsequent Start is no longer rejected as already started.
	if !socket.closed || ln.getClient() != nil || ln.cipherKey != "" || ln.reqID != 0 {
		t.Fatal("cancelled listener retained connection state")
	}
	if err := ln.Start(ctx, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start did not reach context validation: %v", err)
	}
	ln.Stop() // Cleanup remains idempotent.
}

func TestListenerReconnectsAfterConsumerStopsOnError(t *testing.T) {
	// Arrange: a real loopback websocket, without account credentials or payloads.
	var connections atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		connections.Add(1)
		_, _, _ = conn.Read(r.Context())
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ln := &listener{
		sc:        session.NewContext(session.WithHTTPClient(server.Client())),
		ch:        initializeChannels(),
		wsURL:     "ws" + strings.TrimPrefix(server.URL, "http"),
		userAgent: "synthetic-listener-test",
	}
	defer ln.Stop()
	// Act: mimic the application stopping after a listener parsing error, twice.
	for attempt := 0; attempt < 2; attempt++ {
		if err := ln.Start(ctx, false); err != nil {
			t.Fatalf("start %d: %v", attempt, err)
		}
		select {
		case <-ln.Connected():
		case <-ctx.Done():
			t.Fatal("connection did not become ready")
		}
		ln.emitError(ctx, errors.New("synthetic parsing failure"))
		select {
		case <-ln.Error():
		case <-ctx.Done():
			t.Fatal("error did not reach consumer")
		}
		ln.Stop()
	}
	// Assert: recovery performed another actual handshake on the same listener.
	if connections.Load() != 2 || ln.getClient() != nil {
		t.Fatal("listener failed to reopen and release its websocket")
	}
}
