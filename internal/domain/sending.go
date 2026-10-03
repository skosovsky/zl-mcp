package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
)

type SendRequest struct {
	RecipientID string  `json:"recipient_id"`
	Text        string  `json:"text"`
	RequestID   string  `json:"request_id"`
	ReplyTo     *string `json:"reply_to_message_id,omitempty"`
}

func (r SendRequest) Validate() error {
	if _, err := uuid.Parse(r.RequestID); err != nil || len(r.RequestID) != 36 {
		return Invalid("request_id must be a UUID.")
	}
	if !utf8.ValidString(r.RecipientID) || utf8.RuneCountInString(r.RecipientID) < 1 || utf8.RuneCountInString(r.RecipientID) > 256 {
		return Invalid("recipient_id must contain 1–256 Unicode characters.")
	}
	if !utf8.ValidString(r.Text) || strings.TrimSpace(r.Text) == "" || utf8.RuneCountInString(r.Text) > 2048 {
		return Invalid("text must be nonblank UTF-8 with at most 2048 Unicode characters.")
	}
	if r.ReplyTo != nil && (!utf8.ValidString(*r.ReplyTo) || len(*r.ReplyTo) == 0 || utf8.RuneCountInString(*r.ReplyTo) > 256) {
		return Invalid("reply_to_message_id must contain 1–256 Unicode characters.")
	}
	return nil
}

// Fingerprint preserves exact arguments without storing the message text.
func (r SendRequest) Fingerprint() string {
	b, _ := json.Marshal(struct {
		Recipient string
		Text      string
		Reply     *string
	}{r.RecipientID, r.Text, r.ReplyTo})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type SendOperation struct {
	RequestID   string    `json:"request_id"`
	RecipientID string    `json:"recipient_id"`
	Status      string    `json:"status"`
	MessageID   *string   `json:"message_id"`
	Reason      *string   `json:"reason"`
	UpdatedAt   time.Time `json:"updated_at"`
	RetrySafe   bool      `json:"retry_safe"`
}

// QuoteMetadata contains protocol identifiers only. Text remains in the corpus.
type QuoteMetadata struct {
	ClientMessageID string `json:"client_message_id"`
	MessageType     string `json:"message_type"`
	Timestamp       string `json:"timestamp"`
	TTL             int    `json:"ttl"`
}

type SendQuote struct {
	MessageID string
	SenderID  string
	Text      string
	Metadata  QuoteMetadata
}
