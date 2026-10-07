package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/skosovsky/zl-mcp/internal/control"
)

func mobileLedgerCommand(name string) bool {
	return name == "account-archive-restore" || name == "account-archive-preserve" || name == "account-archive-inspect" || name == "account-archive-remove" || name == "account-archive-prepare" || name == "account-archive-capture" || name == "account-archive-status" || name == "probe-recall" || name == "mobile-backup-prepare" || name == "mobile-backup-status" || name == "mobile-backup-cancel" || name == "mobile-backup-probe-offer" || name == "mobile-backup-probe-archive"
}
func mobileLedgerArguments(args []string, in io.Reader) (string, any, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("mobile ledger command required")
	}
	switch args[0] {
	case "probe-recall":
		if len(args) != 2 {
			return "", nil, fmt.Errorf("probe-recall requires the exact sent request UUID")
		}
		return "cli_probe_recall", map[string]any{"send_request_id": args[1]}, nil
	case "mobile-backup-prepare", "account-archive-prepare", "account-archive-inspect", "account-archive-preserve", "account-archive-restore":
		if len(args) != 1 {
			return "", nil, fmt.Errorf("mobile-backup-prepare reads request JSON from stdin")
		}
		body, err := io.ReadAll(io.LimitReader(in, (16<<10)+1))
		if err != nil || len(body) > 16<<10 || !json.Valid(body) {
			return "", nil, fmt.Errorf("invalid or oversized mobile request JSON")
		}
		// Preserve raw keys for the service's duplicate-key and schema checks.
		method := "cli_prepare_mobile_backup"
		if args[0] == "account-archive-prepare" {
			method = "cli_prepare_account_archive"
		}
		if args[0] == "account-archive-inspect" {
			method = "cli_inspect_account_archive"
		}
		if args[0] == "account-archive-preserve" {
			method = "cli_preserve_account_archive"
		}
		if args[0] == "account-archive-restore" {
			method = "cli_restore_account_archive"
		}
		return method, json.RawMessage(body), nil
	case "account-archive-remove":
		if len(args) != 2 && len(args) != 3 {
			return "", nil, fmt.Errorf("account-archive-remove requires source_id and optional cache|library")
		}
		values := map[string]any{"source_id": args[1]}
		if len(args) == 3 {
			values["source_storage"] = args[2]
		}
		return "cli_remove_account_archive", values, nil
	case "account-archive-status":
		if len(args) != 2 {
			return "", nil, fmt.Errorf("account-archive-status requires source_id")
		}
		return "cli_account_archive_status", map[string]any{"source_id": args[1]}, nil
	case "mobile-backup-status":
		if len(args) != 2 {
			return "", nil, fmt.Errorf("mobile-backup-status requires operation_id")
		}
		return "cli_mobile_backup_status", map[string]any{"operation_id": args[1]}, nil
	case "mobile-backup-cancel", "mobile-backup-probe-offer", "mobile-backup-probe-archive", "account-archive-capture":
		if len(args) != 3 && !(args[0] == "mobile-backup-probe-archive" && len(args) == 4) {
			return "", nil, fmt.Errorf("%s requires operation_id and revision", args[0])
		}
		revision, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil || revision < 0 || strconv.FormatInt(revision, 10) != args[2] {
			return "", nil, fmt.Errorf("revision must be a nonnegative canonical integer")
		}
		method := "cli_cancel_prepared_mobile_backup"
		if args[0] == "mobile-backup-probe-offer" {
			method = "cli_probe_mobile_backup_offer"
		}
		if args[0] == "mobile-backup-probe-archive" {
			method = "cli_probe_mobile_backup_archive"
		}
		if args[0] == "account-archive-capture" {
			method = "cli_capture_account_archive"
		}
		arguments := map[string]any{"operation_id": args[1], "revision": revision}
		if len(args) == 4 {
			arguments["comparison_send_request_id"] = args[3]
		}
		return method, arguments, nil
	}
	return "", nil, fmt.Errorf("unknown mobile ledger command")
}
func runMobileLedger(ctx context.Context, stateDir string, args []string, in io.Reader, out io.Writer) error {
	method, arguments, err := mobileLedgerArguments(args, in)
	if err != nil {
		return err
	}
	budget := 10 * time.Second
	if method == "cli_inspect_account_archive" || method == "cli_preserve_account_archive" || method == "cli_restore_account_archive" {
		budget = 140 * time.Second
	}
	if method == "cli_probe_recall" {
		budget = 40 * time.Second
	}
	if method == "cli_probe_mobile_backup_offer" {
		budget = 205 * time.Second
	}
	if method == "cli_probe_mobile_backup_archive" || method == "cli_capture_account_archive" {
		budget = 445 * time.Second
	}
	request, stop := context.WithTimeout(ctx, budget)
	defer stop()
	result, err := control.New(stateDir).Call(request, method, arguments)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
