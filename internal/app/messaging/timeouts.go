package messaging

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	handlerSendTimeout   = 35 * time.Second
	handlerStatusTimeout = 35 * time.Second
)
