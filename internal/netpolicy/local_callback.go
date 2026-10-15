package netpolicy

import (
	"errors"
	"net"
	"net/url"
	"path"
	"strconv"
)

// LocalCallbackURL accepts only canonical, literal same-host callback targets.
// It does not authorize a target; callers must also enforce the exact allowlist.
func LocalCallbackURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, errors.New("invalid local callback URL")
	}
	port, portErr := strconv.Atoi(u.Port())
	if (u.Scheme != "http" && u.Scheme != "https") ||
		(u.Hostname() != "127.0.0.1" && u.Hostname() != "::1") ||
		portErr != nil || port < 1 || port > 65535 || strconv.Itoa(port) != u.Port() ||
		u.Host != net.JoinHostPort(u.Hostname(), u.Port()) ||
		u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		u.RawFragment != "" || u.Opaque != "" || u.RawPath != "" || u.EscapedPath() != u.Path ||
		u.Path == "" || path.Clean(u.Path) != u.Path || u.String() != raw {
		return nil, errors.New("local callback requires a canonical literal loopback URL and explicit port")
	}
	return u, nil
}
