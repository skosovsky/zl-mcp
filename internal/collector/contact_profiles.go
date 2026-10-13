package collector

import (
	"context"
	"errors"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func (g *sessionGuard) ContactProfiles(parent context.Context, ids []string) ([]domain.Contact, error) {
	source, ok := g.upstream.(domain.ContactProfileSource)
	if !ok {
		return nil, errContactsUnsupported
	}
	ctx, stop, err := g.request(parent)
	if err != nil {
		return nil, err
	}
	defer stop()
	contacts, err := source.ContactProfiles(ctx, ids)
	return contacts, g.observe(err)
}

func refreshContactProfiles(parent context.Context, store *storage.Store, source domain.ContactProfileSource) error {
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	previous, err := store.ProfileStatus(ctx)
	if err != nil {
		return err
	}
	status := map[string]any{
		"status":          "running",
		"last_attempt_at": time.Now().UTC().Format(time.RFC3339Nano),
		"last_success_at": previous["last_success_at"],
		"requested_count": 0,
		"returned_count":  0,
		"missing_count":   0,
		"stop_reason":     nil,
	}
	if err = store.SetProfileStatus(ctx, status); err != nil {
		return err
	}
	finish := func(state, reason string, cause error) error {
		status["status"] = state
		status["stop_reason"] = reason
		if state == "completed" {
			status["last_success_at"] = time.Now().UTC().Format(time.RFC3339Nano)
		}
		save, stop := context.WithTimeout(context.WithoutCancel(parent), time.Second)
		defer stop()
		if err := store.SetProfileStatus(save, status); err != nil {
			return err
		}
		return cause
	}
	storageError := func(cause error) error {
		reason := "storage_or_metadata_error"
		if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) || ctx.Err() != nil {
			reason = "cancelled_or_deadline"
		}
		return finish("partial", reason, cause)
	}
	return refreshProfilePages(ctx, store, source, status, finish, storageError)
}

func validateProfileIdentities(ids []string, records []domain.Contact) error {
	requested := map[string]bool{}
	returned := map[string]bool{}
	for _, id := range ids {
		requested[id] = true
	}
	for _, record := range records {
		if !requested[record.ID] || returned[record.ID] {
			return domain.Invalid("Invalid profile response identity.")
		}
		returned[record.ID] = true
	}
	return nil
}

func refreshProfilePages(ctx context.Context, store *storage.Store, source domain.ContactProfileSource, status map[string]any, finish func(string, string, error) error, storageError func(error) error) error {
	var requestedCount, returnedCount, missingCount int
	after := ""
	for range 20 {
		ids, cursor, more, e := store.UnenrichedDirectIDs(ctx, after, 100)
		if e != nil {
			return storageError(e)
		}
		after = cursor
		if len(ids) > 0 {
			requestedCount += len(ids)
			status["requested_count"] = requestedCount
			records, e := source.ContactProfiles(ctx, ids)
			if e != nil {
				state, reason := contactSourceOutcome(ctx, e)
				return finish(state, reason, e)
			}
			if validationErr := validateProfileIdentities(ids, records); validationErr != nil {
				return finish("partial", "storage_or_metadata_error", validationErr)
			}
			if _, e = store.PutContacts(ctx, records); e != nil {
				return storageError(e)
			}
			returnedCount += len(records)
			status["returned_count"] = returnedCount
			missingCount += len(ids) - len(records)
			status["missing_count"] = missingCount
		}
		if !more {
			if missingCount > 0 {
				return finish("partial", "missing_profiles", nil)
			}
			return finish("completed", "known_ids_processed", nil)
		}
	}
	return finish("partial", "page_limit", nil)
}
