package netpolicy

import "testing"

func TestLocalCallbackCanonicalBoundary(t *testing.T) {
	for _, tc := range []struct {
		url   string
		valid bool
	}{
		{"http://127.0.0.1:19001/events", true}, {"https://[::1]:19001/events", true},
		{"http://localhost:19001/events", false}, {"http://127.0.0.2:19001/events", false},
		{"http://[::ffff:127.0.0.1]:19001/events", false}, {"http://192.168.1.1:19001/events", false},
		{"http://127.0.0.1/events", false}, {"http://127.0.0.1:0/events", false},
		{"http://127.0.0.1:65536/events", false}, {"http://127.0.0.1:019001/events", false},
		{"http://user@127.0.0.1:19001/events", false}, {"http://127.0.0.1:19001/events?q=1", false},
		{"http://127.0.0.1:19001/events?", false}, {"http://127.0.0.1:19001/events#x", false},
		{"http://127.0.0.1:19001/a/../events", false}, {"http://127.0.0.1:19001/%65vents", false},
		{"http://127.0.0.1:19001//events", false}, {"http://127.0.0.1:19001", false},
		{"http://::1:19001/events", false}, {"http://127.0.0.1:19001/a%20b", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			// Arrange: a literal candidate URL with no network access.
			// Act.
			_, err := LocalCallbackURL(tc.url)
			// Assert.
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, err=%v", tc.valid, err)
			}
		})
	}
}
