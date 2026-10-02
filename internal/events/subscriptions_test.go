package events

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestCancellationDuringVerificationPreventsActivation(t *testing.T) {
	// Arrange: hold the receiver's challenge reply until cancellation completes.
	ctx := context.Background()
	store, err := storage.Open(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), []string{"group"}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	manager, err := NewSubscriptionManager(store, "synthetic")
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	manager.Client = &http.Client{Transport: callbackRoundTrip(func(r *http.Request) (*http.Response, error) {
		var challenge map[string]string
		if err := json.NewDecoder(r.Body).Decode(&challenge); err != nil {
			return nil, err
		}
		close(entered)
		select {
		case <-release:
		case <-r.Context().Done():
			return nil, r.Context().Err()
		}
		wire, err := json.Marshal(map[string]string{"challenge": challenge["challenge"]})
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(string(wire)))}, nil
	})}
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte(strings.Repeat("x", 32)))
	subscribe, err := json.Marshal(map[string]any{"name": MessageCreated, "arguments": map[string]any{"group_id": "group"}, "delivery": map[string]any{"mode": "webhook", "url": "https://callback.example/events", "secret": secret}, "ttlMs": nil})
	if err != nil {
		t.Fatal(err)
	}
	unsubscribe, err := json.Marshal(map[string]any{"name": MessageCreated, "arguments": map[string]any{"group_id": "group"}, "delivery": map[string]any{"mode": "webhook", "url": "https://callback.example/events"}})
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, err := manager.Call(ctx, "events/subscribe", "owner", subscribe); result <- err }()
	<-entered
	// Act: cancel before the callback verification succeeds.
	_, cancelErr := manager.Call(ctx, "events/unsubscribe", "owner", unsubscribe)
	close(release)
	activationErr := <-result
	// Assert: verification may succeed, but durable cancellation wins.
	if cancelErr != nil {
		t.Fatal(cancelErr)
	}
	var rpc *RPCError
	if !errors.As(activationErr, &rpc) || rpc.Data["reason"] != "cancelled" {
		t.Fatalf("activation error=%v", activationErr)
	}
	var active int
	if err := store.DB.QueryRow("SELECT COUNT(*) FROM event_subscriptions WHERE active=1").Scan(&active); err != nil {
		t.Fatal(err)
	}
	if active != 0 {
		t.Fatalf("active subscriptions=%d", active)
	}
}
