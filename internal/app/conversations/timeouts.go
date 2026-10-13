package conversations

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	handlerGroupRefreshTimeout = 5 * time.Second
)
