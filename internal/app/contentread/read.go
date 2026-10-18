// Package contentread serves bounded text projections through consumer-owned ports.
package contentread

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/zl-mcp/internal/app/archives"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

const (
	MaxPageBytes        = 48 << 10
	maxWireBytes        = 64 << 10
	wireEnvelopeReserve = 1024
	MaxCharacters       = 8192
	DefaultLimit        = 2048
	maxCursorLength     = 12000
	cursorLifetime      = 15 * time.Minute
	legacyProjection    = "legacy_projection"
)

type MessagePort interface {
	Message(context.Context, conversations.MessageRequest) (domain.Message, error)
}
type ArchivePort interface {
	Record(context.Context, string) (archives.Record, error)
}
type Request struct {
	ResourceURI string
	Cursor      string
	Limit       int
}
type Page struct {
	ResourceURI     string  `json:"resource_uri"`
	Source          string  `json:"source"`
	Text            string  `json:"text"`
	TextKind        string  `json:"text_kind"`
	TextProvenance  string  `json:"text_provenance"`
	Offset          int     `json:"offset"`
	Length          int     `json:"length"`
	TotalCharacters int     `json:"total_characters"`
	HasMore         bool    `json:"has_more"`
	NextCursor      *string `json:"next_cursor"`
	Available       bool    `json:"available"`
}

// Handler borrows existing access-checking readers; it owns no account/session/store.
// Its random key deliberately invalidates continuations on process restart.
type Handler struct {
	messages MessagePort
	archives ArchivePort
	key      [32]byte
	now      func() time.Time
}

func New(messages MessagePort, archive ArchivePort) (*Handler, error) {
	if messages == nil || archive == nil {
		return nil, errors.New("content read ports required")
	}
	h := &Handler{messages: messages, archives: archive, key: [32]byte{}, now: time.Now}
	if _, err := rand.Read(h.key[:]); err != nil {
		return nil, err
	}
	return h, nil
}

type continuation struct {
	URI      string `json:"u"`
	Identity string `json:"i"`
	Digest   string `json:"d"`
	Offset   int    `json:"o"`
	Expires  int64  `json:"e"`
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func identityKey(identity shared.ReadIdentity) string {
	raw, _ := json.Marshal([]string{identity.Principal, identity.Account, identity.PolicyRevision})
	return digest(string(raw))
}
func failure(code, reason string) *domain.Error {
	return &domain.Error{
		Code:      code,
		Message:   "Requested message content cannot be read.",
		Retryable: false,
		NextAction: domain.NextAction{
			Tool:        nil,
			Arguments:   map[string]any{},
			Instruction: "Use an accessible text URI from context or browse; restart paging without cursor if expired or changed.",
		},
		Details: map[string]any{"reason": reason},
	}
}
func (h *Handler) seal(c continuation) (string, error) {
	raw, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, h.key[:])
	_, _ = mac.Write(raw)
	return base64.RawURLEncoding.EncodeToString(raw) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}
func (h *Handler) open(token string) (continuation, error) {
	var c continuation
	if len(token) > maxCursorLength {
		return c, failure("INVALID_ARGUMENT", "invalid_cursor")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return c, failure("INVALID_ARGUMENT", "invalid_cursor")
	}
	raw, e := base64.RawURLEncoding.DecodeString(parts[0])
	sig, e2 := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil || e2 != nil {
		return c, failure("INVALID_ARGUMENT", "invalid_cursor")
	}
	mac := hmac.New(sha256.New, h.key[:])
	_, _ = mac.Write(raw)
	if !hmac.Equal(sig, mac.Sum(nil)) || json.Unmarshal(raw, &c) != nil || c.Offset < 0 {
		return c, failure("INVALID_ARGUMENT", "invalid_cursor")
	}
	if h.now().Unix() >= c.Expires {
		return c, failure("INVALID_ARGUMENT", "expired_cursor")
	}
	return c, nil
}
func parseTarget(uri string) (conversations.MessageRequest, string, error) {
	var request conversations.MessageRequest
	u, err := url.Parse(uri)
	if err != nil || u.Scheme != "zalo" || u.RawQuery != "" || u.Fragment != "" || u.User != nil || u.Opaque != "" {
		return request, "", failure("INVALID_ARGUMENT", "invalid_target")
	}
	path := strings.TrimPrefix(u.EscapedPath(), "/")
	if u.Host == "archives" {
		if path == "" || len(path) > 5600 || strings.ContainsAny(path, "/%") {
			return request, "", failure("INVALID_ARGUMENT", "invalid_target")
		}
		return request, path, nil
	}
	parts := strings.Split(path, "/")
	if u.Host == "groups" {
		return legacyTarget(parts)
	}
	if u.Host != "conversations" || len(parts) != 4 || parts[2] != "messages" {
		return request, "", failure("INVALID_ARGUMENT", "invalid_target")
	}
	id, e1 := url.PathUnescape(parts[1])
	mid, e2 := url.PathUnescape(parts[3])
	ref := domain.ConversationRef{Type: parts[0], ID: id}
	if e1 != nil || e2 != nil || !ref.Valid() || mid == "" || len(id) > 256 || len(mid) > 256 {
		return request, "", failure("INVALID_ARGUMENT", "invalid_target")
	}
	request = conversations.MessageRequest{Conversation: ref, MessageID: mid, LegacyGroup: false}
	return request, "", nil
}
func legacyTarget(parts []string) (conversations.MessageRequest, string, error) {
	var request conversations.MessageRequest
	if len(parts) != 3 || parts[1] != "messages" {
		return request, "", failure("INVALID_ARGUMENT", "invalid_target")
	}
	group, groupErr := url.PathUnescape(parts[0])
	message, messageErr := url.PathUnescape(parts[2])
	ref := domain.ConversationRef{Type: domain.ConversationGroup, ID: group}
	if groupErr != nil || messageErr != nil || !ref.Valid() || message == "" || len(group) > 256 || len(message) > 256 {
		return request, "", failure("INVALID_ARGUMENT", "invalid_target")
	}
	return conversations.MessageRequest{Conversation: ref, MessageID: message, LegacyGroup: true}, "", nil
}
func (h *Handler) text(ctx context.Context, uri string) (string, string, string, error) {
	request, token, err := parseTarget(uri)
	if err != nil {
		return "", "", "", err
	}
	if token != "" {
		record, e := h.archives.Record(ctx, token)
		if e != nil {
			return "", "", "", safeReadError(ctx, e)
		}
		if record.TextKind != "plain" && record.TextKind != "rich" {
			return "", "", "", failure("SOURCE_UNSUPPORTED", "unsupported_projection")
		}
		return record.Text, "archive", record.TextKind, nil
	}
	message, err := h.messages.Message(ctx, request)
	if err != nil {
		return "", "", "", safeReadError(ctx, err)
	}
	if request.LegacyGroup {
		return message.Text, "corpus", legacyProjection, nil
	}
	role := legacyProjection
	if message.Content != nil {
		role = message.Content.TextRole
	}
	switch role {
	case "author", "none", legacyProjection:
	default:
		return "", "", "", failure("SOURCE_UNSUPPORTED", "unsupported_projection")
	}
	return message.SemanticText(), "corpus", role, nil
}
func safeReadError(ctx context.Context, err error) error {
	// Some archive owners deliberately collapse private reasons. The request
	// context still distinguishes cancellation/deadline without exposing raw errors.
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	switch {
	case errors.Is(err, context.Canceled):
		return readFailure("STORAGE_ERROR", "read_cancelled", false)
	case errors.Is(err, context.DeadlineExceeded):
		return readFailure("STORAGE_ERROR", "read_deadline", true)
	}
	if e, ok := errors.AsType[*domain.Error](err); ok {
		switch e.Code {
		case "PERMISSION_DENIED":
			return failure("PERMISSION_DENIED", "access_denied")
		case "NOT_FOUND":
			return failure("NOT_FOUND", "unavailable")
		case "COLLECTOR_UNAVAILABLE", "RATE_LIMITED":
			return readFailure(e.Code, "read_unavailable", e.Retryable)
		case "SOURCE_UNAVAILABLE":
			return readFailure(e.Code, "source_unavailable", true)
		case "SOURCE_UNSUPPORTED", "SOURCE_CONTROLS_UNCLASSIFIED":
			return failure(e.Code, "unsupported_projection")
		}
	}
	return readFailure("STORAGE_ERROR", "read_failed", true)
}
func readFailure(code, reason string, retryable bool) error {
	de := failure(code, reason)
	de.Retryable = retryable
	de.NextAction.Instruction = "Retry this read only when its cancellation or storage/collector availability has been resolved; do not infer that the message is absent."
	return de
}

func (h *Handler) Read(ctx context.Context, identity shared.ReadIdentity, request Request) (Page, error) {
	var empty Page
	if len(request.ResourceURI) > 6000 || request.Limit < 1 || request.Limit > MaxCharacters {
		return empty, failure("INVALID_ARGUMENT", "invalid_limit_or_target")
	}
	c := continuation{
		URI:      request.ResourceURI,
		Identity: identityKey(identity),
		Digest:   "",
		Offset:   0,
		Expires:  h.now().Add(cursorLifetime).Unix(),
	}
	if request.Cursor != "" {
		var err error
		c, err = h.open(request.Cursor)
		if err != nil {
			return empty, err
		}
		if c.URI != request.ResourceURI || c.Identity != identityKey(identity) {
			return empty, failure("INVALID_ARGUMENT", "invalid_cursor")
		}
	}
	text, source, kind, err := h.text(ctx, request.ResourceURI)
	if err != nil {
		return empty, err
	}
	if !utf8.ValidString(text) {
		return empty, failure("SOURCE_UNSUPPORTED", "unsupported_projection")
	}
	projection, projectionErr := json.Marshal([]string{source, kind, text})
	if projectionErr != nil {
		return empty, failure("SOURCE_UNSUPPORTED", "unsupported_projection")
	}
	currentDigest := digest(string(projection))
	if c.Digest != "" && c.Digest != currentDigest {
		return empty, failure("INVALID_ARGUMENT", "content_changed")
	}
	c.Digest = currentDigest
	runes := []rune(text)
	if c.Offset > len(runes) {
		return empty, failure("INVALID_ARGUMENT", "invalid_cursor")
	}
	return h.page(c, request, runes, source, kind)
}

func (h *Handler) page(c continuation, request Request, runes []rune, source, kind string) (Page, error) {
	var empty Page
	end := min(c.Offset+request.Limit, len(runes))
	provenance := "retained_message"
	if source == "archive" {
		provenance = "saved_archive_projection"
	}
	page := Page{
		ResourceURI:     request.ResourceURI,
		Source:          source,
		Text:            "",
		TextKind:        kind,
		TextProvenance:  provenance,
		Offset:          c.Offset,
		Length:          0,
		TotalCharacters: len(runes),
		HasMore:         false,
		NextCursor:      nil,
		Available:       true,
	}
	for {
		page.Text = string(runes[c.Offset:end])
		page.Length = end - c.Offset
		page.HasMore = end < len(runes)
		page.NextCursor = nil
		if page.HasMore {
			next := c
			next.Offset = end
			token, e := h.seal(next)
			if e != nil {
				return empty, e
			}
			page.NextCursor = &token
		}
		raw, e := json.Marshal(page)
		if e != nil {
			return empty, e
		}
		// MCP carries the projection twice: structured JSON and its JSON text
		// compatibility representation. Account for the second escaping layer,
		// reserving envelope/SDK metadata space without depending on transport types.
		textRepresentation, encodingErr := json.Marshal(string(raw))
		if encodingErr != nil {
			return empty, encodingErr
		}
		if len(raw) <= MaxPageBytes && len(raw)+len(textRepresentation)+wireEnvelopeReserve <= maxWireBytes {
			return page, nil
		}
		if end == c.Offset {
			return empty, failure("RESPONSE_TOO_LARGE", "oversized_projection")
		}
		end = c.Offset + (end-c.Offset)/2
	}
}
