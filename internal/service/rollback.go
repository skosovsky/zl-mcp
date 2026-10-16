package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"

	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// checkLegacyRollbackCommand never opens a Zalo session or runs a migration.
func checkLegacyRollbackCommand(ctx context.Context, stateDir string, w io.Writer) error {
	unlock, err := local.Lock(stateDir)
	if err != nil {
		return errors.New("stop service before checking rollback")
	}
	defer unlock()
	u := url.URL{
		Scheme:   "file",
		Path:     filepath.Join(stateDir, "messages.sqlite"),
		RawQuery: "mode=ro&_pragma=busy_timeout(5000)",
	}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return errors.New("rollback database unavailable")
	}
	defer db.Close()
	report, err := storage.CheckLegacyRollback(ctx, db)
	if err != nil {
		return errors.New("rollback check failed; do not downgrade")
	}
	if err := json.NewEncoder(w).Encode(report); err != nil {
		return err
	}
	if !report.Compatible {
		return errors.New(
			"rollback blocked by new-profile subscriptions, processing or retained activity content; keep the current binary",
		)
	}
	return nil
}
