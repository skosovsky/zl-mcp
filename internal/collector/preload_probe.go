package collector

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// probePreload is a trusted local read, not an MCP tool or ingestion operation.
func probePreload(parent context.Context, source domain.ConversationPreloadSource) map[string]any {
	result := map[string]any{"source_status": "unsupported", "error_category": "source_unsupported", "source_code": nil, "error_reason": nil, "metadata_count": 0, "direct_message_count": 0, "group_message_count": 0, "unsupported_message_count": 0, "direct_messages_available": false, "group_messages_available": false, "metadata_persisted": false, "messages_persisted": false, "catalog_complete": false, "history_complete": false}
	if source == nil {
		return result
	}
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	page, err := source.ConversationPreload(ctx)
	if err != nil {
		result["source_status"] = "failed"
		category := "unknown"
		var failure *domain.HistorySourceFailure
		var network net.Error
		switch {
		case errors.Is(err, domain.ErrConversationPreloadUnsupported):
			result["source_status"] = "unsupported"
			category = "source_unsupported"
		case errors.Is(err, domain.ErrAuthenticationRequired):
			result["source_status"] = "auth_required"
			category = "auth_required"
		case errors.Is(err, domain.ErrHistoryInvalidPage):
			category = "invalid_source_page"
			var pageFailure *domain.HistoryPageFailure
			if errors.As(err, &pageFailure) {
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
		return result
	}
	direct, group := 0, 0
	if len(page.Entries) > 5000 || page.UnsupportedMessageCount < 0 || len(page.Messages)+page.UnsupportedMessageCount > 1000 {
		result["source_status"] = "failed"
		result["error_category"] = "invalid_source_page"
		return result
	}
	for _, message := range page.Messages {
		switch message.Ref().Type {
		case domain.ConversationDirect:
			direct++
		case domain.ConversationGroup:
			group++
		default:
			result["source_status"] = "failed"
			result["error_category"] = "invalid_source_page"
			return result
		}
	}
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
