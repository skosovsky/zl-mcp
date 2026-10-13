package membership

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	handlerInspectTimeout = 6 * time.Second
	handlerJoinTimeout    = 6 * time.Second
	handlerStatusTimeout  = 6 * time.Second
)
