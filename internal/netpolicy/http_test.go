package netpolicy

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

func TestValidatedDialRejectsEntireMixedDNSAnswer(t *testing.T) {
	for _, blocked := range []string{"127.0.0.1", "10.0.0.1", "::1", "::ffff:192.168.0.1", "100.64.0.1", "169.254.169.254", "192.0.2.1", "2001:db8::1"} {
		t.Run(blocked, func(t *testing.T) {
			// Arrange: a public first answer must not conceal a later reserved address.
			calls := 0
			dial := ValidatedDial(func(context.Context, string) ([]netip.Addr, error) {
				return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr(blocked)}, nil
			}, func(context.Context, string, string) (net.Conn, error) {
				calls++
				return nil, errors.New("unexpected dial")
			})
			// Act.
			_, err := dial(context.Background(), "tcp", "archive.example.com:443")
			// Assert.
			if err == nil || calls != 0 {
				t.Fatal("mixed DNS answer reached network")
			}
		})
	}
}
