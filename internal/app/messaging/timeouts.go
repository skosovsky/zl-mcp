package messaging

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	// ReactionTimeout bounds clear+add plus independent durable finalization.
	ReactionTimeout      = 70 * time.Second
	handlerSendTimeout   = 35 * time.Second
	handlerStatusTimeout = 35 * time.Second
)
