package shared

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	observeQueueReadTimeout = 100 * time.Millisecond
)
