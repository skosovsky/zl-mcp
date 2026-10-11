package shared

import "context"

// AdmissionPort borrows the owner's existing shared read bucket. Transport
// calls it once per tool and independently once per resource request.
type AdmissionPort interface {
	AllowRead(context.Context) (bool, error)
}
