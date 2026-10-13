package architecture

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func repo(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	return p
}
func binary(t *testing.T) string {
	t.Helper()
	p := os.Getenv("ARCH_GO_BINARY")
	if p == "" {
		p = filepath.Join(repo(t), ".tools/arch-go")
	}
	if _, err := os.Stat(p); err != nil {
		t.Skip("pinned arch-go binary unavailable; make architecture runs mandatory controls")
	}
	return p
}
func configs(t *testing.T) ([]byte, []byte) {
	t.Helper()
	old, err := os.ReadFile("testdata/original-config.json")
	if err != nil {
		t.Fatal(err)
	}
	current, err := os.ReadFile(filepath.Join(repo(t), "arch-go.yml"))
	if err != nil {
		t.Fatal(err)
	}
	return old, current
}
func TestNegativeControlUsesExactWeakenedConfig(t *testing.T) {
	// Arrange: weaken only the real MCP allowlist; no independent graph check can mask it.
	_, current := configs(t)
	var config map[string]any
	if err := yaml.Unmarshal(current, &config); err != nil {
		t.Fatal(err)
	}
	for _, item := range config["dependenciesRules"].([]any) {
		rule := item.(map[string]any)
		if rule["package"] == "^github[.]com/skosovsky/zl-mcp/internal/mcpserver.**" {
			allow := rule["shouldOnlyDependsOn"].(map[string]any)
			allow["internal"] = append(allow["internal"].([]any), "^github[.]com/skosovsky/zl-mcp/internal/storage.**")
		}
	}
	weakened, err := yaml.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	// Act: regular controls receive the same weakened bytes used by the architecture run.
	_, err = Controls(context.Background(), binary(t), weakened)
	// Assert: the control itself rejects the weakened policy.
	if err == nil || !strings.Contains(err.Error(), "negative control accepted: mcp-storage") {
		t.Fatalf("expected mcp-storage acceptance to fail controls; got %v", err)
	}
}
func decisions(r Result) [4]bool {
	return [4]bool{r.Exit == 0, r.Report.Summary.Pass, r.Report.Compliance.Pass, r.Report.Coverage.Pass}
}
func TestConfigDecisionEquivalence(t *testing.T) {
	// Arrange: old config is immutable test-only reference, never a production fallback.
	old, current := configs(t)
	b := binary(t)
	ctx := context.Background()
	extra := []controlCase{
		{"domain-root-positive", "internal/domain", "context", "positive", false},
		{"domain-neighbor", "internal/domain_legacy", "context", "coverage", true},
		{"domain-deep-positive", "internal/domain/a/b", "context", "positive", false},
		{"app-own-positive", "internal/app/search/sub", module + "/internal/app/search", "positive", false},
		{"app-other-capability", "internal/app/search", module + "/internal/app/history", "dependency", true},
		{"app-own-shadow", "internal/app/search", module + "/internal/app/search_legacy", "dependency", true},
		{"external-exact", "internal/domain", "github.com/google/uuid/sub", "dependency", true},
		{"exact-net", "internal/config", "net/http", "dependency", true},
		{"shadow-prefix", "internal/foo/internal/domain", "context", "coverage", true},
	}
	for _, c := range append(append([]controlCase{}, cases[:10]...), extra...) {
		t.Run(c.name, func(t *testing.T) {
			// Act: both configs are evaluated by the actual pinned matcher.
			before, err := synthetic(ctx, b, old, controlFiles(c))
			if err != nil {
				t.Fatal(err)
			}
			after, err := synthetic(ctx, b, current, controlFiles(c))
			if err != nil {
				t.Fatal(err)
			}
			// Assert: rule aggregation/timing may differ; decisions and expected reasons must not.
			if decisions(before) != decisions(after) {
				t.Fatalf("decision changed: before=%v after=%v", decisions(before), decisions(after))
			}
			if err = expected(c, after); err != nil {
				t.Fatal(err)
			}
		})
	}
	// Act/Assert: cover every current production import including files invisible on this host.
	files, err := Scan(repo(t))
	if err != nil {
		t.Fatal(err)
	}
	// Current source evolves independently of the immutable historical fixtures.
	r, err := synthetic(ctx, b, current, files)
	if err != nil {
		t.Fatal(err)
	}
	if !passing(r) {
		t.Fatalf("actual source graph failed: %s", r.Stdout)
	}
}
func TestScanHiddenSourcesAndRejectMalformedSource(t *testing.T) {
	// Arrange: hidden sources must be scanned while fixtures, tests and nested modules are excluded.
	root := t.TempDir()
	for name, body := range map[string]string{
		"internal/domain/a_windows.go": "//go:build hidden\n\npackage domain\nimport _ \"context\"\n",
		"internal/domain/a_test.go":    "invalid test source",
		"testdata/a.go":                "invalid fixture source",
		"nested/go.mod":                "module nested", "nested/a.go": "invalid nested source",
	} {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// Act.
	files, err := Scan(root)
	// Assert.
	want := []File{{File: "internal/domain/a_windows.go", Imports: []string{"context"}}}
	if err != nil || !reflect.DeepEqual(files, want) {
		t.Fatalf("files=%v err=%v", files, err)
	}
	if err = os.WriteFile(filepath.Join(root, "bad.go"), []byte("package bad\nfunc Broken( {"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = Scan(root); err == nil {
		t.Fatal("malformed production file accepted")
	}
}
func TestHiddenSourceControls(t *testing.T) {
	// Arrange.
	_, current := configs(t)
	// Act.
	results, err := SourceControls(context.Background(), binary(t), current)
	// Assert.
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 6 {
		t.Fatalf("source controls: %d", len(results))
	}
}
func TestMissingAndStaleReportFailClosed(t *testing.T) {
	// Arrange: executable exits successfully without producing a report; stale PASS exists.
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".arch-go"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".arch-go/report.json"), []byte(`{"summary":{"pass":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	// Act.
	_, err := Run(context.Background(), "/usr/bin/true", root)
	// Assert.
	if err == nil || !strings.Contains(err.Error(), "missing report") {
		t.Fatalf("stale report accepted: %v", err)
	}
}
func TestControlRejectsWrongReason(t *testing.T) {
	// Arrange: valid report with coverage failure but no dependency failure.
	var r Result
	r.Exit = 1
	r.Report.Compliance.Pass = true
	r.Report.Compliance.Details = json.RawMessage(`{}`)
	// Act.
	err := expected(cases[2], r)
	// Assert.
	if err == nil || !strings.Contains(err.Error(), "wrong dependency reason") {
		t.Fatalf("wrong rejection counted: %v", err)
	}
}
func TestBoundedDiagnostics(t *testing.T) {
	// Arrange.
	var b boundedBuffer
	data := make([]byte, (1<<20)+42)
	// Act.
	n, err := b.Write(data)
	// Assert.
	if n != len(data) || err != nil || b.Len() != 1<<20 || !b.overflow {
		t.Fatalf("unbounded diagnostics n=%d len=%d overflow=%v err=%v", n, b.Len(), b.overflow, err)
	}
}

func TestCommandDeadline(t *testing.T) {
	// Arrange: a Go helper subprocess stays alive until the owned group is cancelled.
	if os.Getenv("ZL_ARCH_HANG_HELPER") == "1" {
		for {
			time.Sleep(time.Second)
		}
	}
	t.Setenv("ZL_ARCH_HANG_HELPER", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	started := time.Now()
	// Act.
	_, _, _, err := command(ctx, t.TempDir(), os.Args[0], "-test.run=^TestCommandDeadline$")
	// Assert: cancellation waits for cleanup and reports failure within a bounded time.
	if err == nil || time.Since(started) > 3*time.Second {
		t.Fatalf("deadline handling err=%v duration=%v", err, time.Since(started))
	}
}

func TestHTTPPortPolicyTightening(t *testing.T) {
	// Arrange: historical config permits domain HTTP and lacks a separate consumer-owned port rule.
	prior, current := configs(t)
	b := binary(t)
	ctx := context.Background()
	c := controlCase{"domain-http", "internal/domain", "net/http", "positive", false}
	// Act.
	before, err := synthetic(ctx, b, prior, controlFiles(c))
	if err != nil {
		t.Fatal(err)
	}
	after, err := synthetic(ctx, b, current, controlFiles(c))
	if err != nil {
		t.Fatal(err)
	}
	// Assert: only this explicit historical dependency tightening is expected.
	if !passing(before) {
		t.Fatal("prior domain HTTP allowance unexpectedly absent")
	}
	c.fail = true
	c.kind = "dependency"
	if err = expected(c, after); err != nil {
		t.Fatal(err)
	}
	for _, c := range cases[11:] {
		r, err := synthetic(ctx, b, current, controlFiles(c))
		if err != nil {
			t.Fatal(err)
		}
		if err = expected(c, r); err != nil {
			t.Fatal(err)
		}
	}
}
