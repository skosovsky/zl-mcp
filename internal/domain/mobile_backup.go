package domain

import (
	"context"
	"errors"
	"net/http"
)

var ErrMobileBackupRejected = errors.New("mobile backup rejected")
var ErrMobileBackupUnknown = errors.New("mobile backup result unknown")
var ErrMobileBackupInvalid = errors.New("invalid mobile backup offer")

// MobileBackupOffer is sensitive transport evidence, never a public tool result.
type MobileBackupOffer struct {
	URL          string `json:"-"`
	KeyText      string `json:"-"`
	FileSize     uint64 `json:"-"`
	FromSequence string `json:"-"`
}

func (MobileBackupOffer) String() string   { return "mobile backup offer [redacted]" }
func (MobileBackupOffer) GoString() string { return "mobile backup offer [redacted]" }

// MobileBackupObserver commits dispatch and progress before continuing. Callbacks
// are trusted local functions; any failure stops the operation without exposing an offer.
type MobileBackupObserver struct {
	BeforeDispatch func(publicKey string) error
	Progress       func(state string) error
}

type MobileBackupSource interface {
	ReceiveMobileBackupOffer(context.Context, *MobileBackupObserver) (MobileBackupOffer, error)
}

// MobileIdentityRequest and pairs are private archive-to-session evidence.
type MobileIdentityRequest struct {
	Direct []string `json:"-"`
	Groups []string `json:"-"`
}

func (MobileIdentityRequest) String() string   { return "mobile backup identity request [redacted]" }
func (MobileIdentityRequest) GoString() string { return "mobile backup identity request [redacted]" }

type MobileIdentityPair struct {
	Plain   string `json:"-"`
	Session string `json:"-"`
	Group   bool   `json:"-"`
}

func (MobileIdentityPair) String() string   { return "mobile backup identity [redacted]" }
func (MobileIdentityPair) GoString() string { return "mobile backup identity [redacted]" }

type MobileIdentitySource interface {
	MapMobileBackupIdentities(context.Context, MobileIdentityRequest) ([]MobileIdentityPair, error)
}

// MobileBackupScope borrows cancellation from the existing collector session.
// It does not create or authenticate a session.
type MobileBackupScope interface {
	MobileBackupContext(context.Context) (context.Context, func(), error)
}

// MobileArchiveTransport consumes an archive within the current session scope.
// The caller supplies its validated transport and bounded body consumer; no
// credential getter or session values are returned.
type MobileArchiveTransport interface {
	ConsumeMobileArchive(context.Context, *http.Request, *http.Client, func(context.Context, *http.Response) ([]byte, error)) ([]byte, error)
}
