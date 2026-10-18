package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"unicode/utf8"

	"github.com/google/uuid"
)

// MessageActionIntent is service-owned intent, not a raw upstream object. The
// plan is selected by the host after resolving the requested action semantics.
type MessageActionIntent struct {
	RequestID    string          `json:"request_id"`
	Kind         string          `json:"kind"`
	Plan         string          `json:"plan"`
	Conversation ConversationRef `json:"conversation"`
	MessageIDs   []string        `json:"message_ids"`
	Reaction     string          `json:"reaction"`
}

func (r MessageActionIntent) Validate() error {
	if _, err := uuid.Parse(r.RequestID); err != nil || len(r.RequestID) != 36 {
		return Invalid("request_id must be a UUID.")
	}
	if !r.Conversation.Valid() || !utf8.ValidString(r.Conversation.ID) ||
		utf8.RuneCountInString(r.Conversation.ID) > 256 {
		return Invalid("Exact typed conversation required.")
	}
	if len(r.MessageIDs) < 1 || len(r.MessageIDs) > 50 {
		return Invalid("Select 1–50 exact message IDs.")
	}
	seen := map[string]bool{}
	for _, id := range r.MessageIDs {
		if id == "" || !utf8.ValidString(id) || utf8.RuneCountInString(id) > 256 || seen[id] {
			return Invalid("Message IDs must be distinct nonempty identifiers.")
		}
		seen[id] = true
	}
	return r.validateAction()
}

func (r MessageActionIntent) validateAction() error {
	if r.Kind == "read" {
		if r.Plan != "read" || r.Reaction != "" {
			return Invalid("Invalid read receipt intent.")
		}
		return nil
	}
	if r.Kind != "reaction" || len(r.MessageIDs) != 1 {
		return Invalid("A reaction targets one exact message.")
	}
	if r.Plan == "clear" {
		if r.Reaction != "" {
			return Invalid("Removal must not select a reaction.")
		}
		return nil
	}
	if r.Plan != "add" && r.Plan != "clear_add" {
		return Invalid("Invalid reaction action plan.")
	}
	for _, choice := range ReactionCatalog() {
		if choice.Code == r.Reaction {
			return nil
		}
	}
	return Invalid("Choose a supported reaction code.")
}

func (r MessageActionIntent) Fingerprint() string {
	// UUID is ledger identity, not part of the canonical action arguments.
	r.RequestID = ""
	body, _ := json.Marshal(r)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
