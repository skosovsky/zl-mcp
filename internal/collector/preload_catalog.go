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
func refreshPreloadCatalog(
	parent context.Context,
	store *storage.Store,
	source domain.ConversationPreloadSource,
) error {
	ctx, cancel := context.WithTimeout(parent, refreshPreloadCatalogTimeout)
	defer cancel()
	previous, err := store.PreloadStatus(ctx)
	if err != nil {
		return err
	}
	status := map[string]any{
		fieldStatus:         stateRunningText,
		fieldLastAttemptAt:  time.Now().UTC().Format(time.RFC3339Nano),
		fieldLastSuccessAt:  previous[fieldLastSuccessAt],
		"observed_count":    0,
		"permitted_count":   0,
		"catalog_complete":  false,
		"messages_imported": false,
		fieldStopReason:     nil,
	}
	if err = store.SetPreloadStatus(ctx, status); err != nil {
		return err
	}
	finish := func(state, reason string, cause error) error {
		status[fieldStatus], status[fieldStopReason] = state, reason
		if state == "observed" {
			status[fieldLastSuccessAt] = time.Now().UTC().Format(time.RFC3339Nano)
		}
		save, stop := context.WithTimeout(context.WithoutCancel(parent), time.Second)
		defer stop()
		if setPreloadStatusErr := store.SetPreloadStatus(save, status); setPreloadStatusErr != nil {
			return setPreloadStatusErr
		}
		return cause
	}
	page, err := source.ConversationPreload(ctx)
	if err != nil {
		state, reason := statePartialText, "upstream_unavailable"
		switch {
		case errors.Is(err, domain.ErrConversationPreloadUnsupported):
			state, reason = stateUnsupportedText, reasonSourceUnsupported
		case errors.Is(err, domain.ErrAuthenticationRequired):
			reason = reasonAuthRequired
		case errors.Is(err, domain.ErrHistoryInvalidPage):
			reason = reasonInvalidSourcePage
		case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
			reason = reasonCancelledOrDeadline
		}
		return finish(state, reason, err)
	}
	count, err := store.PutPreloadEntries(ctx, page.Entries)
	if err != nil {
		return finish(statePartialText, "storage_or_metadata_error", err)
	}
	status["observed_count"], status["permitted_count"] = len(page.Entries), count
	return finish("observed", "observed_snapshot", nil)
}

func preloadCatalogLoop(ctx context.Context, store *storage.Store, source domain.ConversationPreloadSource) {
	ticker := time.NewTicker(preloadRefreshInterval)
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
