// Package archiveport owns the current-session transport consumed by mobile backup downloads.
package archiveport

import (
	"context"
	"net/http"
)

// Transport consumes an archive within the current session scope. The caller
// supplies its validated transport and bounded body consumer; no credential
// getter or session values are returned.
type Transport interface {
	ConsumeMobileArchive(
		context.Context,
		*http.Request,
		*http.Client,
		func(context.Context, *http.Response) ([]byte, error),
	) ([]byte, error)
}
