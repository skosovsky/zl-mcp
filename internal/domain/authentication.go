package domain

import "errors"

// ErrAuthenticationRequired stops collection until a new local login.
var ErrAuthenticationRequired = errors.New("authentication required; run local login")
