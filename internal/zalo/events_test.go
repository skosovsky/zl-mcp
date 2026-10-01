package zalo

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/amrakk/zcago/errs"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"log/slog"
	"reflect"
	"strings"
	"testing"
)

func TestListenerDiagnosticsExcludeRawErrorsAndValues(t *testing.T) {
	// Arrange: sensitive-looking data in an upstream error message and JSON value.
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	secret := "SYNTHETIC_COOKIE_OR_MESSAGE_MUST_NOT_BE_LOGGED"
	cause := &json.UnmarshalTypeError{Value: secret, Type: reflect.TypeOf(int64(0)), Field: "quote.cliMsgId"}
	err := errs.WrapZCA(secret, "listener.handleOldMessages", cause)
	// Act
	logListenerError(err)
	// Assert: diagnostic schema/type survive, raw message/value do not.
	text := output.String()
	if strings.Contains(text, secret) || !strings.Contains(text, "listener.handleOldMessages") || !strings.Contains(text, "quote.cliMsgId") {
		t.Fatal("unsafe or missing listener diagnostic")
	}
}

func TestListenerFailurePreservesAuthenticationWithoutRawText(t *testing.T) {
	// Arrange
	auth := errs.WrapZCA("SYNTHETIC_SECRET", "listener.Start", errs.ErrAuthenticationRequired)
	unrelated := errs.NewZCA("SYNTHETIC_SECRET", "listener.Start")
	// Act
	authResult := listenerFailure(auth, "connection failed")
	otherResult := listenerFailure(unrelated, "connection failed")
	// Assert
	if !errors.Is(authResult, domain.ErrAuthenticationRequired) || errors.Is(otherResult, domain.ErrAuthenticationRequired) {
		t.Fatal("incorrect authentication classification")
	}
	if strings.Contains(authResult.Error(), "SYNTHETIC_SECRET") || strings.Contains(otherResult.Error(), "SYNTHETIC_SECRET") {
		t.Fatal("raw upstream data exposed")
	}
}

func TestServerKickRequiresLoginButOtherClosuresDoNot(t *testing.T) {
	for _, code := range []int{0, 1000, 1006, 3000, 3003, 4000} {
		// Arrange: server closure code without upstream reason or payload.
		// Act
		err := listenerCloseFailure(code, "listener disconnected")
		// Assert: forced logout stops retry; duplicate listener is not classified as logout.
		if errors.Is(err, domain.ErrAuthenticationRequired) != (code == 3003) {
			t.Fatalf("wrong closure classification: %d", code)
		}
	}
}
