package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/skosovsky/zl-mcp/tools/harness/internal/architecture"
	"github.com/skosovsky/zl-mcp/tools/harness/internal/performance"
)

func run(args []string) int {
	if len(args) > 0 && args[0] != "architecture" {
		return performanceCommand(args)
	}
	if len(args) == 0 || args[0] != "architecture" {
		fmt.Fprintln(os.Stderr, "usage: harness architecture --root PATH --binary PATH [--evidence PATH]")
		return 2
	}
	flags := flag.NewFlagSet("architecture", flag.ContinueOnError)
	root := flags.String("root", "", "repository root")
	binary := flags.String("binary", "", "pinned arch-go binary")
	evidence := flags.String("evidence", "", "JSON evidence output")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *root == "" || *binary == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "--root and --binary are required")
		return 2
	}
	r, err := filepath.Abs(*root)
	if err != nil {
		return 2
	}
	b, err := filepath.Abs(*binary)
	if err != nil {
		return 2
	}
	result, err := architecture.Check(context.Background(), r, b)
	if *evidence != "" {
		if writeErr := architecture.WriteEvidence(*evidence, result); writeErr != nil {
			fmt.Fprintln(os.Stderr, writeErr)
			return 1
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	//nolint:forbidigo // This executable writes the documented CLI result to stdout.
	fmt.Println(
		"architecture PASS: host and all-build compliance/coverage 100%, closed exceptions, production graph, 14 isolated and 6 source controls",
	)
	return 0
}
func main() { os.Exit(run(os.Args[1:])) }

func performanceCommand(args []string) int {
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	root := flags.String("root", ".", "repository root")
	output := flags.String("output", "", "artifact output path")
	var execute func() error
	switch args[0] {
	case "current-build":
		execute = func() error { return performance.Build(context.Background(), *root, *output) }
	case "bench-mcp":
		binary := flags.String("binary", "", "current evaluation executable")
		build := flags.String("build-manifest", "", "successful build manifest")
		cpu := flags.String("cpu-model", "", "verified host CPU model")
		execute = func() error {
			if *binary == "" || *build == "" {
				return errors.New("--binary and --build-manifest required")
			}
			return performance.Bench(context.Background(), *root, *binary, *build, *output, *cpu)
		}
	case "compare-performance":
		baseline := flags.String("baseline", "", "fixed raw baseline manifest")
		candidate := flags.String("candidate", "", "current raw manifest")
		bb := flags.String("baseline-build", "", "relocated archived build manifest")
		cb := flags.String("candidate-build", "", "relocated current build manifest")
		execute = func() error {
			if *baseline == "" || *candidate == "" {
				return errors.New("--baseline and --candidate required")
			}
			return performance.CompareFiles(*baseline, *candidate, *bb, *cb, *output)
		}
	default:
		fmt.Fprintln(os.Stderr, "unknown harness command:", args[0])
		return 2
	}
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if *output == "" || flags.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "--output required; positional arguments forbidden")
		return 2
	}
	if err := execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	//nolint:forbidigo // This executable writes the documented CLI result to stdout.
	fmt.Println(args[0], "PASS")
	return 0
}
