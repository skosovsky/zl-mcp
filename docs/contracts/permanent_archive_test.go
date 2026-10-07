package contracts

import "testing"

func TestPermanentArchiveRequestRequiresExplicitOwnerRetention(t *testing.T) {
	// Arrange: preservation uses the existing source; no acquisition parameters.
	schema, err := Compile("cli_preserve_account_archive", "input")
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{"method": "cli_preserve_account_archive", "arguments": map[string]any{"source_id": "00000000-0000-4000-8000-000000000001", "retention": "until_owner_deletion"}}
	// Act / Assert: explicit preservation is valid, implicit retention is not.
	if err := schema.Validate(valid); err != nil {
		t.Fatal(err)
	}
	for _, args := range []map[string]any{
		{"source_id": "00000000-0000-4000-8000-000000000001"},
		{"source_id": "bad", "retention": "until_owner_deletion"},
		{"source_id": "00000000-0000-4000-8000-000000000001", "retention": "seven_days"},
		{"source_id": "00000000-0000-4000-8000-000000000001", "retention": "until_owner_deletion", "download": true},
	} {
		if schema.Validate(map[string]any{"method": "cli_preserve_account_archive", "arguments": args}) == nil {
			t.Fatal("ambiguous or acquisition request accepted")
		}
	}
	if _, err := Compile("cli_preserve_account_archive", "output"); err != nil {
		t.Fatal(err)
	}
	for _, suffix := range []string{"input", "output"} {
		if _, err := Compile("cli_restore_account_archive", suffix); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range Names() {
		if name == "cli_preserve_account_archive" {
			t.Fatal("owner operation exposed as MCP tool")
		}
	}
}
