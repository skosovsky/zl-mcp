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
	status := map[string]any{"status": "running", "last_attempt_at": time.Now().UTC().Format(time.RFC3339Nano), "last_success_at": previous["last_success_at"], "requested_count": 0, "returned_count": 0, "missing_count": 0, "stop_reason": nil}
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
	storageFailure := func(cause error) error {
		reason := "storage_or_metadata_error"
		if errors.Is(cause, context.Canceled) || errors.Is(cause, context.DeadlineExceeded) || ctx.Err() != nil {
			reason = "cancelled_or_deadline"
		}
		return finish("partial", reason, cause)
	}
	after := ""
	for batch := 0; batch < 20; batch++ {
		ids, cursor, more, e := store.UnenrichedDirectIDs(ctx, after, 100)
		if e != nil {
			return storageFailure(e)
		}
		after = cursor
		if len(ids) > 0 {
			status["requested_count"] = status["requested_count"].(int) + len(ids)
			records, e := source.ContactProfiles(ctx, ids)
			if e != nil {
				state, reason := "partial", "upstream_unavailable"
				switch {
				case errors.Is(e, errContactsUnsupported):
					state = "unsupported"
					reason = "source_unsupported"
				case errors.Is(e, domain.ErrAuthenticationRequired):
					reason = "auth_required"
				case ctx.Err() != nil:
					reason = "cancelled_or_deadline"
				}
				return finish(state, reason, e)
			}
			requested := map[string]bool{}
			returned := map[string]bool{}
			for _, id := range ids {
				requested[id] = true
			}
			for _, record := range records {
				if !requested[record.ID] || returned[record.ID] {
					return finish("partial", "storage_or_metadata_error", domain.Invalid("Invalid profile response identity."))
				}
				returned[record.ID] = true
			}
			if _, e = store.PutContacts(ctx, records); e != nil {
				return storageFailure(e)
			}
			status["returned_count"] = status["returned_count"].(int) + len(records)
			status["missing_count"] = status["missing_count"].(int) + len(ids) - len(records)
		}
		if !more {
			if status["missing_count"].(int) > 0 {
				return finish("partial", "missing_profiles", nil)
			}
			return finish("completed", "known_ids_processed", nil)
		}
	}
	return finish("partial", "page_limit", nil)
}
