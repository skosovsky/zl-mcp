package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// ReadIdentity returns the existing account binding and a deterministic current
// policy revision. It doesn't bind accounts or introduce persisted authority.
func (s *Store) ReadIdentity(ctx context.Context) (string, string, error) {
	account, err := historyAccount(ctx, s.DB)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	refs := []domain.ConversationRef{}
	for ref, enabled := range s.policy.Selected {
		if enabled {
			refs = append(refs, ref)
		}
	}
	sort.Slice(refs, func(i, j int) bool {
		if refs[i].Type != refs[j].Type {
			return refs[i].Type < refs[j].Type
		}
		return refs[i].ID < refs[j].ID
	})
	raw, err := json.Marshal(struct {
		All      bool                     `json:"All"`
		Selected []domain.ConversationRef `json:"Selected"`
	}{s.policy.All, refs})
	if err != nil {
		return "", "", err
	}
	sum := sha256.Sum256(raw)
	return account, hex.EncodeToString(sum[:]), nil
}

func (s *Store) CheckContextAccess(ref domain.ConversationRef, id string, before, after int, general bool) error {
	if before < 0 || before > 20 || after < 0 || after > 20 {
		return domain.Invalid("Context window must be between 0 and 20.")
	}
	if general {
		if !ref.Valid() || id == "" {
			return domain.Invalid("Typed conversation and message ID are required.")
		}
		if !s.AllowsConversation(ref) {
			return subscriptionPermission("Conversation is outside the collection policy.")
		}
	} else if !s.Allowed(ref.ID) {
		return &domain.Error{
			Code:       "PERMISSION_DENIED",
			Message:    "Group is outside the configured collection allowlist.",
			NextAction: domain.NextAction{Instruction: "Update the local collection allowlist if access is intended."},
			Details:    map[string]any{},
		}
	}
	return nil
}
