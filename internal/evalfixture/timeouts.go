package evalfixture

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	fixtureCloseTimeout = 5 * time.Second
)
