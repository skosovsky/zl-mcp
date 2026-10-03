package config

import (
	"strings"
	"testing"
)

func TestSendPermissionsValidateRecipientBoundariesWithoutGrantingCollection(t *testing.T) {
	for _, tc := range []struct {
		name    string
		ids     []string
		invalid bool
	}{
		{"omitted", nil, false}, {"empty", []string{}, false},
		{"distinct", []string{"peer", "another"}, false},
		{"duplicate", []string{"peer", "peer"}, true},
		{"blank ID", []string{""}, true},
		{"Unicode boundary", []string{strings.Repeat("界", 256)}, false},
		{"overlong", []string{strings.Repeat("界", 257)}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			var c Config
			c.Permissions.AllowSend = true
			c.Permissions.SendRecipientIDs = tc.ids
			// Act.
			err := c.validatePermissions()
			// Assert.
			if (err != nil) != tc.invalid {
				t.Fatalf("invalid=%v error=%v", tc.invalid, err)
			}
			if c.Policy().All || len(c.Policy().Selected) != 0 {
				t.Fatal("send permission expanded collection")
			}
		})
	}
}
