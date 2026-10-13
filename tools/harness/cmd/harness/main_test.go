package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIArgumentAndComparatorFailureCodes(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}, {"current-build"}, {"architecture"}, {"compare-performance", "--output", "unused", "extra"}} {
		if code := run(args); code != 2 {
			t.Fatalf("%v returned %d", args, code)
		}
	}
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	report := filepath.Join(dir, "report.json")
	if e := os.WriteFile(bad, []byte("{incomplete"), 0600); e != nil {
		t.Fatal(e)
	}
	code := run([]string{"compare-performance", "--baseline", bad, "--candidate", bad, "--output", report})
	if code != 1 {
		t.Fatalf("bad input returned %d", code)
	}
	raw, e := os.ReadFile(report)
	if e != nil {
		t.Fatal(e)
	}
	var evidence map[string]any
	if e = json.Unmarshal(raw, &evidence); e != nil || evidence["status"] != "FAIL" {
		t.Fatal(e, evidence)
	}
}
func TestMakeToolPathsPreserveSpaces(t *testing.T) {
	root, e := filepath.Abs("../../../..")
	if e != nil {
		t.Fatal(e)
	}
	cmdDefault := exec.Command("make", "-n", "harness")
	cmdDefault.Dir = root
	defaultOut, err := cmdDefault.CombinedOutput()
	if err != nil {
		t.Fatal(err, string(defaultOut))
	}
	if !strings.Contains(string(defaultOut), `-o "`+filepath.Join(root, ".tools", "zl-mcp-harness")+`"`) {
		t.Fatalf("default root tool path wrong: %s", defaultOut)
	}
	tools := filepath.Join(t.TempDir(), "tools with spaces")
	cmd := exec.Command("make", "-n", "harness", "TOOLS_BIN="+tools)
	cmd.Dir = root
	out, e := cmd.CombinedOutput()
	if e != nil {
		t.Fatal(e, string(out))
	}
	want := `-o "` + filepath.Join(tools, "zl-mcp-harness") + `"`
	if !strings.Contains(string(out), want) {
		t.Fatalf("tool path split at spaces: %s", out)
	}
	// Relative tool overrides resolve from repository, even though go build runs in nested module.
	cmd = exec.Command("make", "-n", "harness", "TOOLS_BIN=relative tools")
	cmd.Dir = root
	out, e = cmd.CombinedOutput()
	if e != nil {
		t.Fatal(e, string(out))
	}
	want = `-o "` + filepath.Join(root, "relative tools", "zl-mcp-harness") + `"`
	if !strings.Contains(string(out), want) {
		t.Fatalf("relative tools path wrong: %s", out)
	}
}
