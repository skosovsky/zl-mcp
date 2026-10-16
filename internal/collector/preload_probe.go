package collector

import (
	"context"
	"errors"
	"net"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// probePreload is a trusted local read, not an MCP tool or ingestion operation.
func probePreload(parent context.Context, source domain.ConversationPreloadSource) map[string]any {
	return probePreloadRecord(parent, source, nil)
}

type preloadProbeTarget struct {
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
	MessageID        string `json:"message_id"`
}

func probePreloadRecord(
	parent context.Context,
	source domain.ConversationPreloadSource,
	target *preloadProbeTarget,
) map[string]any {
	result := map[string]any{
		"source_status":             stateUnsupportedText,
		"error_category":            reasonSourceUnsupported,
		"source_code":               nil,
		"error_reason":              nil,
		"metadata_count":            0,
		"direct_message_count":      0,
		"group_message_count":       0,
		"unsupported_message_count": 0,
		"direct_messages_available": false,
		"group_messages_available":  false,
		"metadata_persisted":        false,
		"messages_persisted":        false,
		"catalog_complete":          false,
		"history_complete":          false,
	}
	if target != nil {
		result["record_probe"] = map[string]any{"matched": false, "ambiguous": false, "shape": nil}
	}
	if source == nil {
		return result
	}
	ctx, cancel := context.WithTimeout(parent, probePreloadTimeout)
	defer cancel()
	page, err := source.ConversationPreload(ctx)
	if err != nil {
		projectPreloadFailure(result, err)
		return result
	}
	direct, group := 0, 0
	if len(page.Entries) > 5000 || page.UnsupportedMessageCount < 0 ||
		len(page.Messages)+page.UnsupportedMessageCount > 1000 {
		result["source_status"] = stateFailedText
		result["error_category"] = reasonInvalidSourcePage
		return result
	}
	for _, message := range page.Messages {
		switch message.Ref().Type {
		case domain.ConversationDirect:
			direct++
		case domain.ConversationGroup:
			group++
		default:
			result["source_status"] = stateFailedText
			result["error_category"] = reasonInvalidSourcePage
			return result
		}
	}
	projectExactPreloadRecord(result, page.Messages, target)
	result["source_status"] = "available"
	result["error_category"] = nil
	result["metadata_count"] = len(page.Entries)
	result["direct_message_count"] = direct
	result["group_message_count"] = group
	result["unsupported_message_count"] = page.UnsupportedMessageCount
	result["direct_messages_available"] = page.DirectMessagesAvailable
	result["group_messages_available"] = page.GroupMessagesAvailable
	return result
}

func projectExactPreloadRecord(result map[string]any, messages []domain.Message, target *preloadProbeTarget) {
	if target == nil {
		return
	}
	matched := 0
	var shape *domain.ContentShape
	for _, message := range messages {
		if message.Ref().Type == target.ConversationType && message.Ref().ID == target.ConversationID &&
			message.ID == target.MessageID {
			matched++
			shape = message.ProtocolContentShape
		}
	}
	if matched != 1 {
		shape = nil
	}
	result["record_probe"] = map[string]any{"matched": matched == 1, "ambiguous": matched > 1, "shape": shape}
}

func projectPreloadFailure(result map[string]any, err error) {
	result["source_status"] = stateFailedText
	category := stateUnknownText
	var failure *domain.HistorySourceFailure
	var network net.Error
	switch {
	case errors.Is(err, domain.ErrConversationPreloadUnsupported):
		result["source_status"] = stateUnsupportedText
		category = reasonSourceUnsupported
	case errors.Is(err, domain.ErrAuthenticationRequired):
		result["source_status"] = reasonAuthRequired
		category = reasonAuthRequired
	case errors.Is(err, domain.ErrHistoryInvalidPage):
		category = reasonInvalidSourcePage
		if pageFailure, ok := errors.AsType[*domain.HistoryPageFailure](err); ok {
			result["error_reason"] = pageFailure.Reason()
		}
	case errors.Is(err, context.DeadlineExceeded):
		category = "timeout"
	case errors.Is(err, context.Canceled):
		category = "cancelled"
	case errors.As(err, &failure):
		category = "api_error"
		if failure.APICode != nil {
			result["source_code"] = *failure.APICode
		}
	case errors.As(err, &network):
		category = "network"
	}
	result["error_category"] = category
}
