package domain

import "context"

// Contact exposes only directory metadata; protocol profile fields stay upstream.
type Contact struct {
	ID         string
	Name       string
	Aliases    []string
	Friendship string
}

// ContactSource is optional for sources which support a paginated directory.
// A short page exhausts the observed source, not the conversation history.
type ContactSource interface {
	ContactsPage(ctx context.Context, page, limit int) ([]Contact, error)
}

// ContactProfileSource enriches only exact known peer IDs, never a name search.
type ContactProfileSource interface {
	ContactProfiles(context.Context, []string) ([]Contact, error)
}
