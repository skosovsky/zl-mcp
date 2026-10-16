package zalo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"time"

	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func normalizeReaction(v model.Reaction, source string) (domain.Activity, error) {
	kind := domain.ConversationDirect
	if v.Type == model.ThreadTypeGroup {
		kind = domain.ConversationGroup
	} else if v.Type != model.ThreadTypeUser {
		return domain.Activity{}, errors.New("invalid reaction namespace")
	}
	stamp, err := strconv.ParseInt(v.Data.TS, 10, 64)
	if err != nil || stamp <= 0 {
		return domain.Activity{}, errors.New("invalid reaction timestamp")
	}
	direction := directionIncoming
	if v.IsSelf {
		direction = directionOutgoing
	}
	targets, clients := reactionReferences(v.Data.Content.RMsg)
	var reaction *string
	if v.Data.Content.RIcon != "" {
		value := string(v.Data.Content.RIcon)
		reaction = &value
	}
	// rIcon alone cannot distinguish add vs update; empty is not proof of remove.
	detail := &domain.ReactionObservation{
		TargetMessageIDs: targets,
		Reaction:         reaction,
		Action:           nativeUnknown,
	}
	sourceID := v.Data.ActionID
	basis := "upstream_action_id"
	legacySourceID := ""
	if sourceID == "" || len(sourceID) > 256 {
		identity := []any{
			kind,
			v.ThreadID,
			v.Data.UIDFrom,
			v.Data.MsgID,
			v.Data.CliMsgID,
			v.Data.MsgType,
			v.Data.TS,
			detail,
		}
		if len(clients) > 0 {
			legacyWire, _ := json.Marshal(identity)
			legacyDigest := sha256.Sum256(legacyWire)
			legacySourceID = hex.EncodeToString(legacyDigest[:])
			identity = append(identity, clients)
		}
		wire, _ := json.Marshal(identity)
		digest := sha256.Sum256(wire)
		sourceID = hex.EncodeToString(digest[:])
		basis = "observed_fingerprint"
	}
	actorName := v.Data.DName
	if actorName != nil && len([]rune(*actorName)) > 256 {
		actorName = nil
	}
	a := domain.Activity{
		Family:                   "reaction",
		SchemaVersion:            1,
		ConversationType:         kind,
		ConversationID:           v.ThreadID,
		ConversationName:         nil,
		ActorID:                  v.Data.UIDFrom,
		ActorName:                actorName,
		OccurredAt:               time.UnixMilli(stamp).UTC(),
		Direction:                direction,
		SourceID:                 sourceID,
		IdentityBasis:            basis,
		Source:                   source,
		NativeType:               safeNativeType(v.Data.MsgType),
		Reaction:                 detail,
		Call:                     nil,
		ReactionClientMessageIDs: clients,
		LegacyReactionSourceID:   legacySourceID,
		ProtocolMessageID:        "",
	}
	return a, a.Validate()
}

func deliverReaction(
	v model.Reaction,
	source string,
	onActivity func(domain.Activity) error,
) error {
	if onActivity == nil {
		return nil
	}
	normalized, err := normalizeReaction(v, source)
	if err != nil {
		//nolint:sloglint // Closed diagnostic reason, never native payload/errors/identity.
		slog.Warn(
			"zalo_activity_rejected",
			"family",
			"reaction",
			"reason",
			"invalid_native_activity",
		)
		//nolint:nilerr // Reject one malformed native record without restarting the authenticated listener.
		return nil
	}
	logReactionProjection(v, source, normalized)
	return onActivity(normalized)
}

func logReactionProjection(v model.Reaction, source string, normalized domain.Activity) {
	if source != sourceLive && source != "replay" {
		return
	}
	shape := v.Data.Content.Shape
	references := shape.References
	switch references {
	case "missing", wireNull, "array":
	default:
		references = metadataUnavailable
	}
	fingerprint := sha256.Sum256(
		[]byte(
			normalized.ConversationType + "\x00" + normalized.ConversationID + "\x00" + normalized.SourceID,
		),
	)
	//nolint:sloglint // Closed shape/count allowlist; never reaction values, IDs or native fields.
	slog.Info("zalo_reaction_projection",
		"source", source,
		"reference_shape", references,
		"reference_count", min(max(shape.Count, 0), reactionShapeCountLimit),
		"positive_global_refs", min(max(shape.PositiveGlobal, 0), reactionShapeCountLimit),
		"positive_client_refs", min(max(shape.PositiveClient, 0), reactionShapeCountLimit),
		"has_type", shape.HasType,
		"has_source", shape.HasSource,
		"unknown_field_count", min(max(shape.UnknownFields, 0), reactionShapeCountLimit),
		"record_fingerprint", hex.EncodeToString(fingerprint[:]))
}

func reactionReferences(refs []model.ReactionMessageRef) ([]string, []string) {
	targets := []string{}
	seen := map[string]bool{}
	clients := []string{}
	seenClients := map[string]bool{}
	for _, target := range refs {
		if target.GMsgID <= 0 {
			if target.CMsgID > 0 {
				id := strconv.Itoa(target.CMsgID)
				if !seenClients[id] {
					clients = append(clients, id)
					seenClients[id] = true
				}
			}
			continue
		}
		id := strconv.Itoa(target.GMsgID)
		if !seen[id] {
			targets = append(targets, id)
			seen[id] = true
		}
	}
	sort.Strings(targets)
	sort.Strings(clients)
	return targets, clients
}
