package performance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func hostCommand(ctx context.Context, root string, args ...string) ([]byte, error) {
	return boundedHostCommand(ctx, hostCommandTimeout, root, args...)
}

// Even callers with no deadline cannot hang on host metadata/toolchain commands.
func boundedHostCommand(ctx context.Context, budget time.Duration, root string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	env := append(os.Environ(), "GOWORK=off")
	out, stderr, _, err := process(ctx, root, env, args, commandOutputLimit)
	if err != nil {
		return nil, fmt.Errorf("%s: %w: %s", args[0], err, stderr)
	}
	return out, nil
}
func SourceFingerprint(ctx context.Context, root string) (Object, error) {
	raw, e := hostCommand(ctx, root, "git", "ls-files", "--cached", "--others", "--exclude-standard", "-z")
	if e != nil {
		return nil, e
	}
	names := strings.Split(string(raw), "\x00")
	sort.Strings(names)
	files := []Object{}
	last := ""
	for _, name := range names {
		if name == "" || name == last {
			continue
		}
		last = name
		if strings.HasPrefix(name, "docs/mcp-migration/evidence/") || strings.HasPrefix(name, "docs/lint-evidence/") ||
			name == "docs/mcp-migration/spec-review.md" {
			continue
		}
		path := filepath.Join(root, name)
		info, statErr := os.Stat(path)
		if os.IsNotExist(statErr) {
			continue
		}
		if statErr != nil {
			return nil, statErr
		}
		if !info.Mode().IsRegular() {
			continue
		}
		hash, statErr := fileDigest(path)
		if statErr != nil {
			return nil, statErr
		}
		files = append(files, Object{wirePath: name, wireSHA256: hash, "executable": info.Mode().Perm()&0111 != 0})
	}
	canonical, e := json.Marshal(files)
	if e != nil {
		return nil, e
	}
	return Object{
		"source_digest_sha256": digest(canonical),
		"algorithm":            "go-json-sorted-path-sha256-executable-v1",
		"files":                files,
	}, nil
}

// Build compiles only the current evaluation executable. No git archive or overlay.
func Build(ctx context.Context, root, output string) error {
	root, e := filepath.Abs(root)
	if e != nil {
		return e
	}
	output, e = filepath.Abs(output)
	if e != nil {
		return e
	}
	//nolint:gosec // G301: this directory holds shareable synthetic evidence/executables, never account state.
	if e = os.Mkdir(output, 0755); e != nil {
		return e
	}
	manifest := Object{fieldStatus: codeFail, "builds": []any{}, wireModel: codeNotRun, wireLive: codeNotRun}
	resultErr := buildCandidate(ctx, root, output, manifest)
	if resultErr != nil {
		manifest["error"] = resultErr.Error()
	}
	if e = Write(filepath.Join(output, "build-manifest.json"), manifest); e != nil {
		return e
	}
	return resultErr
}

func buildCandidate(ctx context.Context, root, output string, manifest Object) error {
	ctx, cancel := context.WithTimeout(ctx, candidateBuildTimeout)
	defer cancel()
	head, e := hostCommand(ctx, root, "git", "rev-parse", "HEAD")
	if e != nil {
		return e
	}
	toolchain, e := hostCommand(ctx, root, "go", "version")
	if e != nil {
		return e
	}
	if toolchainVersion(string(toolchain)) != candidateGoVersion {
		return errors.New("requires pinned go1.27.2")
	}
	sources, e := SourceFingerprint(ctx, root)
	if e != nil {
		return e
	}
	manifest["candidate_parent_sha"] = strings.TrimSpace(string(head))
	manifest["toolchain_pin"] = candidateGoVersion
	manifest["toolchain"] = strings.TrimSpace(string(toolchain))
	manifest["candidate_sources"] = sources
	binary := filepath.Join(output, "zl-mcp-eval-candidate")
	args := []string{"go", "build", "-trimpath", "-o", binary, wireCmdZlMcpEval}
	env := append(os.Environ(), "GOWORK=off")
	out, stderr, code, e := process(ctx, root, env, args, commandOutputLimit)
	//nolint:gosec // G306: this synthetic evidence contains build/host provenance, never account credentials.
	logErr := os.WriteFile(filepath.Join(output, "candidate.build.log"), append(out, stderr...), 0644)
	record := Object{
		"kind":        wireCandidate,
		"target":      wireCmdZlMcpEval,
		"command":     args,
		"exit_status": code,
		"binary":      binary,
	}
	manifest["builds"] = []any{record}
	if logErr != nil {
		return logErr
	}
	if e != nil {
		return e
	}
	hash, e := fileDigest(binary)
	if e != nil {
		return e
	}
	info, e := hostCommand(ctx, root, "go", "version", "-m", binary)
	if e != nil {
		return e
	}
	record["binary_sha256"] = hash
	record["build_info"] = string(info)
	after, e := SourceFingerprint(ctx, root)
	if e != nil {
		return e
	}
	if after["source_digest_sha256"] != sources["source_digest_sha256"] {
		return errors.New("source tree changed during build")
	}
	manifest[fieldStatus] = codePass
	return nil
}
