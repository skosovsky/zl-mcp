package storage

import "github.com/skosovsky/zl-mcp/internal/domain"

func normalizeSubscription(s EventSubscription) (EventSubscription, error) {
	if s.Profile == "" {
		s.Profile = domain.LegacyMessageCreated
	}
	if s.Scope == "" {
		s.Scope = "conversation"
	}
	if s.Profile == domain.LegacyMessageCreated {
		if s.ConversationType == "" {
			s.ConversationType = domain.ConversationGroup
		}
		if s.Scope != "conversation" || s.ConversationType != domain.ConversationGroup {
			return s, domain.Invalid("Legacy event requires one group.")
		}
	} else if s.Profile != domain.ConversationMessageCreated {
		return s, domain.Invalid("Unknown event profile.")
	}
	switch s.Scope {
	case "conversation":
		if !(domain.ConversationRef{Type: s.ConversationType, ID: s.GroupID}).Valid() {
			return s, domain.Invalid("Typed conversation required.")
		}
	case "all":
		if s.GroupID != "" || s.ConversationType != "" {
			return s, domain.Invalid("Wide scope cannot contain a conversation.")
		}
	case "direct", "group":
		if s.GroupID != "" || s.ConversationType != s.Scope {
			return s, domain.Invalid("Type scope has conflicting identity.")
		}
	default:
		return s, domain.Invalid("Unknown subscription scope.")
	}
	return s, nil
}
func subscriptionMatches(profile, scope, kind, id string, ref domain.ConversationRef) bool {
	if profile == domain.LegacyMessageCreated && ref.Type != domain.ConversationGroup {
		return false
	}
	switch scope {
	case "all":
		return true
	case "direct", "group":
		return ref.Type == scope
	case "conversation":
		return kind == ref.Type && id == ref.ID
	default:
		return false
	}
}
