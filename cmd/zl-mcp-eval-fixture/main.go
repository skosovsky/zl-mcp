// zl-mcp-eval-fixture exposes the real MCP implementation over synthetic data.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/evalfixture"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() (resultErr error) {
	//nolint:exhaustruct_v5 // Fixture flags and New fill the remaining defaults; preserve unset optional policy/effect options.
	opts := evalfixture.Options{}
	statsPath := flag.String("stats-file", "", "write final synthetic mutation statistics to a new private file")
	flag.StringVar(&opts.Fixtures, "fixtures", "docs/evals/fixtures.json", "synthetic corpus path")
	flag.StringVar(&opts.SkillDir, "skill-dir", "", "expose the fixed skill documents as eval resources")
	flag.BoolVar(&opts.Empty, "empty", false, "start with no messages")
	flag.BoolVar(&opts.Stopped, "stopped", false, "report a stopped synthetic collector")
	flag.BoolVar(&opts.JoinTimeout, "join-timeout", false, "simulate an uncertain join outcome")
	flag.StringVar(&opts.ApprovalFile, "approval-file", "", "create synthetic prior approval in a new private file")
	flag.StringVar(
		&opts.ApprovalTokenFile,
		"approval-token-file",
		"",
		"seed a prior synthetic approval from a harness token file",
	)
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected positional arguments")
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, nil)))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fixture, err := evalfixture.New(ctx, opts)
	if err != nil {
		return err
	}
	defer func() {
		// JoinManager must finish before the harness reads its mutation count.
		closeErr := fixture.Close()
		if closeErr != nil && resultErr == nil {
			resultErr = closeErr
		}
		if *statsPath != "" {
			file, writeErr := os.OpenFile(*statsPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if writeErr == nil {
				writeErr = json.NewEncoder(file).
					Encode(map[string]any{"join_calls": fixture.Upstream.JoinCalls(), "cleanup_ok": closeErr == nil})
				if err := file.Close(); writeErr == nil {
					writeErr = err
				}
			}
			if writeErr != nil && resultErr == nil {
				resultErr = writeErr
			}
		}
		//nolint:sloglint // Use the service-configured redacting logger; fields remain a bounded allowlist without raw errors or native identities.
		slog.Info("synthetic_eval_finished", "join_calls", fixture.Upstream.JoinCalls(), "cleanup_ok", closeErr == nil)
	}()
	//nolint:exhaustruct_v5 // Preserve pinned SDK transport defaults for omitted limits; bounded ID and framing contracts are tested separately.
	return fixture.Server.Run(ctx, &mcp.StdioTransport{})
}
