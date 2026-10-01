package zalo

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/amrakk/zcago"
	"github.com/amrakk/zcago/errs"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/local"
)

func TestRestoreDistinguishesLoginRejectionFromNetworkFailure(t *testing.T) {
	for _, authFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "network", true: "authentication"}[authFailure], func(t *testing.T) {
			// Arrange: valid private session, actual cookie restoration, controlled login boundary.
			dir := t.TempDir()
			session := savedSession{Version: 2, Credentials: zcago.Credentials{IMEI: "synthetic", UserAgent: "test"}, Cookies: []savedCookie{{Origin: "https://zalo.me", Cookie: http.Cookie{Name: "synthetic", Value: "private", Domain: ".zalo.me", Path: "/"}}}}
			data, err := json.Marshal(session)
			if err != nil {
				t.Fatal(err)
			}
			if err := local.WritePrivate(filepath.Join(dir, "session.json"), data); err != nil {
				t.Fatal(err)
			}
			var calls int
			login := func(_ context.Context, _ *persistentJar, c zcago.Credentials) (zcago.API, error) {
				calls++
				if c.Cookie != nil {
					t.Fatal("lossy cookie union restored")
				}
				if authFailure {
					return nil, errs.WrapZCA("SYNTHETIC_SECRET", "login", errs.ErrAuthenticationRequired)
				}
				return nil, errors.New("SYNTHETIC_SECRET: network unavailable")
			}
			// Act
			_, err = restoreWithLogin(context.Background(), dir, login)
			// Assert: a network failure never demands a new QR or leaks raw upstream text.
			if calls != 1 || errors.Is(err, domain.ErrAuthenticationRequired) != authFailure {
				t.Fatalf("calls=%d error=%v", calls, err)
			}
			if !authFailure {
				var typed *domain.Error
				if !errors.As(err, &typed) || typed.Code != "UPSTREAM_UNAVAILABLE" || !typed.Retryable {
					t.Fatalf("incorrect network result: %v", err)
				}
			}
			after, readErr := os.ReadFile(filepath.Join(dir, "session.json"))
			if readErr != nil || string(after) != string(data) {
				t.Fatal("failed restore changed saved credentials")
			}
		})
	}
}

func TestRestoreRejectsMissingLegacyAndInsecureSessionBeforeLogin(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		mode          os.FileMode
		auth          bool
	}{
		{"missing", "", 0, true},
		{"legacy", `{"version":1}`, 0600, true},
		{"malformed", `{`, 0600, true},
		{"insecure", `{"version":2}`, 0644, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: unusable local session must not reach the login endpoint.
			dir := t.TempDir()
			if tc.mode != 0 {
				p := filepath.Join(dir, "session.json")
				if err := os.WriteFile(p, []byte(tc.content), tc.mode); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(p, tc.mode); err != nil {
					t.Fatal(err)
				}
			}
			calls := 0
			login := func(context.Context, *persistentJar, zcago.Credentials) (zcago.API, error) {
				calls++
				return nil, errors.New("unexpected login")
			}
			// Act
			_, err := restoreWithLogin(context.Background(), dir, login)
			// Assert
			if err == nil || calls != 0 || errors.Is(err, domain.ErrAuthenticationRequired) != tc.auth {
				t.Fatalf("error=%v calls=%d", err, calls)
			}
			if !tc.auth {
				var typed *domain.Error
				if !errors.As(err, &typed) || typed.Code != "STORAGE_ERROR" {
					t.Fatalf("private-file failure incorrectly classified: %v", err)
				}
			}
		})
	}
}

func TestRestoreAuthenticationCodeIsScopedAndTyped(t *testing.T) {
	for _, number := range []int{0, 101, 102, 103, 114, 240, 401} {
		for _, pointer := range []bool{false, true} {
			// Arrange: typed login code; messages must never decide authentication.
			code := errs.ZaloErrorCode(number)
			value := errs.ZaloAPIError{Code: &code, Message: "102 invalid session"}
			var err error = value
			if pointer {
				err = &value
			}
			// Act
			actual := restoreAuthenticationFailure(err)
			// Assert
			if actual != (number == 102) {
				t.Fatalf("unexpected code classification: %d", number)
			}
		}
	}
	// Act/Assert: unknown text cannot turn a network failure into an auth rejection.
	if restoreAuthenticationFailure(errors.New("102 invalid session")) {
		t.Fatal("raw error text guessed")
	}
}
