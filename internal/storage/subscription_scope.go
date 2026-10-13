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
	} else if s.Profile != domain.ConversationMessageCreatedV2 {
		return s, domain.Invalid("Unknown event profile.")
	}
	if err := validateSubscriptionScope(s); err != nil {
		return s, err
	}
	if s.Direction == "" {
		s.Direction = "all"
	}
	if err := validateSubscriptionDirection(s); err != nil {
		return s, err
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

func validateSubscriptionScope(s EventSubscription) error {
	switch s.Scope {
	case "conversation":
		if !(domain.ConversationRef{Type: s.ConversationType, ID: s.GroupID}).Valid() {
			return domain.Invalid("Typed conversation required.")
		}
	case "all":
		if s.GroupID != "" || s.ConversationType != "" {
			return domain.Invalid("Wide scope cannot contain a conversation.")
		}
	case "direct", "group":
		if s.GroupID != "" || s.ConversationType != s.Scope {
			return domain.Invalid("Type scope has conflicting identity.")
		}
	default:
		return domain.Invalid("Unknown subscription scope.")
	}
	return nil
}

func validateSubscriptionDirection(s EventSubscription) error {
	if s.Direction != "all" && s.Direction != "incoming" && s.Direction != "outgoing" {
		return domain.Invalid("Invalid direction filter.")
	}
	if s.Profile != domain.ConversationMessageCreatedV2 && (s.Direction != "all" || s.FirstIncomingOnly) {
		return domain.Invalid("Filtered Events require version 2.")
	}
	if s.FirstIncomingOnly &&
		(s.Direction != "incoming" || !(s.Scope == "direct" || (s.Scope == "conversation" && s.ConversationType == domain.ConversationDirect))) {
		return domain.Invalid("First incoming requires a direct incoming scope.")
	}
	return nil
}
