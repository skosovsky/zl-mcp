package history

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	handlerCancelTimeout = 35 * time.Second
	handlerStartTimeout  = 35 * time.Second
	handlerStatusTimeout = 35 * time.Second
)
