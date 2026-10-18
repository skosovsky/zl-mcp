// Package buildinfo exposes only safe application identity, never local build
// paths, environment variables or protocol versions.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// MetadataKey namespaces application identity outside strict tool output schemas.
const MetadataKey = "zl-mcp/build"

const unknown = "unknown"

// Identity describes the executable which produced an MCP response. A nil Dirty
// value explicitly means unknown, rather than falsely claiming a clean build.
type Identity struct {
	Version  string `json:"version"`
	Revision string `json:"revision"`
	Dirty    *bool  `json:"dirty"`
}

// Current reads Go's embedded metadata, with explicit unknown values for test,
// stripped or non-module builds. A devel module version is not a release tag.
func Current() Identity {
	info, ok := debug.ReadBuildInfo()
	return fromBuildInfo(info, ok)
}

func fromBuildInfo(info *debug.BuildInfo, ok bool) Identity {
	identity := Identity{Version: unknown, Revision: unknown, Dirty: nil}
	if !ok || info == nil {
		return identity
	}
	if safeVersion(info.Main.Version) {
		identity.Version = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if safeRevision(setting.Value) {
				identity.Revision = strings.ToLower(setting.Value)
			}
		case "vcs.modified":
			if setting.Value == "true" || setting.Value == "false" {
				dirty := setting.Value == "true"
				identity.Dirty = &dirty
			}
		}
	}
	return identity
}

func safeVersion(value string) bool {
	if value == "" || value == "(devel)" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(".-+_", r)
		if !valid {
			return false
		}
	}
	return true
}

func safeRevision(value string) bool {
	if len(value) != 40 && len(value) != 64 {
		return false
	}
	for _, r := range value {
		valid := r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F'
		if !valid {
			return false
		}
	}
	return true
}

// DisplayVersion provides an MCP implementation version without collapsing
// unknown dirty state to false. The structured identity remains authoritative.
func (identity Identity) DisplayVersion() string {
	dirty := unknown
	if identity.Dirty != nil {
		if *identity.Dirty {
			dirty = "dirty"
		} else {
			dirty = "clean"
		}
	}
	return identity.Version + "+revision." + identity.Revision + "." + dirty
}
