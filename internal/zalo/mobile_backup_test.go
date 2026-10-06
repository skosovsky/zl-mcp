package zalo

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/errs"
	"github.com/amrakk/zcago/listener"
	"github.com/amrakk/zcago/model"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type mobileOfferFake struct {
	events             chan model.MobileSyncEvent
	failures           chan error
	public             string
	registered         bool
	requests, releases int
	ack                error
	dispatch           func(string)
	mapping            func([]string) ([]byte, error)
}

func (f *mobileOfferFake) SubscribeMobileSync(public, host string) (listener.MobileSyncSubscription, func(), error) {
	f.public = public
	f.registered = true
	return listener.MobileSyncSubscription{Events: f.events, Errors: f.failures}, func() { f.registered = false; f.releases++ }, nil
}
func (f *mobileOfferFake) RequestMobileBackup(ctx context.Context, public string) error {
	if !f.registered || public != f.public {
		return errors.New("receiver not registered")
	}
	f.requests++
	if f.dispatch != nil {
		f.dispatch(public)
	}
	return f.ack
}
func (f *mobileOfferFake) GetMobileIdentityMapping(ctx context.Context, direct, groups []string) ([]byte, error) {
	if f.mapping != nil {
		return f.mapping(direct)
	}
	mapped := direct[0]
	if mapped == "123" {
		mapped = "9007199254740993"
	}
	return json.Marshal(map[string][]string{"fids": {mapped}})
}
func newMobileOfferFake() *mobileOfferFake {
	return &mobileOfferFake{events: make(chan model.MobileSyncEvent, 8), failures: make(chan error, 1)}
}
func mobileOfferKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal("synthetic key generation failed")
	}
	return key
}
func syntheticOffer(t *testing.T, public string) model.MobileSyncEvent {
	t.Helper()
	der, err := base64.StdEncoding.DecodeString(public)
	if err != nil {
		t.Fatal("synthetic key encoding")
	}
	k, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		t.Fatal("synthetic public key")
	}
	cipher, err := rsa.EncryptPKCS1v15(rand.Reader, k.(*rsa.PublicKey), []byte(strings.Repeat("a", 32)))
	if err != nil {
		t.Fatal("synthetic encryption")
	}
	return model.MobileSyncEvent{Action: "syncmsg_info", PublicKey: public, UID: "123", URL: "https://synthetic.invalid/backup", EncryptedKey: base64.StdEncoding.EncodeToString(cipher), FileSize: 16, FromSequence: "9007199254740995", DatabaseInfo: `{"db_format":1}`}
}

func TestMobileBackupOfferPreregistrationAndUnknownAcknowledgement(t *testing.T) {
	for _, ack := range []error{nil, api.ErrMobileBackupUnknown} {
		// Arrange: replies arrive during dispatch, before acknowledgement returns.
		f := newMobileOfferFake()
		f.ack = ack
		key := mobileOfferKey(t)
		f.dispatch = func(public string) {
			for _, n := range []int{2, 1} {
				n := n
				f.events <- model.MobileSyncEvent{Action: "user_confirm", PublicKey: public, PCName: "Web", UserAction: &n}
			}
			f.events <- syntheticOffer(t, public)
		}
		var states []string
		// Act.
		offer, err := receiveMobileBackupOffer(context.Background(), "9007199254740993", f, f, &domain.MobileBackupObserver{BeforeDispatch: func(string) error { states = append(states, "dispatching"); return nil }, Progress: func(s string) error { states = append(states, s); return nil }}, func() (*rsa.PrivateKey, error) { return key, nil })
		// Assert: request never repeated, receiver released and key rendered as observed format-1 hex.
		if err != nil || f.requests != 1 || f.releases != 1 || f.registered || offer.KeyText != strings.Repeat("61", 32) || offer.FromSequence != "9007199254740995" || offer.PlainAccountID != "123" || offer.SessionAccountID != "9007199254740993" {
			t.Fatalf("offer operation: %v", err)
		}
		want := "dispatching,waiting_for_confirmation,mobile_restoring,waiting_for_backup,offer_ready"
		if ack != nil {
			want = strings.Replace(want, "waiting_for_confirmation", "request_result_unknown", 1)
		}
		if strings.Join(states, ",") != want {
			t.Fatal("state semantics lost")
		}
		serialized, _ := json.Marshal(offer)
		if string(serialized) != "{}" || strings.Contains(fmt.Sprintf("%#v", offer), "synthetic.invalid") {
			t.Fatal("offer data leaked")
		}
	}
}

func TestMobileBackupOfferTermination(t *testing.T) {
	key := mobileOfferKey(t)
	for _, tc := range []struct {
		name     string
		ack      error
		dispatch func(*mobileOfferFake, string)
		want     error
	}{
		{"authentication", errs.ErrAuthenticationRequired, nil, domain.ErrAuthenticationRequired},
		{"unknown timeout", api.ErrMobileBackupUnknown, nil, domain.ErrMobileBackupUnknown},
		{"rejected", nil, func(f *mobileOfferFake, p string) {
			n := 0
			f.events <- model.MobileSyncEvent{Action: "user_confirm", PublicKey: p, PCName: "Web", UserAction: &n}
		}, domain.ErrMobileBackupRejected},
		{"queue failure", nil, func(f *mobileOfferFake, p string) { f.failures <- errors.New("synthetic-private-detail") }, domain.ErrMobileBackupInvalid},
		{"unsupported format", nil, func(f *mobileOfferFake, p string) {
			e := syntheticOffer(t, p)
			e.DatabaseInfo = `{"db_format":0}`
			f.events <- e
		}, domain.ErrHistoryUnsupported},
		{"wrong account", nil, func(f *mobileOfferFake, p string) {
			e := syntheticOffer(t, p)
			e.UID = "9007199254740994"
			f.events <- e
		}, domain.ErrMobileBackupInvalid},
		{"bad encrypted key", nil, func(f *mobileOfferFake, p string) {
			e := syntheticOffer(t, p)
			e.EncryptedKey = "invalid"
			f.events <- e
		}, domain.ErrMobileBackupInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			f := newMobileOfferFake()
			f.ack = tc.ack
			if tc.dispatch != nil {
				f.dispatch = func(p string) { tc.dispatch(f, p) }
			}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			// Act.
			offer, err := receiveMobileBackupOffer(ctx, "9007199254740993", f, f, nil, func() (*rsa.PrivateKey, error) { return key, nil })
			// Assert.
			if !errors.Is(err, tc.want) || offer.KeyText != "" || f.requests != 1 || f.releases != 1 || f.registered {
				t.Fatalf("termination: %v", err)
			}
			if strings.Contains(err.Error(), "synthetic-private-detail") {
				t.Fatal("source failure leaked")
			}
		})
	}
}

func TestMobileBackupPrecancelDoesNotGenerateOrDispatch(t *testing.T) {
	// Arrange.
	f := newMobileOfferFake()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	generated := 0
	// Act.
	_, err := receiveMobileBackupOffer(ctx, "owner", f, f, nil, func() (*rsa.PrivateKey, error) { generated++; return nil, nil })
	// Assert.
	if !errors.Is(err, context.Canceled) || generated != 0 || f.requests != 0 || f.registered {
		t.Fatal("precancelled operation started")
	}
}

func TestMobileBackupOfferPersistenceFailureStopsWork(t *testing.T) {
	key := mobileOfferKey(t)
	sentinel := errors.New("synthetic journal failure")
	for _, stage := range []string{"dispatch", "waiting_for_confirmation", "offer_ready"} {
		t.Run(stage, func(t *testing.T) {
			// Arrange: the receiver can return a valid offer, but a journal commit fails.
			f := newMobileOfferFake()
			f.dispatch = func(public string) { f.events <- syntheticOffer(t, public) }
			observer := &domain.MobileBackupObserver{
				BeforeDispatch: func(string) error {
					if stage == "dispatch" {
						return sentinel
					}
					return nil
				},
				Progress: func(state string) error {
					if state == stage {
						return sentinel
					}
					return nil
				},
			}
			// Act.
			offer, err := receiveMobileBackupOffer(context.Background(), "9007199254740993", f, f, observer, func() (*rsa.PrivateKey, error) { return key, nil })
			// Assert: no successful offer escapes; a failed predispatch commit makes zero requests.
			want := 1
			if stage == "dispatch" {
				want = 0
			}
			if !errors.Is(err, sentinel) || offer != (domain.MobileBackupOffer{}) || f.requests != want || f.releases != 1 || f.registered {
				t.Fatal("journal failure did not stop work")
			}
		})
	}
}

func TestMobileBackupOfferCancellationAfterDispatchCommit(t *testing.T) {
	// Arrange: cancellation occurs while the predispatch journal is being committed.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := mobileOfferKey(t)
	f := newMobileOfferFake()
	observer := &domain.MobileBackupObserver{BeforeDispatch: func(string) error { cancel(); return nil }}
	// Act.
	offer, err := receiveMobileBackupOffer(ctx, "9007199254740993", f, f, observer, func() (*rsa.PrivateKey, error) { return key, nil })
	// Assert: durable dispatch intent does not force a network request after cancellation.
	if !errors.Is(err, context.Canceled) || offer != (domain.MobileBackupOffer{}) || f.requests != 0 || f.releases != 1 {
		t.Fatal("cancelled request dispatched")
	}
}

func TestMobileBackupTransferStatusesDoNotRejectOffer(t *testing.T) {
	for _, code := range []int{0, 7} {
		// Arrange: native status-first semantics preserve active/idle controls.
		f := newMobileOfferFake()
		key := mobileOfferKey(t)
		f.dispatch = func(public string) {
			confirm := 1
			f.events <- model.MobileSyncEvent{Action: "user_confirm", PublicKey: public, PCName: "Web", UserAction: &confirm}
			for _, status := range []int{1, 2} {
				status := status
				f.events <- model.MobileSyncEvent{Action: "transfer_error", PublicKey: public, PCName: "Web", Status: &status, ErrorCode: &code}
			}
			f.events <- syntheticOffer(t, public)
		}
		// Act.
		offer, err := receiveMobileBackupOffer(context.Background(), "9007199254740993", f, f, nil, func() (*rsa.PrivateKey, error) { return key, nil })
		// Assert: progress does not terminate or resend the original request.
		if err != nil || offer.FileSize != 16 || f.requests != 1 || f.releases != 1 {
			t.Fatal("phone status rejected valid offer", err)
		}
	}
}

func TestMobileBackupTransferFailureIsNotProgress(t *testing.T) {
	// Arrange: explicit nonzero failure without an active/idle status.
	f := newMobileOfferFake()
	key := mobileOfferKey(t)
	code := 7
	f.dispatch = func(public string) {
		f.events <- model.MobileSyncEvent{Action: "transfer_error", PublicKey: public, PCName: "Web", ErrorCode: &code}
	}
	// Act.
	offer, err := receiveMobileBackupOffer(context.Background(), "9007199254740993", f, f, nil, func() (*rsa.PrivateKey, error) { return key, nil })
	// Assert: failure never yields an archive or retries.
	if !errors.Is(err, domain.ErrMobileBackupRejected) || offer.FileSize != 0 || f.requests != 1 {
		t.Fatal("invalid transfer outcome", err)
	}
}

func TestMobileBackupZeroErrorControlContinuesToOffer(t *testing.T) {
	// Arrange: the observed phone response has error_code=0 and no status.
	f := newMobileOfferFake()
	key := mobileOfferKey(t)
	f.dispatch = func(public string) {
		confirm, zero := 1, 0
		f.events <- model.MobileSyncEvent{Action: "user_confirm", PublicKey: public, PCName: "Web", UserAction: &confirm}
		f.events <- model.MobileSyncEvent{Action: "transfer_error", PublicKey: public, PCName: "Web", ErrorCode: &zero}
		f.events <- syntheticOffer(t, public)
	}
	// Act.
	offer, err := receiveMobileBackupOffer(context.Background(), "9007199254740993", f, f, nil, func() (*rsa.PrivateKey, error) { return key, nil })
	// Assert: only the subsequent validated offer counts as success.
	if err != nil || offer.FileSize != 16 || f.requests != 1 || f.releases != 1 {
		t.Fatal("zero code terminated offer wait", err)
	}
}

func TestMobileBackupZeroErrorControlDoesNotInventSuccess(t *testing.T) {
	// Arrange: a zero-code control arrives without an archive.
	f := newMobileOfferFake()
	key := mobileOfferKey(t)
	f.dispatch = func(public string) {
		zero := 0
		f.events <- model.MobileSyncEvent{Action: "transfer_error", PublicKey: public, PCName: "Web", ErrorCode: &zero}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	// Act.
	offer, err := receiveMobileBackupOffer(ctx, "9007199254740993", f, f, nil, func() (*rsa.PrivateKey, error) { return key, nil })
	// Assert: the original bounded wait expires, with no retry or claimed archive.
	if !errors.Is(err, context.DeadlineExceeded) || offer.FileSize != 0 || f.requests != 1 || f.releases != 1 {
		t.Fatal("zero control lost bounded wait", err)
	}
}

func TestMobileBackupAccountMappingFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		err  error
	}{
		{"foreign owner", `{"fids":["456"]}`, nil},
		{"missing owner", `{"fids":[]}`, nil},
		{"unavailable", "", errors.New("private-mapping-detail")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: a correlated, valid offer belongs to a separate plain ID namespace.
			f := newMobileOfferFake()
			f.dispatch = func(p string) { f.events <- syntheticOffer(t, p) }
			f.mapping = func(ids []string) ([]byte, error) {
				if len(ids) != 1 || ids[0] != "123" {
					t.Fatal("wrong mapping input")
				}
				return []byte(tc.body), tc.err
			}
			key := mobileOfferKey(t)
			// Act.
			offer, err := receiveMobileBackupOffer(context.Background(), "9007199254740993", f, f, nil, func() (*rsa.PrivateKey, error) { return key, nil })
			// Assert: no usable offer or private upstream error escapes.
			if !errors.Is(err, domain.ErrMobileBackupInvalid) || offer != (domain.MobileBackupOffer{}) || f.requests != 1 || f.releases != 1 {
				t.Fatal("account binding failed open")
			}
			if strings.Contains(err.Error(), "private-mapping-detail") {
				t.Fatal("private mapping error leaked")
			}
		})
	}
}
