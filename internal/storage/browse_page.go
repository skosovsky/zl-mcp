package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// BrowsePage retains the original insertion high-water mark while allowing
// transport to reduce the count. Shortening never advances an unseen cursor.
type BrowsePage struct {
	Value map[string]any
	read  func(context.Context, int) (map[string]any, error)
}

func (p *BrowsePage) Shorten(ctx context.Context, n int) error {
	value, err := p.read(ctx, n)
	if err == nil {
		p.Value = value
	}
	return err
}

func (s *Store) BrowsePage(
	ctx context.Context,
	ref domain.ConversationRef,
	since, until, order string,
	limit int,
	token string,
) (*BrowsePage, error) {
	if !ref.Valid() {
		return nil, domain.Invalid("Typed conversation required.")
	}
	if !s.AllowsConversation(ref) {
		return nil, subscriptionPermission("Conversation is outside collection policy.")
	}
	if _, err := browseBounds(since, until); err != nil {
		return nil, err
	}
	if limit < 0 || limit > 50 {
		return nil, domain.Invalid("Use limit 1–50.")
	}
	if order == "" {
		order = wireDesc
	}
	// Browse performs canonical validation and authorization on every read.
	filter, _ := json.Marshal([]string{"browse", ref.Type, ref.ID, since, until, order})
	sum := sha256.Sum256(filter)
	c, err := s.browseCursor(ctx, hex.EncodeToString(sum[:]), token)
	if err != nil {
		return nil, err
	}
	frozen, err := s.EncodeCursor(c)
	if err != nil {
		return nil, err
	}
	read := func(ctx context.Context, n int) (map[string]any, error) {
		return s.Browse(ctx, ref, since, until, order, n, frozen)
	}
	value, err := read(ctx, limit)
	if err != nil {
		return nil, err
	}
	return &BrowsePage{Value: value, read: read}, nil
}
