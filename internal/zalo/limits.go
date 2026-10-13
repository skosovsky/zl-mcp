package zalo

import "time"

// Native bounds and defaults retain the established service contract.
const (
	nativeHTTPTimeout           = 30 * time.Second
	identityBatchSize           = 50
	listenerDiagnosticsInterval = 15 * time.Second
	mobileOfferRSAKeyBits       = 2048
	maximumReplayPages          = 100
	maximumReplayMessages       = 10000
)
