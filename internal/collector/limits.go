package collector

import "time"

// Native bounds and defaults retain the established service contract.
const (
	contactProfilePageSize     = 100
	contactPageSize            = 200
	contactRefreshInterval     = 5 * time.Minute
	preloadRefreshInterval     = 5 * time.Minute
	joinPlanLifetime           = 10 * time.Minute
	joinPlanTokenBytes         = 32
	joinPollMillis             = 1000
	joinRateRetryMillis        = 60000
	collectorRequestLimit      = 16 << 10
	collectorHeartbeatInterval = 10 * time.Second
	groupRefreshInterval       = 5 * time.Minute
	maximumListenerBackoff     = 40 * time.Second
)
