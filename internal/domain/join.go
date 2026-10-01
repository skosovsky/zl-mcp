package domain

import "errors"

// ErrJoinPendingApproval means the upstream explicitly requires administrator
// approval for this join request. It never permits repeating the mutation.
var ErrJoinPendingApproval = errors.New("join pending administrator approval")
