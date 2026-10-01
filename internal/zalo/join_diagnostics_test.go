package zalo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/amrakk/zcago/errs"
)

func TestJoinDiagnosticsOnlyRecordStructuralFields(t *testing.T) {
	// Arrange: secret-bearing errors from each supported boundary; no live account.
	code := errs.ZaloErrorCode(240)
	marker := "private-cookie-and-invite"
	cases := []struct {
		name string
		err  error
		kind string
		code bool
	}{
		{"api-value", fmt.Errorf("wrapped %s: %w", marker, errs.ZaloAPIError{Code: &code, Message: marker}), "api", true},
		{"api-pointer", &errs.ZaloAPIError{Code: &code, Message: marker}, "api", true},
		{"api-without-code", errs.ZaloAPIError{Message: marker}, "api", false},
		{"decode", fmt.Errorf("%s: %w", marker, &json.UnmarshalTypeError{Value: marker, Type: reflect.TypeOf(""), Field: marker, Struct: marker}), "decode", false},
		{"syntax", &json.SyntaxError{}, "decode", false},
		{"transport", &url.Error{Op: marker, URL: marker, Err: errors.New(marker)}, "transport", false},
		{"canceled", fmt.Errorf("%s: %w", marker, context.Canceled), "canceled", false},
		{"deadline", context.DeadlineExceeded, "deadline", false},
		{"unknown", errors.New(marker), "unclassified", false},
	}
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var output bytes.Buffer
			slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
			// Act
			logJoinFailure(tc.err)
			// Assert: allowlisted fields only, no original message or JSON metadata.
			var event map[string]any
			if err := json.Unmarshal(output.Bytes(), &event); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), marker) || event["failure_kind"] != tc.kind || event["msg"] != "join upstream failure" {
				t.Fatalf("unsafe diagnostics: %s", output.String())
			}
			expectedFields := 4
			if tc.code {
				expectedFields++
				if event["upstream_code"] != float64(240) {
					t.Fatalf("lost code: %v", event)
				}
			}
			if len(event) != expectedFields {
				t.Fatalf("unexpected diagnostic fields: %v", event)
			}
		})
	}
	// Act/Assert: successful joins emit no failure event.
	var output bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&output, nil)))
	logJoinFailure(nil)
	if output.Len() != 0 {
		t.Fatal("success logged as failure")
	}
}
