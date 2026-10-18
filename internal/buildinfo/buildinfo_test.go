package buildinfo

import (
	"runtime/debug"
	"strings"
	"testing"
)

func TestBuildIdentitySafeAndExplicitUnknown(t *testing.T) {
	cases := []struct {
		name              string
		info              *debug.BuildInfo
		available         bool
		version, revision string
		dirty             *bool
	}{
		{"unavailable", nil, false, "unknown", "unknown", nil},
		{"empty", &debug.BuildInfo{}, true, "unknown", "unknown", nil},
		{"development", &debug.BuildInfo{Main: debug.Module{Version: "(devel)"}}, true, "unknown", "unknown", nil},
		{
			"release",
			&debug.BuildInfo{
				Main: debug.Module{Version: "v1.2.3"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: strings.Repeat("a", 40)},
					{Key: "vcs.modified", Value: "false"},
				},
			},
			true,
			"v1.2.3",
			strings.Repeat("a", 40),
			new(false),
		},
		{
			"dirty",
			&debug.BuildInfo{Settings: []debug.BuildSetting{{Key: "vcs.modified", Value: "true"}}},
			true,
			"unknown",
			"unknown",
			new(true),
		},
		{
			"unsafe",
			&debug.BuildInfo{
				Main: debug.Module{Version: "/private/local/path"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "/private/local/path"},
					{Key: "vcs.modified", Value: "no"},
				},
			},
			true,
			"unknown",
			"unknown",
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange: embedded metadata supplied independently of the checkout.
			// Act
			got := fromBuildInfo(tc.info, tc.available)
			// Assert
			if got.Version != tc.version || got.Revision != tc.revision || (got.Dirty == nil) != (tc.dirty == nil) ||
				got.Dirty != nil && *got.Dirty != *tc.dirty {
				t.Fatalf("unexpected identity: %+v", got)
			}
			if strings.Contains(got.DisplayVersion(), "/") {
				t.Fatal("local path leaked")
			}
		})
	}
}
