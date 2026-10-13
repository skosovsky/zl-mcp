package netpolicy

import "time"

// Native bounds and defaults retain the established service contract.
const (
	dialTimeout           = 5 * time.Second
	tlsHandshakeTimeout   = 5 * time.Second
	responseHeaderTimeout = 5 * time.Second
)
