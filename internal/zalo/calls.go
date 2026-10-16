package zalo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

const (
	nativeMissCallAction = "recommened.misscall"
	nativeCallTimeAction = "recommened.calltime"
)

var errCallRecord = errors.New("call observation is not a message")

// A direct summary discriminator was observed in the retained native record.
// Neither its name nor the numeric params prove a call outcome or duration unit.
func isCallContent(d model.TMessage, ref domain.ConversationRef) bool {
	return ref.Type == domain.ConversationDirect && d.MsgType == nativeRecommended &&
		d.Content.Attachment != nil &&
		(d.Content.Attachment.Action == nativeMissCallAction || d.Content.Attachment.Action == nativeCallTimeAction)
}

func callMessageData(message model.Message) (model.TMessage, domain.ConversationRef, bool) {
	switch v := message.(type) {
	case model.UserMessage:
		ref := domain.ConversationRef{Type: domain.ConversationDirect, ID: v.ThreadID()}
		return v.Data, ref, isCallContent(v.Data, ref)
	default:
		return model.TMessage{}, domain.ConversationRef{}, false
	}
}

func normalizeCall(message model.Message, source string) (domain.Activity, error) {
	data, ref, matched := callMessageData(message)
	if !matched || !validCallParams(data.Content.Attachment.Action, data.Content.Attachment.Params) {
		return domain.Activity{}, errors.New("unsupported call observation shape")
	}
	stamp, err := strconv.ParseInt(data.TS, 10, 64)
	if err != nil || stamp <= 0 || data.MsgID == "" || !ref.Valid() || data.UIDFrom == "" {
		return domain.Activity{}, errors.New("invalid call observation identity")
	}
	identity, _ := json.Marshal(
		[]string{ref.Type, ref.ID, data.MsgID, data.UIDFrom, data.TS, data.Content.Attachment.Action},
	)
	digest := sha256.Sum256(identity)
	direction := directionIncoming
	if message.IsSelf() {
		direction = directionOutgoing
	}
	a := domain.Activity{
		Family:           "call",
		SchemaVersion:    1,
		ConversationType: ref.Type,
		ConversationID:   ref.ID,
		ConversationName: nil,
		ActorID:          data.UIDFrom,
		ActorName:        boundedContentString(data.DName, callActorNameLimit),
		OccurredAt: time.UnixMilli(stamp).
			UTC(),
		Direction:     direction,
		SourceID:      hex.EncodeToString(digest[:]),
		IdentityBasis: "observed_fingerprint",
		Source:        source,
		NativeType:    safeNativeType(data.MsgType),
		Reaction:      nil,
		Call: &domain.CallObservation{
			CallID:       nil,
			Status:       nativeUnknown,
			NativeStatus: nil,
			Mode:         nativeUnknown,
			DurationMS:   nil,
		},
		ReactionClientMessageIDs: nil,
		LegacyReactionSourceID:   "",
		ProtocolMessageID:        data.MsgID,
	}
	return a, a.Validate()
}

func validCallParams(action, raw string) bool {
	if len(raw) > 64<<10 || !json.Valid([]byte(raw)) {
		return false
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var params map[string]any
	if decoder.Decode(&params) != nil {
		return false
	}
	keys := []string{nativeCallIDField, nativeCallTypeField, nativeCallDurationField}
	if action == nativeMissCallAction {
		keys = append(keys, nativeCallReasonField)
	} else if action != nativeCallTimeAction {
		return false
	}
	for _, key := range keys {
		if _, ok := params[key].(json.Number); !ok {
			return false
		}
	}
	return true
}

func deliverIncomingActivity(message model.Message, source string,
	onMessage func(domain.Message) error, onActivity func(domain.Activity) error) error {
	if _, _, matched := callMessageData(message); !matched {
		return deliverIncoming(message, source, onMessage)
	}
	a, err := normalizeCall(message, source)
	if err != nil {
		//nolint:sloglint // Closed reason only; native payload, IDs and errors stay private.
		slog.Warn("zalo_activity_rejected", "family", "call", nativeCallReasonField, "invalid_native_activity")
		//nolint:nilerr // Reject only this malformed call without restarting the authenticated listener.
		return nil
	}
	if onActivity == nil {
		return nil
	}
	return onActivity(a)
}

func logSkippedNonMessage(source string, err error) {
	if !errors.Is(err, errCallRecord) {
		logControlRecord(source)
		return
	}
	switch source {
	case sourceLive, sourceReplay, "history", "preload":
	default:
		source = nativeUnknown
	}
	//nolint:sloglint // Closed source/family only, no native payload or identity.
	slog.Info("zalo_activity_skipped", "family", "call", "source", source)
}
