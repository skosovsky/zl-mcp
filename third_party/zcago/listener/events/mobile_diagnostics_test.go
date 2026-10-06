package events

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"
)

func TestMobileDecodeFailureLogsReasonWithoutPayload(t *testing.T) {
	// Arrange: malformed metadata includes private source fields.
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	payload := []byte(`{"public_key":"private-public","url":"https://private-url.invalid","encrypted_key":"private-key"}`)
	// Act.
	event, err := decodeMobileSync("syncmsg_info", payload)
	// Assert: diagnostics identify the rejected field group with no source values.
	if event != nil || !errors.Is(err, ErrMobileSyncControl) || !strings.Contains(output.String(), "ACCOUNT_ID") || strings.Contains(output.String(), "private-") {
		t.Fatal("unsafe or missing decode diagnostic")
	}
}
