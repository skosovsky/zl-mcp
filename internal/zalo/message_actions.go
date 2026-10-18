package zalo

import (
	"context"
	"errors"
	"strconv"

	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/model"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func actionThread(ref domain.ConversationRef) (model.ThreadType, error) {
	switch ref.Type {
	case domain.ConversationDirect:
		return model.ThreadTypeUser, nil
	case domain.ConversationGroup:
		return model.ThreadTypeGroup, nil
	default:
		return model.ThreadTypeUser, domain.Invalid("Typed conversation required.")
	}
}

// MarkSeen sends only observed native metadata for incoming messages. It makes
// no assertion about point-only read state or sender-visible UI confirmation.
func (c *Client) MarkSeen(ctx context.Context, targets []domain.MessageActionTarget) (domain.ReadReceiptResult, error) {
	if len(targets) == 0 || len(targets) > 50 {
		return domain.ReadReceiptResult{}, domain.Invalid("Read receipts require 1–50 targets.")
	}
	ref := targets[0].Conversation
	kind, err := actionThread(ref)
	if err != nil {
		return domain.ReadReceiptResult{}, err
	}
	own := c.AccountID()
	if own == "" {
		return domain.ReadReceiptResult{}, domain.ErrAuthenticationRequired
	}
	native := make([]model.OutboundMessage, len(targets))
	for i, t := range targets {
		p := t.Seen
		if t.Conversation != ref || !ref.Valid() || p == nil || p.MessageType == "" ||
			!domain.ValidActionMessageID(t.MessageID) || !domain.ValidActionMessageID(t.ClientMessageID) ||
			p.ClientMessageID != t.ClientMessageID ||
			t.SenderID == "" ||
			t.SenderID == own ||
			(ref.Type == domain.ConversationDirect && (t.SenderID != ref.ID || p.RecipientID != own)) ||
			(ref.Type == domain.ConversationGroup && p.RecipientID != ref.ID) {
			return domain.ReadReceiptResult{}, domain.Invalid("Read receipt target lacks matching native provenance.")
		}
		stamp, parseErr := strconv.ParseInt(p.Timestamp, 10, 64)
		if parseErr != nil || stamp <= 0 {
			return domain.ReadReceiptResult{}, domain.Invalid("Read receipt timestamp is unavailable.")
		}
		native[i] = model.OutboundMessage{
			MsgID:    t.MessageID,
			CliMsgID: t.ClientMessageID,
			UIDFrom:  t.SenderID,
			IDTo:     p.RecipientID,
			MsgType:  p.MessageType,
			ST:       p.ST,
			AT:       p.AT,
			CMD:      p.CMD,
			TS:       stamp,
		}
	}
	response, err := c.api.SendSeenEvent(ctx, native, kind)
	if err != nil {
		return domain.ReadReceiptResult{}, classifyMessageActionError(err)
	}
	// Status semantics require native acceptance; do not label arbitrary decoded
	// values as sender-visible read state. Only a nonnil protocol reply is known.
	if response == nil {
		return domain.ReadReceiptResult{}, errors.New("upstream read receipt result is ambiguous")
	}
	result := domain.ReadReceiptResult{Status: nil}
	if response.HasStatus {
		result.Status = new(response.Status)
	}
	// Present status zero was observed with sender-visible read state in approved
	// direct and group cases. It means protocol acceptance, never per-message UI
	// isolation. Historical unknown results remain unchanged in the action ledger.
	if response.HasStatus && response.Status == 0 {
		return result, nil
	}
	return result, domain.ErrUnverifiedReadStatus
}

func classifyMessageActionError(err error) error {
	if confirmedSendFailure(err) {
		return domain.ErrMessageActionRejected
	}
	return errors.New("upstream message action result is ambiguous")
}

// React accepts only service-selected native reaction data. A public catalog is
// enforced at the application boundary; raw rType/source never come from a model.
func (c *Client) React(ctx context.Context, target domain.MessageActionTarget, code string, remove bool) error {
	icon, valid := nativeReaction(code)
	if remove {
		icon = model.ReactionNone
		valid = code == ""
	}
	if !valid {
		return domain.Invalid("Select a supported reaction code or explicit removal.")
	}
	kind, err := actionThread(target.Conversation)
	if err != nil {
		return err
	}
	if !target.Conversation.Valid() || target.MessageID == "" || target.ClientMessageID == "" {
		return domain.Invalid("Reaction target provenance is unavailable.")
	}
	response, err := c.api.AddReaction(
		ctx,
		api.AddReactionDestination{
			ThreadID: target.Conversation.ID,
			Type:     kind,
			Data:     api.AddReactionData{MsgID: target.MessageID, CliMsgID: target.ClientMessageID},
		},
		model.NewReactionData(icon),
	)
	if err != nil {
		return classifyMessageActionError(err)
	}
	if response == nil || len(response.MsgIDs) == 0 {
		return errors.New("upstream reaction result is ambiguous")
	}
	return nil
}
