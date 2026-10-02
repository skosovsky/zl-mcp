package events

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var errCallback = errors.New("callback request failed")

func callbackURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return nil, errors.New("callback requires an HTTPS URL without credentials or fragment")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return nil, errors.New("invalid callback port")
		}
	}
	if ip, err := netip.ParseAddr(u.Hostname()); err == nil && !publicIP(ip) {
		return nil, errors.New("callback address is not public")
	}
	return u, nil
}

var blockedCallbackPrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"), netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"), netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"), netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"), netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/23"), netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func publicIP(ip netip.Addr) bool {
	if ip.Zone() != "" {
		return false
	}
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range blockedCallbackPrefixes {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

type callbackLookup func(context.Context, string) ([]netip.Addr, error)
type callbackDial func(context.Context, string, string) (net.Conn, error)

func validatedDial(lookup callbackLookup, dial callbackDial) callbackDial {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, errCallback
		}
		ips, err := lookup(ctx, host)
		if err != nil || len(ips) == 0 {
			return nil, errCallback
		}
		for _, ip := range ips {
			if !publicIP(ip) {
				return nil, errors.New("callback DNS resolves to a non-public address")
			}
		}
		for _, ip := range ips {
			conn, err := dial(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
			if err == nil {
				return conn, nil
			}
		}
		return nil, errCallback
	}
}

func newCallbackClient() *http.Client {
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	transport := &http.Transport{
		DialContext: validatedDial(func(ctx context.Context, host string) ([]netip.Addr, error) {
			return net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		}, dialer.DialContext),
		TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 5 * time.Second,
		DisableKeepAlives: true,
	}
	return &http.Client{Transport: transport, Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return errCallback }}
}

func callbackKey(secret string) ([]byte, error) {
	if !strings.HasPrefix(secret, "whsec_") {
		return nil, errors.New("invalid callback signing secret")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(secret, "whsec_"))
	if err != nil || len(key) < 24 || len(key) > 64 {
		return nil, errors.New("invalid callback signing secret")
	}
	return key, nil
}

func postCallback(ctx context.Context, client *http.Client, rawURL, secret, subscription, id string, body []byte) (*http.Response, error) {
	if len(body) > 262144 {
		return nil, errors.New("callback payload exceeds 256 KiB")
	}
	u, err := callbackURL(rawURL)
	if err != nil {
		return nil, err
	}
	key, err := callbackKey(secret)
	if err != nil {
		return nil, err
	}
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	mac := hmac.New(sha256.New, key)
	fmt.Fprintf(mac, "%s.%s.", id, timestamp)
	mac.Write(body)
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
	if err != nil {
		return nil, errCallback
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("webhook-id", id)
	request.Header.Set("webhook-timestamp", timestamp)
	request.Header.Set("webhook-signature", "v1,"+base64.StdEncoding.EncodeToString(mac.Sum(nil)))
	request.Header.Set("X-MCP-Subscription-Id", subscription)
	response, err := client.Do(request)
	if err != nil {
		return nil, errCallback
	}
	return response, nil
}

func verifyCallback(ctx context.Context, client *http.Client, rawURL, secret, subscription string) error {
	challengeBytes := make([]byte, 32)
	if _, err := rand.Read(challengeBytes); err != nil {
		return errors.New("callback challenge generation failed")
	}
	challenge := base64.RawURLEncoding.EncodeToString(challengeBytes)
	body, _ := json.Marshal(map[string]string{"type": "verification", "challenge": challenge})
	response, err := postCallback(ctx, client, rawURL, secret, subscription, "verification_"+challenge, body)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return errors.New("callback verification rejected")
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	if err != nil || len(data) > 65536 {
		return errors.New("callback verification response invalid")
	}
	var result struct {
		Challenge string `json:"challenge"`
	}
	if json.Unmarshal(data, &result) != nil || subtle.ConstantTimeCompare([]byte(result.Challenge), []byte(challenge)) != 1 {
		return errors.New("callback verification challenge mismatch")
	}
	return nil
}

// NewCallbackClient applies the production HTTPS and DNS policy.
func NewCallbackClient() *http.Client { return newCallbackClient() }

func ValidateCallbackURL(raw string) (*url.URL, error) { return callbackURL(raw) }
func DecodeSigningKey(secret string) ([]byte, error)   { return callbackKey(secret) }
func VerifyCallback(ctx context.Context, client *http.Client, rawURL, secret, subscription string) error {
	return verifyCallback(ctx, client, rawURL, secret, subscription)
}
