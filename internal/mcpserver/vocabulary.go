package mcpserver

// Shared wire vocabulary retains the existing serialized spellings.
const (
	protocolVersion           = "2026-07-28"
	codeStorageError          = "STORAGE_ERROR"
	codeToolError             = "TOOL_ERROR"
	contentTypeJSON           = "application/json"
	wireMessages              = "messages"
	contentTypeText           = "text/plain"
	methodToolsList           = "tools/list"
	wireZalo                  = "zalo"
	toolAckSubscriptionEvents = "zalo_ack_subscription_events"
	toolCancelHistoryImport   = "zalo_cancel_history_import"
)
