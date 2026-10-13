package historyimport

import "time"

// Native bounds and defaults retain the established service contract.
const (
	historyPollInterval       = 500 * time.Millisecond
	historyPollBatch          = 20
	mobilePhonePhaseBudget    = 180 * time.Second
	mobileDownloadPhaseBudget = 120 * time.Second
	mobileReadPhaseBudget     = 30 * time.Second
	cloudReadPhaseBudget      = 30 * time.Second
)
