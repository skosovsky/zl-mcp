package zalo

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/amrakk/zcago/errs"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type callbackDiagnosticError struct {
	called *bool
}

func (e callbackDiagnosticError) Error() string { return "SYNTHETIC_PRIVATE_ERROR" }

func (e callbackDiagnosticError) As(any) bool {
	*e.called = true
	panic("diagnostics must not invoke upstream As callbacks")
}

func TestListenerDiagnosticsDoNotInvokeAsCallbacks(t *testing.T) {
	// Arrange: an upstream error can supply arbitrary code through As.
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	called := false

	// Act: unknown errors receive only their concrete type diagnostic.
	logListenerError(callbackDiagnosticError{called: &called})

	// Assert: neither callbacks nor private error text enter diagnostics.
	if called || strings.Contains(output.String(), "SYNTHETIC_PRIVATE_ERROR") {
		t.Fatal("diagnostics invoked a callback or exposed private error text")
	}
}

func TestListenerDiagnosticsRespectDepthBound(t *testing.T) {
	// Arrange: the schema error lies outside the six inspected chain entries.
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	const hiddenField = "SYNTHETIC_SCHEMA_BEYOND_DEPTH_BOUND"
	var upstream error = &json.UnmarshalTypeError{Type: reflect.TypeFor[int64](), Field: hiddenField}
	for range 7 {
		upstream = errs.WrapZCA("SYNTHETIC_PRIVATE_ERROR", "listener.Start", upstream)
	}

	// Act: the diagnostic walk must not search through each wrapped chain.
	logListenerError(upstream)

	// Assert: neither the hidden schema nor upstream messages are exposed.
	if strings.Contains(output.String(), hiddenField) || strings.Contains(output.String(), "SYNTHETIC_PRIVATE_ERROR") {
		t.Fatal("diagnostics searched beyond the bounded chain")
	}
}

func TestListenerDiagnosticsExcludeRawErrorsAndValues(t *testing.T) {
	// Arrange: sensitive-looking data in an upstream error message and JSON value.
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	secret := "SYNTHETIC_COOKIE_OR_MESSAGE_MUST_NOT_BE_LOGGED"
	cause := &json.UnmarshalTypeError{Value: secret, Type: reflect.TypeFor[int64](), Field: "quote.cliMsgId"}
	err := errs.WrapZCA(secret, "listener.handleOldMessages", cause)
	// Act
	logListenerError(err)
	// Assert: diagnostic schema/type survive, raw message/value do not.
	text := output.String()
	if strings.Contains(text, secret) || !strings.Contains(text, "listener.handleOldMessages") ||
		!strings.Contains(text, "quote.cliMsgId") {
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
	if !errors.Is(authResult, domain.ErrAuthenticationRequired) ||
		errors.Is(otherResult, domain.ErrAuthenticationRequired) {
		t.Fatal("incorrect authentication classification")
	}
	if strings.Contains(authResult.Error(), "SYNTHETIC_SECRET") ||
		strings.Contains(otherResult.Error(), "SYNTHETIC_SECRET") {
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
