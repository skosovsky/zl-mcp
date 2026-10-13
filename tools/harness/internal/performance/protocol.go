package performance

import "time"

// Measurement protocol and accepted budgets retain the reviewed numeric values.
const (
	measurementRuns               = 5
	callsPerKindPerRun            = 1000
	measurementWarmupCalls        = 50
	parallelMeasurementKinds      = 3
	measurementRecords            = measurementRuns * parallelMeasurementKinds
	measurementPacingMillis       = 110
	measurementRunBudgetSeconds   = 1200
	measurementTrialBudgetSeconds = 900
	historicalP95Ratio            = 1.15
	maxAllocationRatio            = 2.0
	maxAllocBytesRatio            = 1.20
	historicalAllocationRatio     = 1.20
	commandOutputLimit            = 4 << 20
	processDrainBufferBytes       = 8192
	hostCommandTimeout            = 30 * time.Second
	candidateBuildTimeout         = 180 * time.Second
)
