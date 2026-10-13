package events

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	subscriptionManagerverifySubscriptionCallbackVerifyTimeout = 10 * time.Second
	workerdeliverClaimedFinishTimeout                          = 5 * time.Second
	workerdeliverClaimedTimeout                                = 10 * time.Second
)
