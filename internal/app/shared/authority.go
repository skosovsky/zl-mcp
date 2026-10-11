package shared

import "context"

// ReadIdentity is host-owned local identity, never tool argument data.
// Account is empty only for an already-supported unbound local corpus; owner
// capability binding still isolates that corpus from any other runtime.
type ReadIdentity struct {
	Principal      string
	Account        string
	PolicyRevision string
}
type IdentityPort interface {
	ReadIdentity(context.Context) (ReadIdentity, error)
}
