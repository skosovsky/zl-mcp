package collector

// Shared wire vocabulary retains the existing serialized spellings.
const (
	codeStorageError          = "STORAGE_ERROR"
	codeUpstreamUnavailable   = "UPSTREAM_UNAVAILABLE"
	reasonAuthRequired        = "auth_required"
	reasonCancelledOrDeadline = "cancelled_or_deadline"
	fieldCode                 = "code"
	stateFailedText           = "failed"
	fieldGroupID              = "group_id"
	reasonInvalidSourcePage   = "invalid_source_page"
	fieldLastAttemptAt        = "last_attempt_at"
	fieldLastSuccessAt        = "last_success_at"
	fieldMessage              = "message"
	statePartialText          = "partial"
	stateRunningText          = "running"
	reasonSourceUnsupported   = "source_unsupported"
	fieldStatus               = "status"
	fieldStopReason           = "stop_reason"
	stateUnknownText          = "unknown"
	stateUnsupportedText      = "unsupported"
)
