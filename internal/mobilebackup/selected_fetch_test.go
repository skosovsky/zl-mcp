package mobilebackup

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

type identitySourceFunc func(context.Context, domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error)

func (f identitySourceFunc) MapMobileBackupIdentities(c context.Context, r domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
	return f(c, r)
}
func selectedFetchRequest() domain.MobileBackupRequest {
	return domain.MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "12", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"}
}
func TestSelectedFetchIndependentStagesAndOwnership(t *testing.T) {
	// Arrange: independent complete encrypted fixture with opaque tail.
	v := archiveVectors(t)[2]
	encrypted, _ := hex.DecodeString(v.Ciphertext)
	want, _ := hex.DecodeString(v.Plaintext)
	d, e := NewDownloader([]string{"archive.example.com"})
	if e != nil {
		t.Fatal(e)
	}
	downloads, mappings := 0, 0
	d.client.Transport = downloadTransport(func(r *http.Request) (*http.Response, error) {
		downloads++
		return &http.Response{StatusCode: 200, ContentLength: int64(len(encrypted)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(encrypted))}, nil
	})
	mapper := identitySourceFunc(func(c context.Context, r domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
		mappings++
		if len(r.Direct) != 1 || r.Direct[0]+".db" != v.Filename || len(r.Groups) != 0 {
			t.Fatal("wrong filename mapping request")
		}
		return []domain.MobileIdentityPair{{Plain: r.Direct[0], Session: "12"}}, nil
	})
	offer := domain.MobileBackupOffer{URL: "https://archive.example.com/private", KeyText: strings.Repeat("0123456789abcdef", 4), FileSize: uint64(len(encrypted))}
	// Act.
	got, e := FetchSelectedArchive(context.Background(), d, offer, selectedFetchRequest(), mapper)
	// Assert: one download/map, exact selected bytes, no private result serialization.
	if e != nil || downloads != 1 || mappings != 1 || got.File.Name != v.Filename || !bytes.Equal(got.File.Data, want) || got.TrailingBytes != v.TrailingBytes {
		t.Fatal("selected fetch failed", e)
	}
	b, _ := json.Marshal(got)
	if string(b) != "{}" || fmt.Sprintf("%#v", got) != "mobile backup selected archive [redacted]" {
		t.Fatal("selected fetch exposed")
	}
	owned := got.File.Data
	got.Clear()
	if got.File.Data != nil || !bytes.Equal(owned, make([]byte, len(owned))) {
		t.Fatal("selected fetch retained")
	}
}
func TestSelectedFetchRejectsMappingFailureCancellationAndBudget(t *testing.T) {
	for _, mode := range []string{"missing", "extra", "private-error", "cancel-mapping", "budget", "invalid-selection"} {
		t.Run(mode, func(t *testing.T) {
			// Arrange.
			v := archiveVectors(t)[1]
			encrypted, _ := hex.DecodeString(v.Ciphertext)
			d, _ := NewDownloader([]string{"archive.example.com"})
			downloads := 0
			d.client.Transport = downloadTransport(func(r *http.Request) (*http.Response, error) {
				downloads++
				return &http.Response{StatusCode: 200, ContentLength: int64(len(encrypted)), Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(encrypted))}, nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			mapper := identitySourceFunc(func(c context.Context, r domain.MobileIdentityRequest) ([]domain.MobileIdentityPair, error) {
				switch mode {
				case "missing":
					return nil, nil
				case "extra":
					return []domain.MobileIdentityPair{{Plain: r.Direct[0], Session: "12"}, {Plain: "2", Session: "13"}}, nil
				case "private-error":
					return nil, errors.New("private marker must not escape")
				case "cancel-mapping":
					cancel()
				}
				return []domain.MobileIdentityPair{{Plain: r.Direct[0], Session: "12"}}, nil
			})
			request := selectedFetchRequest()
			if mode == "budget" {
				request.MaxArchiveBytes = int64(len(encrypted) - 1)
			}
			if mode == "invalid-selection" {
				request.ConversationID = "opaque-invalid-numeric"
			}
			offer := domain.MobileBackupOffer{URL: "https://archive.example.com/private", KeyText: strings.Repeat("0123456789abcdef", 4), FileSize: uint64(len(encrypted))}
			// Act.
			got, e := FetchSelectedArchive(ctx, d, offer, request, mapper)
			// Assert: no source error/private prefix or file survives; invalid request avoids download.
			if !errors.Is(e, ErrArchive) || got.File.Data != nil || got.CiphertextBytes != 0 || mode == "invalid-selection" && downloads != 0 || mode == "budget" && downloads != 0 {
				t.Fatal("invalid staged fetch accepted")
			}
		})
	}
}
