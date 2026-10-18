package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/skosovsky/zl-mcp/internal/logging"

	"github.com/skosovsky/zl-mcp/internal/config"
	"github.com/skosovsky/zl-mcp/internal/control"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/zalo"
)

func RunCLI() error {
	flags := flag.NewFlagSet("zl-mcp", flag.ContinueOnError)
	if serviceInvocation() {
		flags.SetOutput(io.Discard)
	}
	path := flags.String("config", "config.toml", "local TOML configuration")
	if e := flags.Parse(os.Args[1:]); e != nil {
		return e
	}
	if flags.NArg() < 1 {
		return errors.New(
			"usage: zl-mcp -config config.toml <login|service|serve|check-rollback|probe-preload|mobile-backup-prepare|mobile-backup-status|mobile-backup-cancel|mobile-backup-probe-offer|mobile-backup-probe-archive|probe-recall|account-archive-prepare|account-archive-capture|account-archive-status|account-archive-preserve|account-archive-restore|account-archive-inspect|account-archive-remove|approve-join preview_id>",
		)
	}
	c, e := config.Load(*path)
	if e != nil {
		return e
	}
	if flags.Arg(0) != "serve" && flags.Arg(0) != "check-rollback" && !mobileLedgerCommand(flags.Arg(0)) {
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
	//nolint:exhaustruct_v5 // Keep default log level/filter behavior; the configured redaction/replacement policy is explicit.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return runCLICommand(ctx, c, flags)
}

func serviceInvocation() bool { return len(os.Args) > 1 && os.Args[len(os.Args)-1] == "service" }

// ReportCLIError preserves the existing startup logging and terminal behavior.
func ReportCLIError(err error) {
	if serviceInvocation() {
		logging.ReportStartupFailure(err)
		info, statErr := os.Stderr.Stat()
		if statErr == nil && info.Mode()&os.ModeCharDevice != 0 {
			fmt.Fprintln(os.Stderr, err)
		}
	} else {
		fmt.Fprintln(os.Stderr, err)
	}
}

func runCLICommand(ctx context.Context, c config.Config, flags *flag.FlagSet) error {
	switch flags.Arg(0) {
	case "check-rollback":
		if flags.NArg() != 1 {
			return errors.New("check-rollback takes no arguments")
		}
		return checkLegacyRollbackCommand(ctx, c.StateDir, os.Stdout)
	case commandProbeRecall,
		commandMobileBackupPrepare,
		commandMobileBackupStatus,
		commandMobileBackupCancel,
		commandMobileBackupProbeOffer,
		commandMobileBackupProbeArchive,
		commandAccountArchivePrepare,
		commandAccountArchiveCapture,
		commandAccountArchiveStatus,
		commandAccountArchivePreserve,
		commandAccountArchiveRestore,
		commandAccountArchiveInspect,
		commandAccountArchiveRemove:
		return runMobileLedger(ctx, c.StateDir, flags.Args(), os.Stdin, os.Stdout)
	case "service":
		if flags.NArg() != 1 {
			return errors.New("service takes no arguments")
		}
		return Run(ctx, c)

	case "serve":
		if flags.NArg() != 1 {
			return errors.New("serve takes no arguments")
		}
		return BridgeStdio(ctx, c)
	case "probe-message-action":
		if flags.NArg() != 1 {
			return errors.New("probe-message-action reads bounded request JSON from stdin")
		}
		args, err := messageActionProbeArguments(os.Stdin)
		if err != nil {
			return err
		}
		result, err := control.New(c.StateDir).Call(ctx, operationMessageActionProbe, args)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)

	case "probe-preload":
		return runPreloadProbeCommand(ctx, c, flags)

	case "approve-join":
		return runApproveJoinCommand(ctx, c, flags)

	case "login":
		return runLoginCommand(ctx, c, flags)

	default:
		return errors.New(
			"unknown command; available: login, service, serve, approve-join, probe-preload, probe-recall, mobile-backup-prepare, mobile-backup-status, mobile-backup-cancel, mobile-backup-probe-offer, mobile-backup-probe-archive",
		)
	}
}

func runPreloadProbeCommand(ctx context.Context, c config.Config, flags *flag.FlagSet) error {
	const selectorArgumentCount = 4
	const selectorMessageIDIndex = 3
	args := map[string]any{}
	switch flags.NArg() {
	case 1:
	case selectorArgumentCount:
		args["conversation_type"] = flags.Arg(1)
		args["conversation_id"] = flags.Arg(2)
		args["message_id"] = flags.Arg(selectorMessageIDIndex)
	default:
		return errors.New("probe-preload accepts no arguments or exact conversation_type conversation_id message_id")
	}
	probeCtx, cancel := context.WithTimeout(ctx, runPreloadProbeCommandProbeTimeout)
	defer cancel()
	result, err := control.New(c.StateDir).Call(probeCtx, "cli_probe_preload", args)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func runApproveJoinCommand(ctx context.Context, c config.Config, flags *flag.FlagSet) error {
	if flags.NArg() != 2 {
		return errors.New("approve-join requires preview_id")
	}
	stat, e := os.Stdin.Stat()
	if e != nil || stat.Mode()&os.ModeCharDevice == 0 {
		return errors.New("approval requires an interactive local terminal")
	}
	client := control.New(c.StateDir)
	q, cancel := context.WithTimeout(ctx, runApproveJoinCommandQTimeout)
	defer cancel()
	preview, e := client.Call(q, "cli_preview", map[string]any{"preview_id": flags.Arg(1)})
	if e != nil {
		return e
	}
	fmt.Fprintf(
		os.Stderr,
		"Join group %q (ID %s, moderator approval: %v)? Type JOIN to approve: ",
		preview["name"],
		preview["group_id"],
		preview["approval_required"],
	)
	line, e := bufio.NewReader(os.Stdin).ReadString('\n')
	if e != nil {
		return e
	}
	if strings.TrimSpace(line) != "JOIN" {
		return errors.New("approval cancelled")
	}
	approveCtx, approveCancel := context.WithTimeout(ctx, runApproveJoinCommandApproveTimeout)
	defer approveCancel()
	result, e := client.Call(approveCtx, "cli_approve", map[string]any{"preview_id": flags.Arg(1)})
	if e != nil {
		return e
	}
	fmt.Fprintln(os.Stdout, result["plan_token"])
	return nil
}

func runLoginCommand(ctx context.Context, c config.Config, flags *flag.FlagSet) error {
	if flags.NArg() != 1 {
		return errors.New("login takes no arguments")
	}
	unlock, e := local.Lock(c.StateDir)
	if e != nil {
		return e
	}
	defer unlock()
	loginCtx, cancel := context.WithTimeout(ctx, runLoginCommandLoginTimeout)
	defer cancel()
	client, e := zalo.Login(loginCtx, c.StateDir, os.Stderr)
	if e != nil {
		return e
	}
	store, e := storage.OpenWithPolicy(
		ctx,
		filepath.Join(c.StateDir, "messages.sqlite"),
		c.Policy(),
		c.Storage.RetentionDays,
	)
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
}
