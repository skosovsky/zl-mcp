package collector

import (
	"context"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// refreshPreloadCatalog merges only metadata from one bounded source snapshot.
// Messages stay on explicit silent imports or the ordinary live/replay path.
func refreshPreloadCatalog(parent context.Context, store *storage.Store, source domain.ConversationPreloadSource) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	previous, err := store.PreloadStatus(ctx)
	if err != nil {
		return err
	}
	status := map[string]any{"status": "running", "last_attempt_at": time.Now().UTC().Format(time.RFC3339Nano), "last_success_at": previous["last_success_at"], "observed_count": 0, "permitted_count": 0, "catalog_complete": false, "messages_imported": false, "stop_reason": nil}
	if err = store.SetPreloadStatus(ctx, status); err != nil {
		return err
	}
	finish := func(state, reason string, cause error) error {
		status["status"], status["stop_reason"] = state, reason
		if state == "observed" {
			status["last_success_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
		save, stop := context.WithTimeout(context.WithoutCancel(parent), time.Second)
		defer stop()
		if err := store.SetPreloadStatus(save, status); err != nil {
			return err
		}
		return cause
	}
	page, err := source.ConversationPreload(ctx)
	if err != nil {
		state, reason := "partial", "upstream_unavailable"
		switch {
		case errors.Is(err, domain.ErrConversationPreloadUnsupported):
			state, reason = "unsupported", "source_unsupported"
		case errors.Is(err, domain.ErrAuthenticationRequired):
			reason = "auth_required"
		case errors.Is(err, domain.ErrHistoryInvalidPage):
			reason = "invalid_source_page"
		case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
			reason = "cancelled_or_deadline"
		}
		return finish(state, reason, err)
	}
	count, err := store.PutPreloadEntries(ctx, page.Entries)
	if err != nil {
		return finish("partial", "storage_or_metadata_error", err)
	}
	status["observed_count"], status["permitted_count"] = len(page.Entries), count
	return finish("observed", "observed_snapshot", nil)
}

func preloadCatalogLoop(ctx context.Context, store *storage.Store, source domain.ConversationPreloadSource) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		_ = refreshPreloadCatalog(ctx, store, source)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
