package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/skosovsky/zl-mcp/internal/logging"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/control"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/service"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/zalo"
)

func main() {
	if e := run(); e != nil {
		if serviceInvocation() {
			logging.ReportStartupFailure(e)
			info, statErr := os.Stderr.Stat()
			if statErr == nil && info.Mode()&os.ModeCharDevice != 0 {
				fmt.Fprintln(os.Stderr, e)
			}
		} else {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}
}
func run() error {
	flags := flag.NewFlagSet("zl-mcp", flag.ContinueOnError)
	if serviceInvocation() {
		flags.SetOutput(io.Discard)
	}
	path := flags.String("config", "config.toml", "local TOML configuration")
	if e := flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	if flags.NArg() < 1 {
		return fmt.Errorf("usage: zl-mcp -config config.toml <login|service|serve|probe-preload|mobile-backup-prepare|mobile-backup-status|mobile-backup-cancel|mobile-backup-probe-offer|mobile-backup-probe-archive|probe-recall|account-archive-prepare|account-archive-capture|account-archive-status|account-archive-preserve|account-archive-restore|account-archive-inspect|account-archive-remove|approve-join preview_id>")
	}
	c, e := config.Load(*path)
	if e != nil {
		return e
	}
	if flags.Arg(0) != "serve" && !mobileLedgerCommand(flags.Arg(0)) {
		if e = c.Prepare(); e != nil {
			return e
		}
	}
	level := slog.LevelInfo
	switch c.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	switch flags.Arg(0) {
	case "probe-recall", "mobile-backup-prepare", "mobile-backup-status", "mobile-backup-cancel", "mobile-backup-probe-offer", "mobile-backup-probe-archive", "account-archive-prepare", "account-archive-capture", "account-archive-status", "account-archive-preserve", "account-archive-restore", "account-archive-inspect", "account-archive-remove":
		return runMobileLedger(ctx, c.StateDir, flags.Args(), os.Stdin, os.Stdout)
	case "service":
		if flags.NArg() != 1 {
			return fmt.Errorf("service takes no arguments")
		}
		return service.Run(ctx, c)

	case "serve":
		if flags.NArg() != 1 {
			return fmt.Errorf("serve takes no arguments")
		}
		return service.BridgeStdio(ctx, c)
	case "probe-preload":
		if flags.NArg() != 1 {
			return fmt.Errorf("probe-preload takes no arguments")
		}
		probeCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		defer cancel()
		result, err := control.New(c.StateDir).Call(probeCtx, "cli_probe_preload", map[string]any{})
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "approve-join":
		if flags.NArg() != 2 {
			return fmt.Errorf("approve-join requires preview_id")
		}
		stat, e := os.Stdin.Stat()
		if e != nil || stat.Mode()&os.ModeCharDevice == 0 {
			return fmt.Errorf("approval requires an interactive local terminal")
		}
		client := control.New(c.StateDir)
		q, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		preview, e := client.Call(q, "cli_preview", map[string]any{"preview_id": flags.Arg(1)})
		if e != nil {
			return e
		}
		fmt.Fprintf(os.Stderr, "Join group %q (ID %s, moderator approval: %v)? Type JOIN to approve: ", preview["name"], preview["group_id"], preview["approval_required"])
		line, e := bufio.NewReader(os.Stdin).ReadString('\n')
		if e != nil {
			return e
		}
		if strings.TrimSpace(line) != "JOIN" {
			return fmt.Errorf("approval cancelled")
		}
		approveCtx, approveCancel := context.WithTimeout(ctx, 5*time.Second)
		defer approveCancel()
		result, e := client.Call(approveCtx, "cli_approve", map[string]any{"preview_id": flags.Arg(1)})
		if e != nil {
			return e
		}
		fmt.Fprintln(os.Stdout, result["plan_token"])
		return nil
	case "login":
		if flags.NArg() != 1 {
			return fmt.Errorf("login takes no arguments")
		}
		unlock, e := local.Lock(c.StateDir)
		if e != nil {
			return e
		}
		defer unlock()
		loginCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()
		client, e := zalo.Login(loginCtx, c.StateDir, os.Stderr)
		if e != nil {
			return e
		}
		store, e := storage.OpenWithPolicy(ctx, filepath.Join(c.StateDir, "messages.sqlite"), c.Policy(), c.Storage.RetentionDays)
		if e != nil {
			return e
		}
		defer store.Close()
		if e = store.BindAccount(ctx, client.AccountID()); e != nil {
			return e
		}
		if e = client.Save(c.StateDir); e != nil {
			return e
		}
		fmt.Fprintln(os.Stderr, "Session saved locally. QR image removed.")
		return nil
	default:
		return fmt.Errorf("unknown command; available: login, service, serve, approve-join, probe-preload, probe-recall, mobile-backup-prepare, mobile-backup-status, mobile-backup-cancel, mobile-backup-probe-offer, mobile-backup-probe-archive")
	}
}

func serviceInvocation() bool { return len(os.Args) > 1 && os.Args[len(os.Args)-1] == "service" }
