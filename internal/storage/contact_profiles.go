package storage

import (
	"context"
	"encoding/json"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// UnenrichedDirectIDs scans a bounded raw page. The cursor advances even over
// excluded entries, so selected policies cannot create an endless empty page.
func (s *Store) UnenrichedDirectIDs(ctx context.Context, after string, limit int) (ids []string, cursor string, more bool, err error) {
	if limit < 1 || limit > 100 {
		return nil, "", false, domain.Invalid("Invalid profile batch limit.")
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT c.conversation_id FROM conversations c LEFT JOIN directory_contacts d ON d.peer_id=c.conversation_id WHERE c.conversation_type='direct' AND (d.peer_id IS NULL OR julianday(d.updated_at) IS NULL OR julianday(d.updated_at)<julianday(?)) AND c.conversation_id>? ORDER BY c.conversation_id LIMIT ?`, time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano), after, limit+1)
	if err != nil {
		return nil, "", false, err
	}
	defer rows.Close()
	ids = []string{}
	scanned := 0
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			return nil, "", false, err
		}
		if scanned == limit {
			more = true
			break
		}
		scanned++
		cursor = id
		if s.AllowsConversation(domain.ConversationRef{Type: domain.ConversationDirect, ID: id}) {
			ids = append(ids, id)
		}
	}
	return ids, cursor, more, rows.Err()
}

func (s *Store) SetProfileStatus(ctx context.Context, status map[string]any) error {
	body, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO metadata(key,value) VALUES('profiles_status',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(body))
	return err
}
func (s *Store) ProfileStatus(ctx context.Context) (map[string]any, error) {
	var body string
	if err := s.DB.QueryRowContext(ctx, `SELECT COALESCE((SELECT value FROM metadata WHERE key='profiles_status'),'{}')`).Scan(&body); err != nil {
		return nil, err
	}
	status := map[string]any{}
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		return nil, err
	}
	if len(status) == 0 {
		status = map[string]any{"status": "not_attempted", "last_attempt_at": nil, "last_success_at": nil, "requested_count": 0, "returned_count": 0, "missing_count": 0, "stop_reason": nil}
	}
	return status, nil
}
