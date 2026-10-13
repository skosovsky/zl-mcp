package evalfixture

import "time"

// Local limits preserve existing codec, executable and synthetic contracts.
const (
	fixtureHeartbeatInterval  = 10 * time.Second
	fixtureRetentionDays      = 90
	fixtureHeaderTimeout      = 5 * time.Second
	fixtureArchiveBudgetBytes = 8 << 20
	fixtureKeyPatternRepeats  = 4
)
