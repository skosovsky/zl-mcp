package storage

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"golang.org/x/text/unicode/norm"
)

func (s *Store) migrateContacts(ctx context.Context) error {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS directory_contacts(peer_id TEXT PRIMARY KEY,name TEXT NOT NULL,aliases TEXT NOT NULL,friendship TEXT NOT NULL CHECK(friendship IN ('friend','non_friend','unknown')),updated_at TEXT NOT NULL); INSERT OR IGNORE INTO schema_migrations VALUES(7);`); err != nil {
		return err
	}
	return tx.Commit()
}

// ContactNameKey is a candidate-matching key, never an identity key.
func ContactNameKey(value string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(value)) {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if r == 'đ' {
			r = 'd'
		}
		b.WriteRune(r)
	}
	return norm.NFC.String(b.String())
}

// PutContacts atomically merges a bounded page without messages/events/novelty.
func (s *Store) PutContacts(ctx context.Context, contacts []domain.Contact) (int, error) {
	if len(contacts) > 200 {
		return 0, domain.Invalid("Contact page exceeds 200 records.")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	count := 0
	stamp := now()
	for _, c := range contacts {
		if c.ID == "" || len(c.ID) > 256 || len(c.Name) > 4096 || (c.Friendship != "friend" && c.Friendship != "non_friend" && c.Friendship != "unknown") {
			return 0, domain.Invalid("Invalid contact metadata.")
		}
		ref := domain.ConversationRef{Type: "direct", ID: c.ID}
		if !s.AllowsConversation(ref) {
			continue
		}
		aliases := []string{}
		seen := map[string]bool{}
		for _, name := range append([]string{c.Name}, c.Aliases...) {
			if name != "" && !seen[name] {
				if len(name) > 4096 || len(aliases) >= 10 {
					return 0, domain.Invalid("Contact aliases exceed directory limits.")
				}
				aliases = append(aliases, name)
				seen[name] = true
			}
		}
		body, err := json.Marshal(aliases)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO directory_contacts VALUES(?,?,?,?,?) ON CONFLICT(peer_id) DO UPDATE SET name=excluded.name,aliases=excluded.aliases,friendship=excluded.friendship,updated_at=excluded.updated_at`, c.ID, c.Name, string(body), c.Friendship, stamp); err != nil {
			return 0, err
		}
		var name any
		if c.Name != "" {
			name = c.Name
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO conversations VALUES('direct',?,?,'contacts_catalog','unknown',?,?) ON CONFLICT(conversation_type,conversation_id) DO UPDATE SET name=COALESCE(excluded.name,conversations.name),updated_at=excluded.updated_at`, c.ID, name, stamp, stamp); err != nil {
			return 0, err
		}
		count++
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

func (s *Store) SetContactStatus(ctx context.Context, status map[string]any) error {
	body, err := json.Marshal(status)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `INSERT INTO metadata(key,value) VALUES('contacts_status',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, string(body))
	return err
}

func (s *Store) ContactStatus(ctx context.Context) (map[string]any, error) {
	var body string
	err := s.DB.QueryRowContext(ctx, "SELECT COALESCE((SELECT value FROM metadata WHERE key='contacts_status'),'{}')").Scan(&body)
	if err != nil {
		return nil, err
	}
	v := map[string]any{}
	if err = json.Unmarshal([]byte(body), &v); err != nil {
		return nil, err
	}
	if len(v) == 0 {
		v = map[string]any{"status": "not_attempted", "last_attempt_at": nil, "last_success_at": nil, "observed_count": 0, "permitted_count": 0, "stop_reason": nil}
	}
	return v, nil
}

// The join keeps one SQLite read per catalogue traversal. In particular it must
// not acquire a second connection while the result rows hold our single handle.
const directorySelect = `SELECT c.conversation_type,c.conversation_id,c.name,c.metadata_source,c.availability,c.first_discovered_at,c.updated_at,COALESCE(d.aliases,''),COALESCE(d.friendship,'unknown'),EXISTS(SELECT 1 FROM messages m WHERE m.conversation_type=c.conversation_type AND m.conversation_id=c.conversation_id) FROM conversations c LEFT JOIN directory_contacts d ON c.conversation_type='direct' AND d.peer_id=c.conversation_id`

func directoryMetadata(raw, source string) ([]string, []string, error) {
	aliases := []string{}
	sources := []string{source}
	if raw != "" {
		if err := json.Unmarshal([]byte(raw), &aliases); err != nil {
			return nil, nil, err
		}
		if source != "contacts_catalog" {
			sources = append(sources, "contacts_catalog")
		}
	}
	return aliases, sources, nil
}

func directoryNameMatches(name *string, aliases []string, query string) bool {
	key := ContactNameKey(query)
	if name != nil && strings.Contains(ContactNameKey(*name), key) {
		return true
	}
	for _, alias := range aliases {
		if strings.Contains(ContactNameKey(alias), key) {
			return true
		}
	}
	return false
}
