package evaluation

import "time"

// Local limits preserve existing codec, executable and synthetic contracts.
const (
	measurementWarmupCalls       = 50
	measurementPacingMillis      = 110
	minimumMeasuredCalls         = 1000
	measurementPacingInterval    = 110 * time.Millisecond
	p95Percent                   = 95
	percentileRoundUp            = 99
	percentileScale              = 100
	maximumModelToolCalls        = 30
	modelDriverHeaderTimeout     = 5 * time.Second
	maximumEvaluationOutputBytes = 4 << 20
	maximumSkillBytes            = 64 << 10
	modelCaseBudgetSeconds       = 180
	offlineCaseBudgetSeconds     = 60
	baseContractAssertions       = 6
	maximumEvidenceEvents        = 3
	maximumEvidenceBytes         = 8192
	offlineCaseTimeout           = 60 * time.Second
	evaluationCleanupTimeout     = 5 * time.Second
)
