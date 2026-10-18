package contentread

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/app/archives"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type fixture struct {
	text         string
	role         string
	readErr      error
	lastRequest  conversations.MessageRequest
	deny         bool
	archiveCalls int
	messageCalls int
}

func (f *fixture) Message(_ context.Context, request conversations.MessageRequest) (domain.Message, error) {
	f.messageCalls++
	f.lastRequest = request
	if f.readErr != nil {
		return domain.Message{}, f.readErr
	}
	if f.deny {
		return domain.Message{}, failure("PERMISSION_DENIED", "access_denied")
	}
	message := domain.Message{Text: f.text}
	if f.role != "" {
		message.Content = &domain.MessageContent{TextRole: f.role}
	}
	return message, nil
}
func (f *fixture) Record(context.Context, string) (archives.Record, error) {
	f.archiveCalls++
	if f.readErr != nil {
		return archives.Record{}, f.readErr
	}
	if f.deny {
		return archives.Record{}, failure("PERMISSION_DENIED", "access_denied")
	}
	return archives.Record{Text: f.text, TextKind: "rich"}, nil
}
func reader(t *testing.T, f *fixture) *Handler {
	t.Helper()
	h, err := New(f, f)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

//nolint:gocognit // Ordered AAA scenario verifies every page and complete reconstruction for both independent owners.
func TestPagesBoundUnicodeAndJSONWithoutMissingText(t *testing.T) {
	for _, uri := range []string{"zalo://conversations/direct/chat/messages/message", "zalo://archives/opaque-server-token"} {
		t.Run(uri, func(t *testing.T) {
			// Arrange: escaped controls expand in JSON; emoji spans several Unicode scalars.
			f := &fixture{text: strings.Repeat("\x01👩🏽‍💻🙂\"\\", 4000)}
			h := reader(t, f)
			identity := shared.ReadIdentity{Principal: "local", Account: "account", PolicyRevision: "1"}
			q := Request{ResourceURI: uri, Limit: 8192}
			var joined strings.Builder
			offset := 0
			pages := 0
			// Act: page with variable effective length until the exact retained text ends.
			for {
				p, err := h.Read(t.Context(), identity, q)
				if err != nil {
					t.Fatal(err)
				}
				raw, _ := json.Marshal(p)
				// Assert: bounded wire bytes and contiguous scalar coordinates on every page.
				escapedRaw, _ := json.Marshal(string(raw))
				if len(raw) > MaxPageBytes || len(raw)+len(escapedRaw)+wireEnvelopeReserve > maxWireBytes ||
					p.Offset != offset ||
					p.Length != len([]rune(p.Text)) {
					t.Fatal("invalid page bounds")
				}
				joined.WriteString(p.Text)
				offset += p.Length
				pages++
				if !p.HasMore {
					if p.NextCursor != nil {
						t.Fatal("final cursor")
					}
					break
				}
				if p.NextCursor == nil || p.Length == 0 {
					t.Fatal("no progress")
				}
				q.Cursor = *p.NextCursor
			}
			if joined.String() != f.text || pages < 2 {
				t.Fatal("text not reconstructed")
			}
			if strings.Contains(uri, "archives") && f.messageCalls != 0 {
				t.Fatal("archive identity mixed with corpus")
			}
		})
	}
}
func TestEveryPageRechecksAccessContentAndIdentity(t *testing.T) {
	// Arrange.
	f := &fixture{text: "abc"}
	h := reader(t, f)
	id := shared.ReadIdentity{Principal: "one", Account: "account", PolicyRevision: "1"}
	q := Request{ResourceURI: "zalo://conversations/group/chat/messages/m", Limit: 1}
	first, err := h.Read(t.Context(), id, q)
	if err != nil {
		t.Fatal(err)
	}
	q.Cursor = *first.NextCursor
	// Act and assert: no prior page or cursor grants continued access.
	f.deny = true
	if _, err = h.Read(t.Context(), id, q); err == nil {
		t.Fatal("policy denial ignored")
	}
	f.deny = false
	f.text = "xyz"
	if _, err = h.Read(t.Context(), id, q); err == nil {
		t.Fatal("changed projection accepted")
	}
	f.text = "abc"
	for _, other := range []shared.ReadIdentity{{Principal: "two", Account: "account", PolicyRevision: "1"}, {Principal: "one", Account: "other", PolicyRevision: "1"}, {Principal: "one", Account: "account", PolicyRevision: "2"}} {
		if _, err = h.Read(t.Context(), other, q); err == nil {
			t.Fatal("foreign identity cursor")
		}
	}
	q.ResourceURI = "zalo://conversations/direct/chat/messages/m"
	if _, err = h.Read(t.Context(), id, q); err == nil {
		t.Fatal("foreign namespace cursor")
	}
}
func TestEmptyTextInvalidTargetsExpiryTamperingAndRestart(t *testing.T) {
	// Arrange.
	f := &fixture{text: ""}
	h := reader(t, f)
	id := shared.ReadIdentity{}
	q := Request{ResourceURI: "zalo://conversations/direct/chat/messages/m", Limit: 1}
	// Act/assert: legitimate empty text has a terminal, available page.
	page, err := h.Read(t.Context(), id, q)
	if err != nil || page.Text != "" || page.HasMore || page.NextCursor != nil {
		t.Fatal("empty", err)
	}
	for _, uri := range []string{"https://example.com", "zalo://archives/ar:fake/row", "zalo://conversations/direct/chat/messages/m/content", "zalo://conversations/direct/chat/messages/m?x=1"} {
		q.ResourceURI = uri
		if _, err = h.Read(t.Context(), id, q); err == nil {
			t.Fatal("bad target", uri)
		}
	}
	q.ResourceURI = "zalo://conversations/direct/chat/messages/m"
	f.text = "abc"
	page, err = h.Read(t.Context(), id, q)
	if err != nil {
		t.Fatal(err)
	}
	q.Cursor = *page.NextCursor
	restarted := reader(t, f)
	if _, err = restarted.Read(t.Context(), id, q); err == nil {
		t.Fatal("restart cursor accepted")
	}
	original := q.Cursor
	q.Cursor = original + "x"
	if _, err = h.Read(t.Context(), id, q); err == nil {
		t.Fatal("tamper")
	}
	q.Cursor = original
	h.now = func() time.Time { return time.Now().Add(16 * time.Minute) }
	if _, err = h.Read(t.Context(), id, q); err == nil {
		t.Fatal("expired cursor")
	}
}

func TestAuthorProjectionAndExactScalarBoundary(t *testing.T) {
	// Arrange: a page-sized string with authored provenance.
	f := &fixture{text: strings.Repeat("я", MaxCharacters), role: "author"}
	h := reader(t, f)
	identity := shared.ReadIdentity{}
	q := Request{ResourceURI: "zalo://conversations/direct/peer/messages/m", Limit: MaxCharacters}
	// Act/assert: exactly the limit is terminal and does not split UTF8.
	page, err := h.Read(t.Context(), identity, q)
	if err != nil || page.Length != MaxCharacters || page.HasMore || page.Text != f.text || page.TextKind != "author" {
		t.Fatal("exact boundary", err)
	}
	// A non-author attachment projection deliberately returns no author text.
	f.role = "none"
	page, err = h.Read(t.Context(), identity, q)
	if err != nil || page.Text != "" || page.TotalCharacters != 0 || page.TextKind != "none" {
		t.Fatal("metadata became author text", err)
	}
}

func TestReadErrorsDistinguishMissingInfrastructureAndCancellationWithoutLeaks(t *testing.T) {
	// Arrange: owner messages/details deliberately contain private diagnostics.
	private := "credential-secret private-db-path account-123"
	cases := []struct {
		name         string
		ownerErr     error
		code, reason string
		retry        bool
	}{
		{
			name:     "missing",
			ownerErr: &domain.Error{Code: "NOT_FOUND", Message: private, Details: map[string]any{"private": private}},
			code:     "NOT_FOUND",
			reason:   "unavailable",
		},
		{
			name:     "permission",
			ownerErr: &domain.Error{Code: "PERMISSION_DENIED", Message: private},
			code:     "PERMISSION_DENIED",
			reason:   "access_denied",
		},
		{name: "storage", ownerErr: errors.New(private), code: "STORAGE_ERROR", reason: "read_failed", retry: true},
		{
			name:     "storage-domain",
			ownerErr: &domain.Error{Code: "STORAGE_ERROR", Message: private},
			code:     "STORAGE_ERROR",
			reason:   "read_failed",
			retry:    true,
		},
		{
			name:     "collector",
			ownerErr: &domain.Error{Code: "COLLECTOR_UNAVAILABLE", Message: private, Retryable: true},
			code:     "COLLECTOR_UNAVAILABLE",
			reason:   "read_unavailable",
			retry:    true,
		},
		{
			name:     "archive-unavailable",
			ownerErr: &domain.Error{Code: "SOURCE_UNAVAILABLE", Message: private},
			code:     "SOURCE_UNAVAILABLE",
			reason:   "source_unavailable",
			retry:    true,
		},
		{
			name:     "cancelled",
			ownerErr: fmt.Errorf("%s: %w", private, context.Canceled),
			code:     "STORAGE_ERROR",
			reason:   "read_cancelled",
		},
		{
			name:     "deadline",
			ownerErr: fmt.Errorf("%s: %w", private, context.DeadlineExceeded),
			code:     "STORAGE_ERROR",
			reason:   "read_deadline",
			retry:    true,
		},
	}
	for _, uri := range []string{"zalo://conversations/direct/chat/messages/message", "zalo://groups/chat/messages/message", "zalo://archives/issued-token"} {
		for _, tc := range cases {
			t.Run(uri+"/"+tc.name, func(t *testing.T) {
				f := &fixture{readErr: tc.ownerErr}
				h := reader(t, f)
				// Act: read through the same consumer boundary as production.
				_, err := h.Read(t.Context(), shared.ReadIdentity{}, Request{ResourceURI: uri, Limit: 10})
				// Assert: only sanitized closed categories/retry semantics cross the boundary.
				de, ok := errors.AsType[*domain.Error](err)
				if !ok || de.Code != tc.code || de.Retryable != tc.retry || de.Details["reason"] != tc.reason {
					t.Fatalf("wrong classified error: %+v", de)
				}
				raw, marshalErr := json.Marshal(de)
				if marshalErr != nil || strings.Contains(string(raw), private) {
					t.Fatal("private owner diagnostics leaked", marshalErr)
				}
			})
		}
	}
	// Context still wins when an archive owner has collapsed its private reason.
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	f := &fixture{readErr: &domain.Error{Code: "SOURCE_UNAVAILABLE", Message: private}}
	_, err := reader(
		t,
		f,
	).Read(cancelled, shared.ReadIdentity{}, Request{ResourceURI: "zalo://archives/issued-token", Limit: 10})
	de, ok := errors.AsType[*domain.Error](err)
	if !ok || de.Details["reason"] != "read_cancelled" || de.Retryable {
		t.Fatal("cancelled owner reason lost")
	}
}

func TestLegacyURIUsesExplicitGroupNamespaceAndOriginalProjection(t *testing.T) {
	// Arrange: typed semantic projection would suppress this compatibility text.
	f := &fixture{text: strings.Repeat("legacy text ", 1000), role: "none"}
	h := reader(t, f)
	q := Request{ResourceURI: "zalo://groups/opaque%2Fgroup/messages/message%25id", Limit: 8192}
	// Act: follow the exact legacy resource URI.
	page, err := h.Read(t.Context(), shared.ReadIdentity{}, q)
	// Assert: explicit LegacyGroup route, original text, honest projection provenance.
	if err != nil || !f.lastRequest.LegacyGroup || f.lastRequest.Conversation.Type != domain.ConversationGroup ||
		f.lastRequest.Conversation.ID != "opaque/group" ||
		f.lastRequest.MessageID != "message%id" ||
		page.Text != f.text[:8192] ||
		page.TextKind != "legacy_projection" {
		t.Fatal("legacy route/projection changed", err)
	}
	q.ResourceURI = "zalo://conversations/group/opaque%2Fgroup/messages/message%25id"
	q.Cursor = ""
	typed, err := h.Read(t.Context(), shared.ReadIdentity{}, q)
	if err != nil || f.lastRequest.LegacyGroup || typed.Text != "" || typed.TextKind != "none" {
		t.Fatal("typed semantic projection changed", err)
	}
}
