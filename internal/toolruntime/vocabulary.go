package toolruntime

// Shared wire vocabulary retains the existing serialized spellings.
const (
	toolMarkMessagesRead              = "zalo_mark_messages_read"
	toolSetMessageReaction            = "zalo_set_message_reaction"
	toolGetMessageActionStatus        = "zalo_get_message_action_status"
	toolAckSubscriptionEvents         = "zalo_ack_subscription_events"
	toolCancelHistoryImport           = "zalo_cancel_history_import"
	toolGetConversationMessageContext = "zalo_get_conversation_message_context"
	toolGetMessageContext             = "zalo_get_message_context"
	toolImportConversationHistory     = "zalo_import_conversation_history"
	toolJoinGroup                     = "zalo_join_group"
	toolSearchConversationMessages    = "zalo_search_conversation_messages"
	toolSearchMessages                = "zalo_search_messages"
	toolSendDirectMessage             = "zalo_send_direct_message"
)
