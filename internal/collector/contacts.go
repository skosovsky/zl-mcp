package collector

import (
	"context"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

var errContactsUnsupported = errors.New("contacts source unsupported")

func (g *sessionGuard) ContactsPage(parent context.Context, page, limit int) ([]domain.Contact, error) {
	source, ok := g.upstream.(domain.ContactSource)
	if !ok {
		return nil, errContactsUnsupported
	}
	ctx, stop, err := g.request(parent)
	if err != nil {
		return nil, err
	}
	defer stop()
	contacts, err := source.ContactsPage(ctx, page, limit)
	return contacts, g.observe(err)
}

// refreshContacts bounds work and preserves records from partial runs. Counts
// describe unique IDs observed in this run, not total inbox size.
func refreshContacts(parent context.Context, store *storage.Store, source domain.ContactSource) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	previous, err := store.ContactStatus(ctx)
	if err != nil {
		return err
	}
	var observedCount, permittedCount int
	status := map[string]any{
		"status":          "running",
		"last_attempt_at": time.Now().UTC().Format(time.RFC3339Nano),
		"last_success_at": previous["last_success_at"],
		"observed_count":  0,
		"permitted_count": 0,
		"stop_reason":     nil,
	}
	if err = store.SetContactStatus(ctx, status); err != nil {
		return err
	}
	seen := map[string]bool{}
	finish := func(state, reason string, cause error) error {
		return finishContactRefresh(parent, store, status, state, reason, cause)
	}
	for page := 1; page <= 20; page++ {
		records, e := source.ContactsPage(ctx, page, 200)
		if e != nil {
			state, reason := contactSourceOutcome(ctx, e)
			return finish(state, reason, e)
		}
		if len(records) > 200 {
			return finish("partial", "oversized_page", domain.Invalid("Contact source exceeded page limit."))
		}
		fresh := []domain.Contact{}
		for _, record := range records {
			if !seen[record.ID] {
				fresh = append(fresh, record)
			}
		}
		// Merge the entire page atomically, including validation of repeated IDs.
		if _, e = store.PutContacts(ctx, records); e != nil {
			return finish("partial", "storage_or_metadata_error", e)
		}
		observed, permitted := observeFreshContacts(store, fresh, seen)
		observedCount += observed
		permittedCount += permitted
		status["observed_count"] = observedCount
		status["permitted_count"] = permittedCount
		if len(records) < 200 {
			return finish("exhausted", "short_page", nil)
		}
		if len(fresh) == 0 {
			return finish("partial", "repeated_page", nil)
		}
	}
	return finish("partial", "page_limit", nil)
}

func contactsLoop(ctx context.Context, store *storage.Store, source domain.ContactSource) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		_ = refreshContacts(ctx, store, source)
		if profiles, ok := source.(domain.ContactProfileSource); ok && ctx.Err() == nil {
			_ = refreshContactProfiles(ctx, store, profiles)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func observeFreshContacts(store *storage.Store, fresh []domain.Contact, seen map[string]bool) (int, int) {
	observed, permitted := 0, 0
	for _, record := range fresh {
		if seen[record.ID] {
			continue
		}
		seen[record.ID] = true
		observed++
		if store.AllowsConversation(domain.ConversationRef{Type: domain.ConversationDirect, ID: record.ID}) {
			permitted++
		}
	}
	return observed, permitted
}

func contactSourceOutcome(ctx context.Context, err error) (string, string) {
	switch {
	case errors.Is(err, errContactsUnsupported):
		return "unsupported", "source_unsupported"
	case errors.Is(err, domain.ErrAuthenticationRequired):
		return "partial", "auth_required"
	case ctx.Err() != nil:
		return "partial", "cancelled_or_deadline"
	default:
		return "partial", "upstream_unavailable"
	}
}

func finishContactRefresh(parent context.Context, store *storage.Store, status map[string]any, state, reason string, cause error) error {
	status["status"] = state
	status["stop_reason"] = reason
	if state == "exhausted" {
		status["last_success_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	}
	// Shutdown cancellation must not leave a misleading running source status.
	saveCtx, stop := context.WithTimeout(context.WithoutCancel(parent), time.Second)
	defer stop()
	if e := store.SetContactStatus(saveCtx, status); e != nil {
		return e
	}
	return cause

}
