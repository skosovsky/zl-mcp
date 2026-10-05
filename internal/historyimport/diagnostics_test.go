package historyimport

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestSourceFailureDiagnosticsOmitPrivateErrorText(t *testing.T) {
	// Arrange
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	code := 114
	private := errors.New("private-response-body-token-and-url")
	source := domain.NewHistorySourceFailure(&code, private)
	// Act
	logSourceFailure("00000000-0000-4000-8000-000000000001", source)
	logSourceFailure("00000000-0000-4000-8000-000000000001", context.DeadlineExceeded)
	logSourceFailure("00000000-0000-4000-8000-000000000001", domain.ErrHistoryInvalidPage)
	// Assert
	text := output.String()
	if strings.Contains(text, private.Error()) || strings.Contains(source.Error(), private.Error()) || !errors.Is(source, private) {
		t.Fatal("private source error leaked or causal identity lost")
	}
	for _, required := range []string{`"category":"api_error"`, `"source_code":114`, `"category":"timeout"`, `"category":"invalid_source_page"`} {
		if !strings.Contains(text, required) {
			t.Fatalf("missing safe diagnostic: %s", required)
		}
	}
}
