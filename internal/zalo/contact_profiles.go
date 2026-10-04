package zalo

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func validateProfileIDs(ids []string) error {
	if len(ids) < 1 || len(ids) > 100 {
		return domain.Invalid("Profile request requires 1–100 exact peer IDs.")
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if id == "" || len(id) > 256 || seen[id] {
			return domain.Invalid("Invalid or duplicate profile peer ID.")
		}
		seen[id] = true
	}
	for _, id := range ids {
		if seen[id+"_0"] {
			return domain.Invalid("Ambiguous profile version keys.")
		}
	}
	return nil
}

func (c *Client) ContactProfiles(ctx context.Context, ids []string) ([]domain.Contact, error) {
	if err := validateProfileIDs(ids); err != nil {
		return nil, err
	}
	// The pinned SDK mutates its variadic ID slice by adding version suffixes.
	// Keep the caller's exact identities and allowlist independent of that change.
	response, err := c.api.GetUserInfo(ctx, append([]string(nil), ids...)...)
	if err != nil {
		if errors.Is(err, errs.ErrAuthenticationRequired) {
			return nil, domain.ErrAuthenticationRequired
		}
		return nil, err
	}
	return profileContacts(ids, response)
}

func profileContacts(ids []string, response *api.GetUserInfoResponse) ([]domain.Contact, error) {
	if err := validateProfileIDs(ids); err != nil {
		return nil, err
	}
	if response == nil {
		return nil, fmt.Errorf("profile response unavailable")
	}
	allowed := map[string]string{}
	for _, id := range ids {
		allowed[id] = id
		allowed[id+"_0"] = id
	}
	contacts := map[string]domain.Contact{}
	for key, user := range response.ChangedProfiles {
		id, ok := allowed[key]
		if !ok || (user.UserId != "" && user.UserId != id) {
			return nil, fmt.Errorf("profile response identity does not match request")
		}
		if _, duplicate := contacts[id]; duplicate {
			return nil, fmt.Errorf("duplicate profile response identity")
		}
		user.UserId = id
		contacts[id] = contactMetadata(user)
	}
	ordered := make([]string, 0, len(contacts))
	for id := range contacts {
		ordered = append(ordered, id)
	}
	sort.Strings(ordered)
	result := make([]domain.Contact, 0, len(contacts))
	for _, id := range ordered {
		result = append(result, contacts[id])
	}
	return result, nil
}
