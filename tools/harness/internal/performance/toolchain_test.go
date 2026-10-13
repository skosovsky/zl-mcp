package performance

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExactToolchainToken(t *testing.T) {
	// Arrange: adjacent patch versions and embedded tokens must not match the pin.
	for _, output := range []string{
		"go version go1.27.20 darwin/arm64", "go version devel-go1.27.2 darwin/arm64",
		"prefix go version go1.27.2", "go1.27.2",
	} {
		// Act / Assert.
		if toolchainVersion(output) == candidateGoVersion {
			t.Fatalf("accepted unpinned toolchain %q", output)
		}
	}
}

func TestComparisonRejectsInconsistentToolchain(t *testing.T) {
	// Arrange: a complete current candidate with accepted historical baseline.
	cases := map[string]func(Object){
		"adjacent patch": func(build Object) { build["toolchain"] = "go version go1.27.20 darwin/arm64" },
		"wrong build token": func(build Object) {
			object(array(build["builds"])[0])["build_info"] = "/binary: go1.27.20\n"
		},
		"missing policy": func(build Object) { delete(build, "toolchain_pin") },
		"changed policy": func(build Object) { build["toolchain_pin"] = historicalGoVersion },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			baseline, candidate, path := pair(t)
			build, _, err := readObject(path)
			if err != nil {
				t.Fatal(err)
			}
			change(build)
			if err = Write(path, build); err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			candidate["build_manifest_sha256"] = digest(raw)
			// Act / Assert: even a correctly hashed manifest cannot bless version mismatch.
			requireStatus(t, Compare(baseline, candidate, "", path), codeFail)
		})
	}
}

func TestSharedWorkerBuildFailureCleanup(t *testing.T) {
	// Arrange: run the existing worker test in a subprocess whose go compiler fails.
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	compiler := filepath.Join(dir, "go")
	// executable synthetic compiler fixture, isolated in t.TempDir; no secrets.
	if err = os.WriteFile(compiler, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	// execute only the current test binary, with a fixed test filter and isolated fake compiler.
	command := exec.Command(binary, "-test.run=^TestProcessWatchdogAndBoundedOutput$")
	command.Env = append(os.Environ(), "PATH="+dir, "TMPDIR="+dir)
	// Act: TestMain must own cleanup even when the shared worker never builds.
	output, runErr := command.CombinedOutput()
	if runErr == nil {
		t.Fatalf("expected worker build failure: %s", output)
	}
	// Assert: no shared worker directory survives the failed suite.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "zl-go-perf-test-worker-") {
			t.Fatalf("worker directory survived build failure: %s", entry.Name())
		}
	}
}
