package zalo

import (
	"context"
	"errors"
	"fmt"

	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/model"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func (c *Client) ContactsPage(ctx context.Context, page, limit int) ([]domain.Contact, error) {
	if page < 1 || limit < 1 || limit > 200 {
		return nil, domain.Invalid("Invalid contact page.")
	}
	users, err := c.api.GetAllFriends(ctx, model.OffsetPaginationOptions{Page: page, Count: limit})
	if err != nil {
		if errors.Is(err, errs.ErrAuthenticationRequired) {
			return nil, domain.ErrAuthenticationRequired
		}
		return nil, err
	}
	if users == nil {
		return nil, fmt.Errorf("contacts response unavailable")
	}
	if len(*users) > limit {
		return nil, fmt.Errorf("contacts response exceeds requested limit")
	}
	contacts := make([]domain.Contact, 0, len(*users))
	for _, user := range *users {
		contacts = append(contacts, contactMetadata(user))
	}
	return contacts, nil
}

func contactMetadata(user model.User) domain.Contact {
	name := user.DisplayName
	if name == "" {
		name = user.ZaloName
	}
	friendship := "unknown"
	// The upstream int field cannot distinguish an absent isFr from zero. Do
	// not classify absent/invalid profile metadata as a confirmed non-friend.
	if user.IsFr == 1 {
		friendship = "friend"
	}
	return domain.Contact{ID: user.UserId, Name: name, Aliases: []string{user.DisplayName, user.ZaloName}, Friendship: friendship}
}
