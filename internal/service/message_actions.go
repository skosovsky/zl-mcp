package service

import (
	"context"
	"encoding/json"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	messagingapp "github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/messageactions"
)

const messageActionReadKind = "read"

// The caller holds the source lock. Status reads need neither an active session
// nor write permission. Writes follow service and current session lifetimes.
func (p *membershipPort) messageActionCall(ctx context.Context, method string, args any) (map[string]any, error) {
	body, err := json.Marshal(args)
	if err != nil {
		return nil, domain.Invalid("Invalid message action arguments.")
	}
	schema, err := contracts.Compile(method, "input")
	if err != nil {
		return nil, err
	}
	var value any
	if json.Unmarshal(body, &value) != nil || schema.Validate(value) != nil {
		return nil, domain.Invalid("Invalid message action arguments.")
	}
	manager := messageactions.Manager{
		Store:         p.store,
		Source:        p.actions,
		ReadChats:     p.readChats,
		ReactionChats: p.reactionChats,
	}
	result, err := p.executeMessageAction(ctx, &manager, method, body)
	if err != nil {
		return nil, messageactions.PublicError(err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	err = json.Unmarshal(raw, &out)
	return out, err
}

func (p *membershipPort) executeMessageAction(
	ctx context.Context,
	manager *messageactions.Manager,
	method string,
	body []byte,
) (domain.MessageActionOperation, error) {
	if method == "zalo_get_message_action_status" {
		var request struct {
			RequestID string `json:"request_id"`
		}
		if err := json.Unmarshal(body, &request); err != nil {
			return domain.MessageActionOperation{}, domain.Invalid("Invalid message action status arguments.")
		}
		return manager.Status(ctx, request.RequestID)
	}
	intent, err := publicMessageActionIntent(method, body)
	if err != nil {
		return domain.MessageActionOperation{}, err
	}
	operation, cancel := context.WithCancel(ctx)
	defer cancel()
	if p.actionContext != nil {
		stop := context.AfterFunc(p.actionContext, cancel)
		defer stop()
		if p.actionContext.Err() != nil {
			cancel()
		}
	}
	return manager.Execute(operation, intent)
}

func publicMessageActionIntent(method string, body []byte) (domain.MessageActionIntent, error) {
	switch method {
	case "zalo_mark_messages_read":
		var request messagingapp.MarkReadRequest
		if err := json.Unmarshal(body, &request); err != nil {
			return domain.MessageActionIntent{}, domain.Invalid("Invalid read receipt arguments.")
		}
		return domain.MessageActionIntent{
			RequestID:    request.RequestID,
			Kind:         messageActionReadKind,
			Reaction:     "",
			Plan:         messageActionReadKind,
			Conversation: domain.ConversationRef{Type: request.ConversationType, ID: request.ConversationID},
			MessageIDs:   request.MessageIDs,
		}, nil
	case "zalo_set_message_reaction":
		var request messagingapp.ReactionRequest
		if err := json.Unmarshal(body, &request); err != nil {
			return domain.MessageActionIntent{}, domain.Invalid("Invalid reaction arguments.")
		}
		plan := "clear"
		if request.Action == "set" {
			plan = "clear_add"
		}
		return domain.MessageActionIntent{
			RequestID:    request.RequestID,
			Kind:         "reaction",
			Plan:         plan,
			Conversation: domain.ConversationRef{Type: request.ConversationType, ID: request.ConversationID},
			MessageIDs:   []string{request.MessageID},
			Reaction:     request.Reaction,
		}, nil
	default:
		return domain.MessageActionIntent{}, domain.Invalid("Unknown message action.")
	}
}
