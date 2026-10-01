package websocketx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/amrakk/zcago/errs"
)

func TestDialClassifiesOnlyExplicitUnauthorized(t *testing.T) {
	for _, status := range []int{401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			// Arrange: exercise the actual websocket handshake without a live account.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte("SYNTHETIC_PRIVATE_RESPONSE"))
			}))
			defer server.Close()
			// Act
			_, err := Dial(context.Background(), server.URL, nil)
			// Assert: forbidden/rate-limit/server errors do not force a fresh login.
			if err == nil || errors.Is(err, errs.ErrAuthenticationRequired) != (status == 401) {
				t.Fatalf("incorrect classification for %d: %v", status, err)
			}
			if status == 401 && err.Error() != "authentication required" {
				t.Fatalf("authentication error exposed upstream data: %v", err)
			}
		})
	}
}
