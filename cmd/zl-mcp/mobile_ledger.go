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
	return name == "mobile-backup-prepare" || name == "mobile-backup-status" || name == "mobile-backup-cancel"
}
func mobileLedgerArguments(args []string, in io.Reader) (string, any, error) {
	if len(args) == 0 {
		return "", nil, fmt.Errorf("mobile ledger command required")
	}
	switch args[0] {
	case "mobile-backup-prepare":
		if len(args) != 1 {
			return "", nil, fmt.Errorf("mobile-backup-prepare reads request JSON from stdin")
		}
		body, err := io.ReadAll(io.LimitReader(in, (16<<10)+1))
		if err != nil || len(body) > 16<<10 || !json.Valid(body) {
			return "", nil, fmt.Errorf("invalid or oversized mobile request JSON")
		}
		// Preserve raw keys for the service's duplicate-key and schema checks.
		return "cli_prepare_mobile_backup", json.RawMessage(body), nil
	case "mobile-backup-status":
		if len(args) != 2 {
			return "", nil, fmt.Errorf("mobile-backup-status requires operation_id")
		}
		return "cli_mobile_backup_status", map[string]any{"operation_id": args[1]}, nil
	case "mobile-backup-cancel":
		if len(args) != 3 {
			return "", nil, fmt.Errorf("mobile-backup-cancel requires operation_id and revision")
		}
		revision, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil || revision < 0 || strconv.FormatInt(revision, 10) != args[2] {
			return "", nil, fmt.Errorf("revision must be a nonnegative canonical integer")
		}
		return "cli_cancel_prepared_mobile_backup", map[string]any{"operation_id": args[1], "revision": revision}, nil
	}
	return "", nil, fmt.Errorf("unknown mobile ledger command")
}
func runMobileLedger(ctx context.Context, stateDir string, args []string, in io.Reader, out io.Writer) error {
	method, arguments, err := mobileLedgerArguments(args, in)
	if err != nil {
		return err
	}
	request, stop := context.WithTimeout(ctx, 10*time.Second)
	defer stop()
	result, err := control.New(stateDir).Call(request, method, arguments)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
