package errs

import "errors"

// ErrAuthenticationRequired identifies an explicit HTTP 401 handshake response.
// It contains neither the response body nor the authenticated URL.
var ErrAuthenticationRequired = errors.New("authentication required")
