package storage

import "github.com/skosovsky/zl-mcp/internal/domain"

func normalizeSubscription(s EventSubscription) (EventSubscription, error) {
	if s.Profile == "" {
		s.Profile = domain.LegacyMessageCreated
	}
	if s.Scope == "" {
		s.Scope = wireConversation
	}
	if s.Profile == domain.LegacyMessageCreated {
		if s.ConversationType == "" {
			s.ConversationType = domain.ConversationGroup
		}
		if s.Scope != wireConversation || s.ConversationType != domain.ConversationGroup {
			return s, domain.Invalid("Legacy event requires one group.")
		}
	} else if !domain.IsTypedEventProfile(s.Profile) {
		return s, domain.Invalid("Unknown event profile.")
	}
	if s.Profile == domain.CallObservedV1 && s.Scope != domain.ConversationDirect &&
		(s.Scope != wireConversation || s.ConversationType != domain.ConversationDirect) {
		return s, domain.Invalid("Call observations currently support direct chats only.")
	}
	if err := validateSubscriptionScope(s); err != nil {
		return s, err
	}
	if s.Direction == "" {
		s.Direction = directionAll
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
	case directionAll:
		return true
	case domain.ConversationDirect, domain.ConversationGroup:
		return ref.Type == scope
	case wireConversation:
		return kind == ref.Type && id == ref.ID
	default:
		return false
	}
}

func validateSubscriptionScope(s EventSubscription) error {
	switch s.Scope {
	case wireConversation:
		if !(domain.ConversationRef{Type: s.ConversationType, ID: s.GroupID}).Valid() {
			return domain.Invalid("Typed conversation required.")
		}
	case directionAll:
		if s.GroupID != "" || s.ConversationType != "" {
			return domain.Invalid("Wide scope cannot contain a conversation.")
		}
	case domain.ConversationDirect, domain.ConversationGroup:
		if s.GroupID != "" || s.ConversationType != s.Scope {
			return domain.Invalid("Type scope has conflicting identity.")
		}
	default:
		return domain.Invalid("Unknown subscription scope.")
	}
	return nil
}

func validateSubscriptionDirection(s EventSubscription) error {
	if s.Direction != directionAll && s.Direction != directionIncoming && s.Direction != directionOutgoing {
		return domain.Invalid("Invalid direction filter.")
	}
	if domain.IsActivityProfile(s.Profile) && s.FirstIncomingOnly {
		return domain.Invalid("Activity has no first incoming message fact.")
	}
	if !domain.IsTypedEventProfile(s.Profile) && (s.Direction != directionAll || s.FirstIncomingOnly) {
		return domain.Invalid("Filtered Events require version 2.")
	}
	if s.FirstIncomingOnly &&
		(s.Direction != directionIncoming || (s.Scope != domain.ConversationDirect && (s.Scope != wireConversation || s.ConversationType != domain.ConversationDirect))) {
		return domain.Invalid("First incoming requires a direct incoming scope.")
	}
	return nil
}
