package performance

import "strings"

const (
	candidateGoVersion    = "go1.27.2"
	historicalGoVersion   = "go1.27.1"
	toolchainHeaderTokens = 3
)

// toolchainVersion parses the exact token emitted by go version.
func toolchainVersion(output string) string {
	fields := strings.Fields(output)
	if len(fields) < toolchainHeaderTokens || fields[0] != "go" || fields[1] != "version" {
		return ""
	}
	return fields[2]
}

func binaryGoVersion(output string) string {
	line, _, _ := strings.Cut(output, "\n")
	_, version, found := strings.Cut(line, ": ")
	if found {
		return version
	}
	return line // Historical synthetic fixtures contain only the version token.
}

func validGoProvenance(side, measured string, build, binary Object) bool {
	want := historicalGoVersion
	if side == wireCandidate && build["toolchain_pin"] != nil {
		if build["toolchain_pin"] != candidateGoVersion {
			return false
		}
		want = candidateGoVersion
	}
	// An unversioned candidate is historical evidence, never a current Build output.
	return measured == want && toolchainVersion(str(build["toolchain"])) == want &&
		binaryGoVersion(str(binary["build_info"])) == want
}
