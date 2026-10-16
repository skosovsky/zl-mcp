package domain

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	ReactionObservedV1 = "zalo.conversation.reaction.observed.v1"
	CallObservedV1     = "zalo.conversation.call.observed.v1"
)

// Activity is a protocol observation, not a message or proof of a complete call
// lifecycle. Unknown facts stay explicit. Media/transport URLs are never retained.
type Activity struct {
	Family           string               `json:"family"`
	SchemaVersion    int                  `json:"schema_version"`
	ConversationType string               `json:"conversation_type"`
	ConversationID   string               `json:"conversation_id"`
	ConversationName *string              `json:"conversation_name"`
	ActorID          string               `json:"actor_id"`
	ActorName        *string              `json:"actor_name"`
	OccurredAt       time.Time            `json:"occurred_at"`
	Direction        string               `json:"direction"`
	SourceID         string               `json:"source_id"`
	IdentityBasis    string               `json:"identity_basis"`
	Source           string               `json:"source"`
	NativeType       *string              `json:"native_type"`
	Reaction         *ReactionObservation `json:"reaction,omitempty"`
	Call             *CallObservation     `json:"call,omitempty"`
	// Native client-only references are local resolution hints, not public IDs.
	ReactionClientMessageIDs []string `json:"-"`
	// Compatibility identity from the earlier candidate's weaker fingerprint.
	LegacyReactionSourceID string `json:"-"`
	// Native message identity prevents replay of a previously projected call
	// message from creating a second notification after a classifier upgrade.
	ProtocolMessageID string `json:"-"`
}

type ReactionObservation struct {
	TargetMessageIDs []string `json:"target_message_ids"`
	Reaction         *string  `json:"reaction"`
	Action           string   `json:"action"`
}
type CallObservation struct {
	CallID       *string `json:"call_id"`
	Status       string  `json:"status"`
	NativeStatus *int32  `json:"native_status"`
	Mode         string  `json:"mode"`
	DurationMS   *int64  `json:"duration_ms"`
}

func (a Activity) Ref() ConversationRef {
	return ConversationRef{Type: a.ConversationType, ID: a.ConversationID}
}
func (a Activity) Profile() string {
	if a.Family == activityReaction {
		return ReactionObservedV1
	}
	if a.Family == activityCall {
		return CallObservedV1
	}
	return ""
}
func IsActivityProfile(profile string) bool {
	return profile == ReactionObservedV1 || profile == CallObservedV1
}
func IsTypedEventProfile(profile string) bool {
	return IsConversationMessageProfile(profile) || IsActivityProfile(profile)
}

// Validate refuses ambiguous unions and unbounded metadata before persistence.
func (a Activity) Validate() error {
	if err := a.validateIdentity(); err != nil {
		return err
	}
	switch a.Family {
	case activityReaction:
		return a.validateReaction()
	case activityCall:
		return a.validateCall()
	default:
		return errors.New("unknown activity family")
	}
}

const (
	activityIDLimit       = 256
	activityNativeLimit   = 64
	activityReactionLimit = 128
	activityTargetsLimit  = 16
	activityTargetIDLimit = 64
	activityMaxDuration   = 9007199254740991
	activityMaxYear       = 9999
)

func (a Activity) validateIdentity() error {
	if a.ProtocolMessageID != "" && (a.Family != activityCall || !activityID(a.ProtocolMessageID)) {
		return errors.New("invalid activity native message identity")
	}
	if !a.Ref().Valid() || !activityID(a.ConversationID) || !activityID(a.ActorID) || !activityID(a.SourceID) ||
		a.SchemaVersion != 1 ||
		a.OccurredAt.IsZero() ||
		a.OccurredAt.Year() < 1 ||
		a.OccurredAt.Year() > activityMaxYear {
		return errors.New("invalid activity identity")
	}
	if !activityOptional(a.ConversationName, activityIDLimit) ||
		!activityOptional(a.ActorName, activityIDLimit) ||
		!activityOptional(a.NativeType, activityNativeLimit) {
		return errors.New("invalid activity metadata")
	}
	if a.NativeType != nil && !activityNative(*a.NativeType) {
		return errors.New("invalid activity native type")
	}
	switch a.Direction {
	case "incoming", "outgoing", stateUnknownText:
	default:
		return errors.New("invalid activity direction")
	}
	if a.Source != "live" && a.Source != "replay" {
		return errors.New("activity source is not live or replay")
	}
	if a.IdentityBasis != "upstream_action_id" && a.IdentityBasis != "observed_fingerprint" {
		return errors.New("invalid activity identity basis")
	}
	if a.LegacyReactionSourceID != "" &&
		(a.Family != activityReaction || a.IdentityBasis != "observed_fingerprint" ||
			!activityID(a.LegacyReactionSourceID) || utf8.RuneCountInString(a.LegacyReactionSourceID) > activityIDLimit) {
		return errors.New("invalid legacy reaction identity")
	}
	return nil
}
func (a Activity) validateReaction() error {
	if a.Reaction == nil || a.Call != nil {
		return errors.New("invalid reaction union")
	}
	r := a.Reaction
	if r.TargetMessageIDs == nil ||
		len(r.TargetMessageIDs)+len(a.ReactionClientMessageIDs) > activityTargetsLimit ||
		!activityOptional(r.Reaction, activityReactionLimit) {
		return errors.New("invalid reaction observation")
	}
	switch r.Action {
	case "add", "remove", "update", stateUnknownText:
	default:
		return errors.New("invalid reaction action")
	}
	seen := map[string]bool{}
	for _, id := range r.TargetMessageIDs {
		if !activityID(id) || utf8.RuneCountInString(id) > activityTargetIDLimit || seen[id] {
			return errors.New("invalid reaction target")
		}
		seen[id] = true
	}
	seen = map[string]bool{}
	for _, id := range a.ReactionClientMessageIDs {
		if !activityID(id) || utf8.RuneCountInString(id) > activityTargetIDLimit || seen[id] {
			return errors.New("invalid reaction client reference")
		}
		seen[id] = true
	}
	return nil
}
func (a Activity) validateCall() error {
	if a.Call == nil || a.Reaction != nil || len(a.ReactionClientMessageIDs) > 0 ||
		!activityOptional(a.Call.CallID, activityIDLimit) {
		return errors.New("invalid call union")
	}
	switch a.Call.Status {
	case stateUnknownText,
		"started",
		"ringing",
		"answered",
		"ended",
		"missed",
		activityCancelled,
		"declined":
	default:
		return errors.New("invalid call status")
	}
	if a.Call.Mode != "voice" && a.Call.Mode != "video" && a.Call.Mode != stateUnknownText {
		return errors.New("invalid call mode")
	}
	if a.Call.DurationMS != nil &&
		(*a.Call.DurationMS < 0 || *a.Call.DurationMS > activityMaxDuration) {
		return errors.New("invalid call duration")
	}
	return nil
}
func activityNative(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		letter := r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z'
		valid := letter || r >= '0' && r <= '9' || strings.ContainsRune("._-", r)
		if !valid {
			return false
		}
	}
	return true
}
func activityID(s string) bool {
	return s != "" && utf8.ValidString(s) && utf8.RuneCountInString(s) <= activityIDLimit
}
func activityOptional(s *string, limit int) bool {
	return s == nil || utf8.ValidString(*s) && utf8.RuneCountInString(*s) <= limit
}
