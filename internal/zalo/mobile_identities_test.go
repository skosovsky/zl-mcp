package zalo

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/amrakk/zcago"
	"github.com/amrakk/zcago/errs"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type identityMappingFake struct {
	calls  int
	body   []byte
	err    error
	cancel context.CancelFunc
}

func (f *identityMappingFake) GetMobileIdentityMapping(ctx context.Context, direct, groups []string) ([]byte, error) {
	f.calls++
	if f.cancel != nil {
		f.cancel()
	}
	return f.body, f.err
}

func TestMobileIdentityAdapterValidatesWholeReplyAndClearsPlaintext(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		err        error
		want       error
	}{
		{"exact", `{"fids":["9007199254740995"],"gids":["g9007199254740997"]}`, nil, nil},
		{"count_mismatch", `{"fids":[],"gids":["5"]}`, nil, domain.ErrMobileBackupInvalid},
		{"partial", `{"fids":["2"]}`, nil, domain.ErrMobileBackupInvalid},
		{"upstream_error", `private-marker`, errors.New("private-marker"), domain.ErrMobileBackupInvalid},
		{"auth", `private-marker`, errs.ErrAuthenticationRequired, domain.ErrAuthenticationRequired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			f := &identityMappingFake{body: []byte(tc.body), err: tc.err}
			request := domain.MobileIdentityRequest{Direct: []string{"9007199254740993"}, Groups: []string{"18446744073709551615"}}
			// Act.
			pairs, err := mapMobileIdentities(context.Background(), request, f)
			// Assert: no partial mappings and no raw SDK error exposure; owned reply buffer cleared.
			if f.calls != 1 || !bytes.Equal(f.body, make([]byte, len(f.body))) {
				t.Fatal("request repeated or plaintext retained")
			}
			if tc.want == nil {
				if err != nil || len(pairs) != 2 || pairs[0].Session != "9007199254740995" || !pairs[1].Group {
					t.Fatal("valid mapping rejected")
				}
			} else if !errors.Is(err, tc.want) || pairs != nil {
				t.Fatal("invalid mapping exposed")
			}
		})
	}
}

func TestMobileIdentityAdapterCancellationAndInputBounds(t *testing.T) {
	// Arrange.
	f := &identityMappingFake{body: []byte(`{"fids":["2"]}`)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act / Assert: reject before calling the SDK.
	if _, err := mapMobileIdentities(ctx, domain.MobileIdentityRequest{Direct: []string{"1"}}, f); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel bypass")
	}
	if _, err := mapMobileIdentities(context.Background(), domain.MobileIdentityRequest{Direct: []string{"01"}}, f); !errors.Is(err, domain.ErrMobileBackupInvalid) {
		t.Fatal("invalid identity input")
	}
	if f.calls != 0 {
		t.Fatal("invalid request dispatched")
	}
	// Cancellation during a successful SDK response still discards it.
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	f.cancel = cancel
	if pairs, err := mapMobileIdentities(ctx, domain.MobileIdentityRequest{Direct: []string{"1"}}, f); !errors.Is(err, context.Canceled) || pairs != nil || f.calls != 1 {
		t.Fatal("cancelled mapping exposed")
	}
}

type changingIdentityAPI struct {
	zcago.API
	owner  string
	change bool
	calls  int
}

func (f *changingIdentityAPI) GetOwnID() string { return f.owner }
func (f *changingIdentityAPI) GetMobileIdentityMapping(context.Context, []string, []string) ([]byte, error) {
	f.calls++
	if f.change {
		f.owner = "other-account"
	}
	return []byte(`{"fids":["2"]}`), nil
}
func TestMobileIdentityClientRejectsAccountChange(t *testing.T) {
	// Arrange: a successful mapping belongs to the previous account.
	backend := &changingIdentityAPI{owner: "synthetic-account", change: true}
	client := &Client{api: backend}
	// Act.
	result, err := client.MapMobileBackupIdentities(context.Background(), domain.MobileIdentityRequest{Direct: []string{"1"}})
	// Assert.
	if !errors.Is(err, domain.ErrMobileBackupInvalid) || result != nil || backend.calls != 1 {
		t.Fatal("changed-account mapping exposed")
	}
	backend.owner = ""
	backend.calls = 0
	if _, err = client.MapMobileBackupIdentities(context.Background(), domain.MobileIdentityRequest{Direct: []string{"1"}}); !errors.Is(err, domain.ErrAuthenticationRequired) || backend.calls != 0 {
		t.Fatal("unowned session dispatched")
	}
}
