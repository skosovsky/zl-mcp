package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// AllowRead shares one 10/s token bucket (burst 20) across every MCP process
// using this account's state database. Only budget metadata is modified.
func (s *Store) AllowRead(ctx context.Context) (bool, error) {
	return s.allowReadAt(ctx, time.Now().UnixNano())
}

func (s *Store) allowReadAt(ctx context.Context, at int64) (bool, error) {
	var remaining float64
	// A single statement atomically refills and consumes, avoiding read/modify/write
	// races between independent connections. Clock rollback never creates tokens.
	err := s.DB.QueryRowContext(ctx, `INSERT INTO read_budget(id,tokens,last_ns) VALUES(1,19,?)
 ON CONFLICT(id) DO UPDATE SET
 tokens=min(20,read_budget.tokens+max(0,(excluded.last_ns-read_budget.last_ns)/100000000.0))-1,
 last_ns=max(read_budget.last_ns,excluded.last_ns)
 WHERE min(20,read_budget.tokens+max(0,(excluded.last_ns-read_budget.last_ns)/100000000.0))>=1
 RETURNING tokens`, at).Scan(&remaining)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}
