package main

import "time"

// Operation deadlines retain the existing native/IPC request budgets.
const (
	runMeasurementModeTimeout = 20 * time.Minute
)
