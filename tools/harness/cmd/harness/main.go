package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/skosovsky/zl-mcp/tools/harness/internal/architecture"
)

func run(args []string) int {
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
	fmt.Println("architecture PASS: host and all-build compliance/coverage 100%, closed exceptions, production graph, 10 isolated and 6 source controls")
	return 0
}
func main() { os.Exit(run(os.Args[1:])) }
