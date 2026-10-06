package zalo

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/amrakk/zcago/model"
)

func TestMobileControlDiagnosticsExcludePrivateFields(t *testing.T) {
	// Arrange: selected scalar fields coexist with private transport values.
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	status, code, action := 0, 7, 1
	event := model.MobileSyncEvent{Action: "transfer_error", PublicKey: "private-public-key", PCName: "private-name", UID: "private-user", URL: "https://private-url.invalid/key", EncryptedKey: "private-key", DatabaseInfo: "private-db", Status: &status, ErrorCode: &code, UserAction: &action}
	// Act.
	logMobileControl(event, "private-public-key", "private-user")
	// Assert: diagnostics retain statuses and presence but none of their private values.
	logs := output.String()
	for _, secret := range []string{event.PublicKey, event.PCName, event.UID, event.URL, event.EncryptedKey, event.DatabaseInfo} {
		if strings.Contains(logs, secret) {
			t.Fatal("private control value leaked")
		}
	}
	if !strings.Contains(logs, `"status":0`) || !strings.Contains(logs, `"upstream_error_code":7`) || !strings.Contains(logs, `"key_match":true`) {
		t.Fatal("selected diagnostics lost")
	}
}

func TestUnknownMobileActionIsNotLoggedVerbatim(t *testing.T) {
	// Arrange.
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	// Act.
	logMobileControl(model.MobileSyncEvent{Action: "private-source-action"}, "key", "owner")
	// Assert.
	if strings.Contains(output.String(), "private-source-action") || !strings.Contains(output.String(), `"action":"unknown"`) {
		t.Fatal("unsafe action category")
	}
}
