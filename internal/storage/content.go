package storage

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// migrateMessageContent is additive: the previous binary can continue using its
// unchanged columns. It cannot classify new content; old records remain unknown.
func (s *Store) migrateMessageContent(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // A committed transaction needs no rollback.
	var exists int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations WHERE version=15").
		Scan(&exists); err != nil {
		return err
	}
	if exists != 0 {
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, `
ALTER TABLE messages ADD COLUMN content_json TEXT;
INSERT INTO schema_migrations VALUES(15);`); err != nil {
		return err
	}
	return tx.Commit()
}

// The private journal envelope preserves structured metadata without extending
// the legacy public domain.Message JSON representation.
type storedContentMessage struct {
	domain.Message

	Content *domain.MessageContent `json:"content,omitempty"`
}

func encodeContentMessage(m domain.Message) ([]byte, error) {
	return json.Marshal(storedContentMessage{Message: m, Content: m.Content})
}
func decodeContentMessage(b []byte) (domain.Message, error) {
	var v storedContentMessage
	err := json.Unmarshal(b, &v)
	v.Message.Content = v.Content
	return v.Message, err
}

//nolint:nilnil // SQL NULL deliberately represents absent optional content.
func contentJSON(c *domain.MessageContent) (any, error) {
	if c == nil {
		return nil, nil
	}
	b, err := json.Marshal(c)
	return string(b), err
}

//nolint:nilnil // A pre-migration record has no retained content, not a read failure.
func decodeStoredContent(raw sql.NullString) (*domain.MessageContent, error) {
	if !raw.Valid {
		return nil, nil
	}
	var c domain.MessageContent
	if err := json.Unmarshal([]byte(raw.String), &c); err != nil {
		return nil, err
	}
	return &c, nil
}
