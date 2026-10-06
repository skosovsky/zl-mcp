package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMobileLedgerArgumentsPreserveRawJSONForServerValidation(t *testing.T) {
	// Arrange: duplicate keys must reach the service as duplicate keys, not be collapsed.
	input := `{"request_id":"first","request_id":"second"}`
	// Act.
	method, args, e := mobileLedgerArguments([]string{"mobile-backup-prepare"}, strings.NewReader(input))
	b, encode := json.Marshal(args)
	// Assert.
	if e != nil || encode != nil || method != "cli_prepare_mobile_backup" || strings.Count(string(b), `"request_id"`) != 2 {
		t.Fatal("CLI lost duplicate-key evidence")
	}
}
func TestMobileLedgerArgumentsRejectInvalidCommandsBudgetsAndRevisions(t *testing.T) {
	for _, args := range [][]string{nil, {"mobile-backup-prepare", "extra"}, {"mobile-backup-status"}, {"mobile-backup-cancel", "id", "-1"}, {"mobile-backup-cancel", "id", "01"}, {"mobile-backup-cancel", "id", "9223372036854775808"}, {"unknown"}} {
		// Arrange / Act / Assert.
		if _, _, e := mobileLedgerArguments(args, strings.NewReader(`{}`)); e == nil {
			t.Fatal("invalid CLI arguments accepted")
		}
	}
	for _, input := range []string{`{} {}`, strings.Repeat(" ", (16<<10)+1), `not-json`} {
		if _, _, e := mobileLedgerArguments([]string{"mobile-backup-prepare"}, strings.NewReader(input)); e == nil {
			t.Fatal("invalid JSON budget accepted")
		}
	}
	method, args, e := mobileLedgerArguments([]string{"mobile-backup-cancel", "id", "0"}, strings.NewReader(""))
	if e != nil || method != "cli_cancel_prepared_mobile_backup" || args.(map[string]any)["revision"] != int64(0) {
		t.Fatal("valid revision lost")
	}
}

func TestMobileOfferProbeArgumentsRequireExactRevision(t *testing.T) {
	// Arrange / Act.
	method, args, err := mobileLedgerArguments([]string{"mobile-backup-probe-offer", "id", "0"}, strings.NewReader(""))
	// Assert.
	if err != nil || method != "cli_probe_mobile_backup_offer" || args.(map[string]any)["revision"] != int64(0) {
		t.Fatal(method, args, err)
	}
	for _, revision := range []string{"-1", "01", "+1", "9223372036854775808"} {
		if _, _, err = mobileLedgerArguments([]string{"mobile-backup-probe-offer", "id", revision}, strings.NewReader("")); err == nil {
			t.Fatal("invalid revision accepted")
		}
	}
}

func TestMobileArchiveDiagnosticArguments(t *testing.T) {
	// Arrange/Act: canonical owner attempt reference.
	method, args, err := mobileLedgerArguments([]string{"mobile-backup-probe-archive", "id", "0"}, strings.NewReader(""))
	// Assert: only the private diagnostic route, with revision preserved.
	if err != nil || method != "cli_probe_mobile_backup_archive" || args.(map[string]any)["revision"] != int64(0) || !mobileLedgerCommand("mobile-backup-probe-archive") {
		t.Fatal(method, args, err)
	}
}

func TestArchiveComparisonAndRecallArguments(t *testing.T) {
	// Arrange / Act: carry the saved send UUID through owner-only routes.
	id := "00000000-0000-4000-8000-000000000031"
	method, args, err := mobileLedgerArguments([]string{"mobile-backup-probe-archive", "operation", "0", id}, strings.NewReader(""))
	recallMethod, recallArgs, recallErr := mobileLedgerArguments([]string{"probe-recall", id}, strings.NewReader(""))
	// Assert: exact saved reference, no arbitrary recipient or message arguments.
	if err != nil || method != "cli_probe_mobile_backup_archive" || args.(map[string]any)["comparison_send_request_id"] != id || recallErr != nil || recallMethod != "cli_probe_recall" || recallArgs.(map[string]any)["send_request_id"] != id {
		t.Fatal("owner reference lost")
	}
	for _, invalid := range [][]string{{"probe-recall"}, {"probe-recall", id, "message"}, {"mobile-backup-probe-offer", "operation", "0", id}, {"mobile-backup-cancel", "operation", "0", id}} {
		if _, _, err := mobileLedgerArguments(invalid, strings.NewReader("")); err == nil {
			t.Fatal("unexpected target arguments accepted")
		}
	}
}
