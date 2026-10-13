package storage

import "time"

// Native bounds and defaults retain the established service contract.
const (
	historyPreviewRunes            = 150
	maximumContactBatch            = 200
	maximumDeliveryJobs            = 100000
	maximumDeliveryBytes           = 256 << 20
	maximumDeliveryAttempts        = 256
	deliveryLease                  = 30 * time.Second
	maximumSubscriptions           = 100
	maximumHistoryPageRecords      = 50
	cloudHistoryClaimBudget        = 30 * time.Second
	mobileHistoryClaimBudget       = 180 * time.Second
	maximumMobileAttempts          = 10000
	maximumPreloadDirectoryEntries = 5000
	cursorKeyBytes                 = 32
	contextDisplayRunes            = 8000
	minimumContextExcerptRunes     = 1000
)
