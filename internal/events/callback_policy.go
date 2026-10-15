package events

import (
	"errors"
	"net"
	"net/http"
	"net/url"

	"github.com/skosovsky/zl-mcp/internal/netpolicy"
)

// CallbackPolicy is immutable after construction. Its zero value permits only
// the original public HTTPS policy; local access requires operator configuration.
type CallbackPolicy struct {
	local map[string]bool
}

const maximumLocalCallbacks = 16

func NewCallbackPolicy(localURLs []string) (CallbackPolicy, error) {
	p := CallbackPolicy{local: make(map[string]bool, len(localURLs))}
	if len(localURLs) > maximumLocalCallbacks {
		return CallbackPolicy{local: nil}, errors.New("too many local callback URLs")
	}
	for _, raw := range localURLs {
		if _, err := netpolicy.LocalCallbackURL(raw); err != nil || len(raw) > 4096 || p.local[raw] {
			return CallbackPolicy{local: nil}, errors.New("invalid or duplicate local callback URL")
		}
		p.local[raw] = true
	}
	return p, nil
}

func (p CallbackPolicy) validate(raw string) (*url.URL, error) {
	if p.local[raw] {
		return netpolicy.LocalCallbackURL(raw)
	}
	return callbackURL(raw)
}

// Client enforces the URL allowlist again at the transport boundary. The local
// path uses no DNS or proxy and HTTPS still uses standard certificate validation.
func (p CallbackPolicy) Client() *http.Client {
	client := NewCallbackClient()
	//nolint:exhaustruct_v5 // Only explicit transport timeouts and literal dialing are required; proxy and custom TLS stay disabled.
	dialer := &net.Dialer{Timeout: callbackRequestTimeout}
	// Nil Proxy bypasses environment proxies; standard TLS verification remains enabled.
	local := &http.Transport{
		DialContext: dialer.DialContext, DisableKeepAlives: true,
		TLSHandshakeTimeout: callbackRequestTimeout, ResponseHeaderTimeout: callbackRequestTimeout,
	}
	client.Transport = callbackTransport{policy: p, public: client.Transport, local: local}
	return client
}

type callbackTransport struct {
	policy CallbackPolicy
	public http.RoundTripper
	local  http.RoundTripper
}

func (t callbackTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if _, err := t.policy.validate(r.URL.String()); err != nil {
		return nil, errCallback
	}
	if t.policy.local[r.URL.String()] {
		return t.local.RoundTrip(r)
	}
	return t.public.RoundTrip(r)
}
