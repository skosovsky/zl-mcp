package messaging

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	managersendOnceObservedFinishTimeout = 3 * time.Second
	managersendOnceObservedSendTimeout   = 30 * time.Second
)
