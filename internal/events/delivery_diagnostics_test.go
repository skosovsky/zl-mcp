package events

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestDeliveryDiagnosticReferenceLinksRetriesToJournal(t *testing.T) {
	// Arrange: one transient callback failure followed by successful receipt.
	var logs bytes.Buffer
	attempts := 0
	w, store, at := readyWorker(t, callbackRoundTrip(func(_ *http.Request) (*http.Response, error) {
		attempts++
		code := http.StatusServiceUnavailable
		if attempts == 2 {
			code = http.StatusAccepted
		}
		return &http.Response{StatusCode: code, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
	}))
	w.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
	// Act: advance scheduling time without sleeping or contacting Zalo.
	for _, when := range []time.Time{at, at.Add(time.Hour)} {
		if handled, err := w.DeliverOne(context.Background(), when); err != nil || !handled {
			t.Fatal(handled, err)
		}
	}
	// Assert: references identify the same durable delivery, attempts are distinct.
	var refs, ids []string
	for line := range strings.SplitSeq(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry["msg"] == "mcp_event_delivery_started" {
			refs = append(refs, entry["delivery_reference"].(string))
			ids = append(ids, entry["correlation_id"].(string))
		}
	}
	if len(refs) != 2 || refs[0] != refs[1] || ids[0] == ids[1] {
		t.Fatal(refs, ids)
	}
	var row int64
	if err := store.DB.QueryRow("SELECT id FROM event_deliveries LIMIT 1").Scan(&row); err != nil {
		t.Fatal(err)
	}
	if store.DeliveryReference(row) != refs[0] {
		t.Fatal("log reference cannot be matched to journal")
	}
}
