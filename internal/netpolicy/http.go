// Package netpolicy applies public-address-only HTTP transport without proxies or redirects.
package netpolicy

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"time"
)

var ErrNetwork = errors.New("public network request failed")

//nolint:gochecknoglobals // Immutable public-address deny ranges are parsed once and only read by request guards.
var blockedPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func PublicIP(ip netip.Addr) bool {
	if ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range blockedPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

type Lookup func(context.Context, string) ([]netip.Addr, error)
type Dial func(context.Context, string, string) (net.Conn, error)

func ValidatedDial(lookup Lookup, dial Dial) Dial {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, ErrNetwork
		}
		ips, err := lookup(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, ErrNetwork
		}
		for _, ip := range ips {
			if !PublicIP(ip) {
				return nil, errors.New("callback DNS resolves to a non-public address")
			}
		}
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, ErrNetwork
	}
}

func NewClient(timeout time.Duration) *http.Client {
	//nolint:exhaustruct_v5 // Keep stdlib defaults for omitted dialing options; the explicit timeout controls this request.
	dialer := &net.Dialer{Timeout: dialTimeout}
	transport := &http.Transport{
		DialContext: ValidatedDial(func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}, dialer.DialContext),
		TLSHandshakeTimeout: tlsHandshakeTimeout, ResponseHeaderTimeout: responseHeaderTimeout,
		DisableKeepAlives: true,
	}
	return &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return ErrNetwork },
	}
}
