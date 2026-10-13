package events

import "time"

// Native bounds and defaults retain the established service contract.
const (
	maximumVerifiedCallbacks  = 512
	callbackRequestTimeout    = 10 * time.Second
	maximumCallbackBodyBytes  = 262144
	callbackChallengeBytes    = 32
	callbackResponseReadLimit = 65537
	retryJitterChoices        = 400001
	retryJitterCenter         = 200000
	retryJitterScale          = 1000000
	workerErrorBuffer         = 5
	workerCompletionBuffer    = 4
	deliveryPollInterval      = 250 * time.Millisecond
)
