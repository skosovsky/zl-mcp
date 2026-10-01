package auth

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/internal/cryptox"
	"github.com/amrakk/zcago/internal/httpx"
	"github.com/amrakk/zcago/session"
)

func TestLoginResponsePreservesCodeAndExplicit401(t *testing.T) {
	for _, status := range []int{200, 401, 403, 500} {
		// Arrange: synthetic response with code 102 and private-looking message.
		response := &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(`{"error_code":102,"error_message":"SYNTHETIC_SECRET","data":null}`))}
		// Act
		_, err := processLoginResponse(session.NewContext(session.WithLogging(false)), response)
		// Assert: only 401 is HTTP authentication evidence; nonzero body codes survive.
		if errors.Is(err, errs.ErrAuthenticationRequired) != (status == 401) {
			t.Fatalf("incorrect status classification: %d", status)
		}
		if status != 401 {
			var apiErr errs.ZaloAPIError
			if !errors.As(err, &apiErr) || apiErr.Code == nil || int(*apiErr.Code) != 102 {
				t.Fatal("login code lost")
			}
		}
		if strings.Contains(err.Error(), "SYNTHETIC_SECRET") {
			t.Fatal("private login response leaked")
		}
	}
}

func TestDecryptedLoginAndServerInfoPreserveCode(t *testing.T) {
	// Arrange: real encryption boundary, synthetic rejected login.
	key := "0123456789abcdef"
	encrypted, err := cryptox.EncodeAESCBC([]byte(key), `{"error_code":102,"error_message":"SYNTHETIC_SECRET","data":null}`, cryptox.EncryptTypeBase64)
	if err != nil {
		t.Fatal(err)
	}
	// Act
	_, err = decryptAndParseLoginData(&httpx.EncryptParamResult{Enk: &key}, &httpx.BaseResponse{Data: &encrypted})
	// Assert
	var apiErr errs.ZaloAPIError
	if !errors.As(err, &apiErr) || apiErr.Code == nil || int(*apiErr.Code) != 102 || strings.Contains(err.Error(), "SYNTHETIC_SECRET") {
		t.Fatalf("decrypted code lost: %v", err)
	}
	// Arrange
	response := &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"error_code":102,"error_message":"SYNTHETIC_SECRET","data":null}`))}
	// Act
	_, err = parseServerInfoResponse(response)
	// Assert
	if !errors.As(err, &apiErr) || apiErr.Code == nil || int(*apiErr.Code) != 102 || strings.Contains(err.Error(), "SYNTHETIC_SECRET") {
		t.Fatalf("server info code lost: %v", err)
	}
}

type rejectedRoundTripper struct{}

func (rejectedRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, context.Canceled
}
func TestServerInfoCancellationWithoutResponseDoesNotPanic(t *testing.T) {
	// Arrange: the concurrent login request can cancel server-info before any response.
	sc := session.NewContext(session.WithLogging(false), session.WithHTTPClient(&http.Client{Transport: rejectedRoundTripper{}}))
	// Act
	_, err := makeServerInfoRequest(context.Background(), sc, map[string]any{})
	// Assert: preserve cancellation instead of dereferencing a nil HTTP response.
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
