package logging

import "time"

// Native bounds and defaults retain the established service contract.
const (
	startupLogSizeMB        = 5
	startupLogBackups       = 3
	startupLogDrainInterval = 10 * time.Millisecond
)
