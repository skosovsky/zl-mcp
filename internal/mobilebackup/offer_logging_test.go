package mobilebackup

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestOfferFailureCodeUsesFixedCategories(t *testing.T) {
	// Arrange: transport errors may contain credentials or payloads.
	cases := []struct {
		err  error
		want string
	}{
		{context.DeadlineExceeded, "TIMEOUT"}, {context.Canceled, "CANCELLED"},
		{domain.ErrAuthenticationRequired, "AUTH_REQUIRED"}, {domain.ErrHistoryUnsupported, "SOURCE_UNAVAILABLE"},
		{domain.ErrMobileBackupUnknown, "REQUEST_UNKNOWN"}, {domain.ErrMobileBackupRejected, "REJECTED"},
		{domain.ErrMobileBackupInvalid, "INVALID_SOURCE_EVENT"}, {errors.New("private-url-and-key"), "EXECUTION_FAILED"},
	}
	for _, c := range cases {
		// Act.
		got := offerFailureCode(c.err)
		// Assert.
		if got != c.want {
			t.Fatal("unexpected safe category")
		}
	}
}

func TestOfferLogsCommittedProgressWithoutPrivateSourceError(t *testing.T) {
	// Arrange: a claimed phone request reaches the confirmation stage then fails.
	s, a, public := runnerAttempt(t)
	var output bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	defer slog.SetDefault(previous)
	source := offerSourceFunc(func(ctx context.Context, o *domain.MobileBackupObserver) (domain.MobileBackupOffer, error) {
		if err := o.BeforeDispatch(public); err != nil {
			return domain.MobileBackupOffer{}, err
		}
		if err := o.Progress("waiting_for_confirmation"); err != nil {
			return domain.MobileBackupOffer{}, err
		}
		return domain.MobileBackupOffer{}, errors.New("private-url-and-key")
	})
	// Act.
	_, err := RunPreparedOffer(context.Background(), s, source, a.OperationID)
	// Assert: persisted stages and a fixed category, never raw error/key.
	logs := output.String()
	if err == nil || !strings.Contains(logs, "dispatching") || !strings.Contains(logs, "waiting_for_confirmation") || !strings.Contains(logs, "EXECUTION_FAILED") || strings.Contains(logs, "private-url-and-key") || strings.Contains(logs, public) {
		t.Fatal("unsafe or missing offer diagnostics")
	}
}
