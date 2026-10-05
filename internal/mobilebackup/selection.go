package mobilebackup

import (
	"context"
	"errors"
	"strings"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

var ErrSelection = errors.New("invalid mobile backup selection")
var ErrSelectedConversationUnavailable = errors.New("selected conversation absent from mobile archive")

func ArchiveIdentityRequest(ctx context.Context, a PlainArchive) (IdentityRequest, error) {
	var result IdentityRequest
	if ctx == nil || ctx.Err() != nil || len(a.Files) < 1 || len(a.Files) > MaxFiles {
		return result, ErrSelection
	}
	seen := make(map[string]bool, len(a.Files))
	var total uint64
	for _, f := range a.Files {
		if ctx.Err() != nil || !validName(f.Name) || seen[f.Name] || len(f.Data) == 0 || uint64(len(f.Data)) > MaxFileBytes {
			return IdentityRequest{}, ErrSelection
		}
		total += uint64(len(f.Data))
		if total > MaxTotalBytes {
			return IdentityRequest{}, ErrSelection
		}
		seen[f.Name] = true
		id := strings.TrimSuffix(f.Name, ".db")
		group := strings.HasPrefix(id, "group_")
		if group {
			id = strings.TrimPrefix(id, "group_")
		}
		if !canonicalIdentity(id) {
			return IdentityRequest{}, ErrSelection
		}
		if group {
			result.Groups = append(result.Groups, id)
		} else {
			result.Direct = append(result.Direct, id)
		}
	}
	if ctx.Err() != nil {
		return IdentityRequest{}, ErrSelection
	}
	return result, nil
}
func SelectArchiveIndex(ctx context.Context, a PlainArchive, pairs []IdentityPair, ref domain.ConversationRef) (int, error) {
	if !ref.Valid() || !canonicalIdentity(ref.ID) {
		return -1, ErrSelection
	}
	request, e := ArchiveIdentityRequest(ctx, a)
	if e != nil || len(pairs) != len(request.Direct)+len(request.Groups) {
		return -1, ErrSelection
	}
	type typedID struct {
		group bool
		id    string
	}
	expected := make(map[typedID]int, len(a.Files))
	for i, f := range a.Files {
		group := strings.HasPrefix(f.Name, "group_")
		id := strings.TrimSuffix(strings.TrimPrefix(f.Name, "group_"), ".db")
		expected[typedID{group, id}] = i
	}
	sources := make(map[typedID]bool, len(pairs))
	targets := make(map[typedID]bool, len(pairs))
	selected := -1
	for _, p := range pairs {
		if ctx.Err() != nil || !canonicalIdentity(p.Plain) || !canonicalIdentity(p.Session) {
			return -1, ErrSelection
		}
		from, to := typedID{p.Group, p.Plain}, typedID{p.Group, p.Session}
		index, ok := expected[from]
		if !ok || sources[from] || targets[to] {
			return -1, ErrSelection
		}
		sources[from], targets[to] = true, true
		if p.Session == ref.ID && p.Group == (ref.Type == domain.ConversationGroup) {
			selected = index
		}
	}
	if ctx.Err() != nil {
		return -1, ErrSelection
	}
	if selected < 0 {
		return -1, ErrSelectedConversationUnavailable
	}
	return selected, nil
}
