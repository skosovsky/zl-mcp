package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/skosovsky/zl-mcp/internal/control"
)

func mobileLedgerCommand(name string) bool {
	return name == commandAccountArchiveRestore || name == commandAccountArchivePreserve ||
		name == commandAccountArchiveInspect ||
		name == commandAccountArchiveRemove ||
		name == commandAccountArchivePrepare ||
		name == commandAccountArchiveCapture ||
		name == commandAccountArchiveStatus ||
		name == commandProbeRecall ||
		name == commandMobileBackupPrepare ||
		name == commandMobileBackupStatus ||
		name == commandMobileBackupCancel ||
		name == commandMobileBackupProbeOffer ||
		name == commandMobileBackupProbeArchive
}

//nolint:cyclop,funlen,gocognit,gocyclo // The closed CLI command grammar keeps exact arity and canonical revision validation next to each protocol command; raw JSON keys remain unchanged.
func mobileLedgerArguments(args []string, in io.Reader) (string, any, error) {
	if len(args) == 0 {
		return "", nil, errors.New("mobile ledger command required")
	}
	switch args[0] {
	case commandProbeRecall:
		if len(args) != 2 {
			return "", nil, errors.New("probe-recall requires the exact sent request UUID")
		}
		return operationProbeRecall, map[string]any{"send_request_id": args[1]}, nil
	case commandMobileBackupPrepare,
		commandAccountArchivePrepare,
		commandAccountArchiveInspect,
		commandAccountArchivePreserve,
		commandAccountArchiveRestore:
		if len(args) != 1 {
			return "", nil, errors.New("mobile-backup-prepare reads request JSON from stdin")
		}
		body, err := io.ReadAll(io.LimitReader(in, maximumOwnerRequestBytes+1))
		if err != nil || len(body) > 16<<10 || !json.Valid(body) {
			return "", nil, errors.New("invalid or oversized mobile request JSON")
		}
		// Preserve raw keys for the service's duplicate-key and schema checks.
		method := operationPrepareMobileBackup
		if args[0] == commandAccountArchivePrepare {
			method = operationPrepareAccountArchive
		}
		if args[0] == commandAccountArchiveInspect {
			method = operationInspectAccountArchive
		}
		if args[0] == commandAccountArchivePreserve {
			method = operationPreserveAccountArchive
		}
		if args[0] == commandAccountArchiveRestore {
			method = operationRestoreAccountArchive
		}
		return method, json.RawMessage(body), nil
	case commandAccountArchiveRemove:
		if len(args) != 2 && len(args) != mobileLedgerArgumentCount {
			return "", nil, errors.New("account-archive-remove requires source_id and optional cache|library")
		}
		values := map[string]any{fieldSourceID: args[1]}
		if len(args) == mobileLedgerArgumentCount {
			values["source_storage"] = args[2]
		}
		return operationRemoveAccountArchive, values, nil
	case commandAccountArchiveStatus:
		if len(args) != 2 {
			return "", nil, errors.New("account-archive-status requires source_id")
		}
		return operationAccountArchiveStatus, map[string]any{fieldSourceID: args[1]}, nil
	case commandMobileBackupStatus:
		if len(args) != 2 {
			return "", nil, errors.New("mobile-backup-status requires operation_id")
		}
		return "cli_mobile_backup_status", map[string]any{"operation_id": args[1]}, nil
	case commandMobileBackupCancel,
		commandMobileBackupProbeOffer,
		commandMobileBackupProbeArchive,
		commandAccountArchiveCapture:
		if len(args) != mobileLedgerArgumentCount &&
			(args[0] != commandMobileBackupProbeArchive || len(args) != mobileComparisonArgumentCount) {
			return "", nil, fmt.Errorf("%s requires operation_id and revision", args[0])
		}
		revision, err := strconv.ParseInt(args[2], 10, 64)
		if err != nil || revision < 0 || strconv.FormatInt(revision, 10) != args[2] {
			return "", nil, errors.New("revision must be a nonnegative canonical integer")
		}
		method := operationCancelPreparedMobileBackup
		if args[0] == commandMobileBackupProbeOffer {
			method = operationProbeMobileBackupOffer
		}
		if args[0] == commandMobileBackupProbeArchive {
			method = operationProbeMobileBackupArchive
		}
		if args[0] == commandAccountArchiveCapture {
			method = operationCaptureAccountArchive
		}
		arguments := map[string]any{"operation_id": args[1], "revision": revision}
		if len(args) == mobileComparisonArgumentCount {
			arguments["comparison_send_request_id"] = args[3]
		}
		return method, arguments, nil
	}
	return "", nil, errors.New("unknown mobile ledger command")
}
func runMobileLedger(ctx context.Context, stateDir string, args []string, in io.Reader, out io.Writer) error {
	method, arguments, err := mobileLedgerArguments(args, in)
	if err != nil {
		return err
	}
	budget := defaultOwnerRequestTimeout
	if method == operationInspectAccountArchive || method == operationPreserveAccountArchive ||
		method == operationRestoreAccountArchive {
		budget = archiveAdministrationTimeout
	}
	if method == operationProbeRecall {
		budget = recallCommandTimeout
	}
	if method == operationProbeMobileBackupOffer {
		budget = mobileOfferCommandTimeout
	}
	if method == operationProbeMobileBackupArchive || method == operationCaptureAccountArchive {
		budget = mobileCaptureCommandTimeout
	}
	request, stop := context.WithTimeout(ctx, budget)
	defer stop()
	result, err := control.New(stateDir).Call(request, method, arguments)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
