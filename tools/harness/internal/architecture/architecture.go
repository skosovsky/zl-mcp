// Package architecture runs one pinned arch-go matcher against host and all-build imports.
package architecture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gopkg.in/yaml.v3"
)

const module = "github.com/skosovsky/zl-mcp"
const Pin = "github.com/arch-go/arch-go/v2\tv2.1.2"

type File struct {
	File    string   `json:"file"`
	Imports []string `json:"imports"`
}
type Report struct {
	Summary struct {
		Pass bool `json:"pass"`
	} `json:"summary"`
	Compliance struct {
		Pass    bool            `json:"pass"`
		Rate    float64         `json:"rate"`
		Details json.RawMessage `json:"details"`
	} `json:"compliance"`
	Coverage struct {
		Pass bool    `json:"pass"`
		Rate float64 `json:"rate"`
	} `json:"coverage"`
}
type Result struct {
	Exit   int    `json:"exit_status"`
	Stdout string `json:"stdout"`
	Stderr string `json:"stderr"`
	Report Report `json:"report"`
}
type Control struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Result Result `json:"result"`
}
type Evidence struct {
	Status         string    `json:"status"`
	BuildInfo      string    `json:"arch_go_build_info"`
	Active         Result    `json:"active"`
	Source         Result    `json:"all_build_source"`
	Files          []File    `json:"file_imports"`
	Dependencies   []string  `json:"production_dependencies"`
	Controls       []Control `json:"controls"`
	SourceControls []Control `json:"source_controls"`
	Error          string    `json:"error,omitempty"`
}

// boundedBuffer drains output while preserving only a bounded diagnostic prefix.
const architectureDiagnosticLimit = 1 << 20
const architectureCommandTimeout = 60 * time.Second

type boundedBuffer struct {
	bytes.Buffer

	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	room := architectureDiagnosticLimit - b.Len()
	if room < n {
		b.overflow = true
	}
	if room > 0 {
		if room < n {
			p = p[:room]
		}
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
func command(ctx context.Context, root, binary string, args ...string) (int, string, string, error) {
	ctx, cancel := context.WithTimeout(ctx, architectureCommandTimeout)
	defer cancel()
	//nolint:gosec // G702: the CLI selects a local executable and passes argv directly, never through a shell.
	cmd := exec.CommandContext(ctx, binary, args...)
	//nolint:exhaustruct_v5 // Only process-group ownership is overridden; retain default credentials, descriptors and platform attributes.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	defer func() {
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
	}()
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var out, errout boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	cmd.WaitDelay = 2 * time.Second
	err := cmd.Run()
	code := 0
	if ctx.Err() != nil {
		return -1, out.String(), errout.String(), ctx.Err()
	}
	if err != nil {
		if ee, ok := errors.AsType[*exec.ExitError](err); ok {
			code = ee.ExitCode()
		} else {
			return -1, out.String(), errout.String(), err
		}
	}
	if out.overflow || errout.overflow {
		return code, out.String(), errout.String(), errors.New("command output exceeded diagnostic bound")
	}
	return code, out.String(), errout.String(), nil
}
func Run(ctx context.Context, binary, root string) (Result, error) {
	// Never accept a stale report from an earlier invocation.
	path := filepath.Join(root, ".arch-go/report.json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return Result{}, err
	}
	code, out, stderr, err := command(ctx, root, binary, "--json")
	//nolint:exhaustruct_v5 // Result initializes only this owner/phase; optional collaborators, buffers and counters keep their zero defaults.
	r := Result{Exit: code, Stdout: out, Stderr: stderr}
	if err != nil {
		return r, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return r, fmt.Errorf("arch-go missing report: %w; stderr: %s", err, stderr)
	}
	var envelope map[string]json.RawMessage
	if err = json.Unmarshal(data, &envelope); err != nil {
		return r, err
	}
	for _, k := range []string{"summary", "compliance", fieldCoverage} {
		if _, ok := envelope[k]; !ok {
			return r, fmt.Errorf("arch-go report missing %s", k)
		}
	}
	if err = json.Unmarshal(data, &r.Report); err != nil {
		return r, err
	}
	return r, nil
}
func passing(r Result) bool {
	return r.Exit == 0 && r.Report.Summary.Pass && r.Report.Compliance.Pass && r.Report.Coverage.Pass &&
		r.Report.Compliance.Rate == 100 &&
		r.Report.Coverage.Rate == 100
}
func Scan(root string) ([]File, error) {
	var files []File
	packageNames := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return scanDirectory(root, path, d)
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, scanErr := scanProductionFile(root, path, packageNames)
		if scanErr != nil {
			return scanErr
		}
		files = append(files, f)
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })
	return files, err
}
func within(path, root string) bool { return path == root || strings.HasPrefix(path, root+"/") }
func security(files []File) error {
	for _, f := range files {
		if strings.HasPrefix(f.File, "internal/service/") && f.File != "internal/service/bridge.go" {
			for _, i := range f.Imports {
				if within(i, "github.com/modelcontextprotocol/go-sdk") {
					return fmt.Errorf("SDK outside existing bridge: %s", f.File)
				}
			}
		}
	}
	return nil
}
func writeModule(root string, config []byte, files []File) error {
	if err := os.WriteFile(
		filepath.Join(root, "go.mod"),
		[]byte("module "+module+"\n\ngo 1.27.1\n"),
		0600,
	); err != nil {
		return err
	}
	//nolint:gosec // G703: root is a harness-owned synthetic workspace; files come from the local AST scanner, not a manifest.
	if err := os.WriteFile(filepath.Join(root, "arch-go.yml"), config, 0600); err != nil {
		return err
	}
	packages := map[string]map[string]bool{}
	for _, f := range files {
		dir := filepath.Dir(f.File)
		if packages[dir] == nil {
			packages[dir] = map[string]bool{}
		}
		for _, i := range f.Imports {
			packages[dir][i] = true
		}
	}
	for dir, imports := range packages {
		target := filepath.Join(root, dir)
		if err := os.MkdirAll(target, 0700); err != nil {
			return err
		}
		var src strings.Builder
		src.WriteString("package fixture\n")
		names := make([]string, 0, len(imports))
		for i := range imports {
			names = append(names, i)
		}
		sort.Strings(names)
		for _, i := range names {
			fmt.Fprintf(&src, "import _ %q\n", i)
		}
		src.WriteString("func Marker() {}\n")
		if err := os.WriteFile(filepath.Join(target, "fixture.go"), []byte(src.String()), 0600); err != nil {
			return err
		}
	}
	return nil
}
func synthetic(ctx context.Context, binary string, config []byte, files []File) (Result, error) {
	root, err := os.MkdirTemp("", "zl-arch-source-")
	if err != nil {
		return Result{}, err
	}
	defer os.RemoveAll(root)
	if err = writeModule(root, config, files); err != nil {
		return Result{}, err
	}
	return Run(ctx, binary, root)
}

type controlCase struct {
	name, pkg, dependency, kind string
	fail                        bool
}

//nolint:gochecknoglobals // Immutable architecture controls are shared by exact-config negative/positive verification.
var cases = []controlCase{
	{"domain-sql", "internal/domain", wireDatabaseSQL, wireDependency, true},
	{"app-sdk", "internal/app/search", wireGithubComModelcontextprotocolGoSdkMcp, wireDependency, true},
	{"mcp-storage", "internal/mcpserver", module + "/internal/storage", wireDependency, true},
	{"production-evaluation", "cmd/zl-mcp", module + "/internal/evaluation", wireDependency, true},
	{"domain-subpackage-sql", "internal/domain/sub", wireDatabaseSQL, wireDependency, true},
	{"uncovered-package", "internal/newboundary", "", fieldCoverage, true},
	{"root-package-uncovered", ".", "", fieldCoverage, true},
	{"shadow-package-uncovered", "internal/foo/zl-mcp/internal/domain", "", fieldCoverage, true},
	{"app-root-positive", "internal/app/search", module + "/internal/domain", wirePositive, false},
	{"app-subpackage-positive", "internal/app/search/sub", module + "/internal/domain", wirePositive, false},
	{"domain-http", "internal/domain", "net/http", wireDependency, true},
	{"archiveport-standard", wireInternalMobilebackupArchiveport, "net/http", wirePositive, false},
	{"archiveport-storage", wireInternalMobilebackupArchiveport, module + "/internal/storage", wireDependency, true},
	{"archiveport-external", wireInternalMobilebackupArchiveport, "github.com/google/uuid", wireDependency, true},
}

func controlFiles(c controlCase) []File {
	f := File{File: filepath.ToSlash(filepath.Join(c.pkg, "fixture.go")), Imports: []string{}}
	if c.dependency != "" {
		f.Imports = append(f.Imports, c.dependency)
	}
	files := []File{f}
	if suffix, local := strings.CutPrefix(c.dependency, module+"/"); local {
		files = append(
			files,
			File{File: suffix + "/fixture.go", Imports: []string{}},
		)
	}
	return files
}
func expected(c controlCase, r Result) error {
	if !c.fail {
		if !passing(r) {
			return fmt.Errorf("positive control failed: %s", c.name)
		}
		return nil
	}
	if r.Exit == 0 || r.Report.Summary.Pass {
		return fmt.Errorf("negative control accepted: %s", c.name)
	}
	if c.kind == fieldCoverage {
		if r.Report.Coverage.Pass {
			return fmt.Errorf("control did not fail coverage: %s", c.name)
		}
	} else if r.Report.Compliance.Pass || !dependencyRejection(r.Report.Compliance.Details, c.pkg, c.dependency) {
		return fmt.Errorf("control failed for wrong dependency reason: %s", c.name)
	}
	return nil
}

// dependencyRejection requires a failed package's diagnostic, never rule text or loader output.
func dependencyRejection(raw json.RawMessage, pkg, dependency string) bool {
	var details struct {
		Rules struct {
			Details []struct {
				Pass     bool `json:"pass"`
				Packages []struct {
					Package string   `json:"package"`
					Pass    bool     `json:"pass"`
					Details []string `json:"details"`
				} `json:"packageDetails"`
			} `json:"details"`
		} `json:"dependenciesRules"`
	}
	if json.Unmarshal(raw, &details) != nil {
		return false
	}
	for _, rule := range details.Rules.Details {
		if rule.Pass {
			continue
		}
		for _, p := range rule.Packages {
			if p.Pass || p.Package != module+"/"+filepath.ToSlash(pkg) {
				continue
			}
			for _, reason := range p.Details {
				if strings.Contains(reason, "imported package '"+dependency+"'") {
					return true
				}
			}
		}
	}
	return false
}
func Controls(ctx context.Context, binary string, config []byte) ([]Control, error) {
	var controls []Control
	for _, c := range cases {
		r, err := synthetic(ctx, binary, config, controlFiles(c))
		if err != nil {
			return controls, err
		}
		if err = expected(c, r); err != nil {
			return controls, err
		}
		controls = append(controls, Control{Name: c.name, Status: codePass, Result: r})
	}
	return controls, nil
}

//nolint:gocognit // Each adversarial fixture keeps write, scan, loader validation and verdict together; loader errors must never count as rejection.
func SourceControls(ctx context.Context, binary string, config []byte) ([]Control, error) {
	cases := []struct{ name, path, tag, dep, reason string }{
		{
			"platform-sdk",
			"internal/service/illegal_windows.go",
			"",
			wireGithubComModelcontextprotocolGoSdkMcp,
			wireSecurity,
		},
		{
			"tagged-sdk",
			"internal/service/illegal.go",
			"//go:build architecture_hidden\n\n",
			wireGithubComModelcontextprotocolGoSdkMcp,
			wireSecurity,
		},
		{"cgo-sql", "internal/domain/illegal.go", "//go:build cgo\n\n", wireDatabaseSQL, wireDependency},
		{"platform-uncovered", "internal/newboundary/illegal_windows.go", "", "", fieldCoverage},
		{"platform-domain-positive", "internal/domain/legal_windows.go", "", "context", wirePositive},
		{
			"tagged-app-positive",
			"internal/app/search/legal.go",
			"//go:build architecture_hidden\n\n",
			module + "/internal/domain",
			wirePositive,
		},
	}
	var out []Control
	for _, c := range cases {
		root, err := os.MkdirTemp("", "zl-arch-source-control-")
		if err != nil {
			return out, err
		}
		path := filepath.Join(root, c.path)
		err = os.MkdirAll(filepath.Dir(path), 0700)
		if err == nil {
			src := c.tag + "package fixture\n"
			if c.dep != "" {
				src += fmt.Sprintf("import _ %q\n", c.dep)
			}
			err = os.WriteFile(path, []byte(src), 0600)
		}
		files, e := Scan(root)
		//nolint:gosec // G104: process/temporary-workspace cleanup cannot replace the original failure or watchdog outcome.
		os.RemoveAll(root)
		if err != nil {
			return out, err
		}
		if e != nil {
			return out, e
		}
		if c.reason == wireSecurity {
			if e = security(files); e == nil || !strings.Contains(e.Error(), "SDK outside existing bridge") {
				return out, fmt.Errorf("hidden SDK control accepted: %s", c.name)
			}
			//nolint:exhaustruct_v5 // Control initializes only this owner/phase; optional collaborators, buffers and counters keep their zero defaults.
			out = append(out, Control{Name: c.name, Status: codePass})
			continue
		}
		r, e := synthetic(ctx, binary, config, files)
		if e != nil {
			return out, e
		}
		cc := controlCase{
			name:       c.name,
			pkg:        filepath.Dir(c.path),
			dependency: c.dep,
			kind:       c.reason,
			fail:       c.reason != wirePositive,
		}
		if e = expected(cc, r); e != nil {
			return out, e
		}
		out = append(out, Control{Name: c.name, Status: codePass, Result: r})
	}
	return out, nil
}
func validateConfig(config []byte) error {
	var doc struct {
		Version   int `yaml:"version"`
		Threshold struct {
			Compliance int `yaml:"compliance"`
			Coverage   int `yaml:"coverage"`
		} `yaml:"threshold"`
		Rules []any `yaml:"dependenciesRules"`
	}
	if err := yaml.Unmarshal(config, &doc); err != nil {
		return err
	}
	if doc.Version != 1 || doc.Threshold.Compliance != 100 || doc.Threshold.Coverage != 100 || len(doc.Rules) == 0 {
		return errors.New("architecture config must retain version 1 and 100% thresholds")
	}
	return nil
}

//nolint:nonamedreturns // Deferred failure handling annotates the returned Evidence without losing the original error.
func Check(ctx context.Context, root, binary string) (e Evidence, err error) {
	e.Status = "FAIL"
	defer func() {
		if err != nil {
			e.Error = err.Error()
		}
	}()
	config, err := os.ReadFile(filepath.Join(root, "arch-go.yml"))
	if err != nil {
		return e, err
	}
	if err = validateConfig(config); err != nil {
		return e, err
	}
	if err = validateClosedExceptions(root); err != nil {
		return e, err
	}
	code, info, _, err := command(ctx, root, "go", "version", "-m", binary)
	if err != nil {
		return e, err
	}
	if code != 0 || !strings.Contains(info, Pin) {
		return e, errors.New("arch-go binary module pin mismatch")
	}
	e.BuildInfo = info
	e.Active, err = Run(ctx, binary, root)
	if err != nil {
		return e, err
	}
	if !passing(e.Active) {
		return e, errors.New("active architecture rules failed")
	}
	e.Files, err = Scan(root)
	if err != nil {
		return e, err
	}
	if err = security(e.Files); err != nil {
		return e, err
	}
	e.Source, err = synthetic(ctx, binary, config, e.Files)
	if err != nil {
		return e, err
	}
	if !passing(e.Source) {
		return e, errors.New("all-build source architecture failed")
	}
	code, deps, stderr, err := command(ctx, root, "go", "list", "-deps", "./cmd/zl-mcp")
	if err != nil {
		return e, err
	}
	if code != 0 {
		return e, fmt.Errorf("production graph loading failed: %s", stderr)
	}
	e.Dependencies = strings.Fields(deps)
	if err = validateProductionDependencies(e.Dependencies); err != nil {
		return e, err
	}
	e.Controls, err = Controls(ctx, binary, config)
	if err != nil {
		return e, err
	}
	e.SourceControls, err = SourceControls(ctx, binary, config)
	if err != nil {
		return e, err
	}
	e.Status = codePass
	return e, nil
}

// WriteEvidence writes complete diagnostics atomically, including failures.
func WriteEvidence(path string, e Evidence) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".architecture-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err = enc.Encode(e); err != nil {
		//nolint:gosec // G104: process/temporary-workspace cleanup cannot replace the original failure or watchdog outcome.
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

var _ io.Writer = (*boundedBuffer)(nil)

func scanDirectory(root, path string, entry os.DirEntry) error {
	if path == root {
		return nil
	}
	if strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "testdata" {
		return filepath.SkipDir
	}
	if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
		return filepath.SkipDir
	}
	return nil
}

func validateClosedExceptions(root string) error {
	var manifest struct {
		Exceptions []json.RawMessage `json:"exceptions"`
	}
	data, err := os.ReadFile(filepath.Join(root, "docs/mcp-migration/architecture-exceptions.json"))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err = json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if _, ok := raw["exceptions"]; !ok || manifest.Exceptions == nil || len(manifest.Exceptions) != 0 {
		return errors.New("temporary architecture exceptions must remain closed")
	}
	return nil
}

func validateProductionDependencies(dependencies []string) error {
	for _, dep := range dependencies {
		for _, forbidden := range []string{module + "/internal/evaluation", module + "/internal/evalfixture", "github.com/skosovsky/evaly"} {
			if within(dep, forbidden) {
				return fmt.Errorf("evaluation in production graph: %s", dep)
			}
		}
	}
	return nil
}

func scanProductionFile(root, path string, packageNames map[string]string) (File, error) {
	ast, e := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
	if e != nil {
		return File{}, e
	}
	dir := filepath.Dir(path)
	if name, ok := packageNames[dir]; ok && name != ast.Name.Name {
		return File{}, fmt.Errorf("conflicting production package names: %s", dir)
	}
	packageNames[dir] = ast.Name.Name
	rel, e := filepath.Rel(root, path)
	if e != nil {
		return File{}, e
	}
	f := File{File: filepath.ToSlash(rel), Imports: []string{}}
	for _, i := range ast.Imports {
		v, e := strconv.Unquote(i.Path.Value)
		if e != nil {
			return File{}, e
		}
		if v != "C" {
			f.Imports = append(f.Imports, v)
		}
	}
	sort.Strings(f.Imports)
	return f, nil
}
