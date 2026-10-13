package mcpserver

// Native bounds and defaults retain the established service contract.
const (
	minimumBearerTokenBytes      = 32
	maximumDiscoveryRequestBytes = 256 << 10
	admissionRetryMillis         = 1000
)
