package contracts

import (
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestDeferredArchiveMetadataUsesBoundedOwnerDiagnosticContract(t *testing.T) {
	// Arrange: compile the actual embedded property, including its local reference.
	doc, err := Document("cli_inspect_account_archive", "output")
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	const location = "https://zl-mcp.local/cli_inspect_account_archive.output.json"
	if err := compiler.AddResource(location, doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location + "#/properties/files/items/properties/sample_deferred_metadata_diagnostics")
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := map[string]any{"rows_with_metadata": 1, "missing_metadata": 0, "invalid_metadata": 0, "attachment_count": 1, "unsupported_metadata_fields": 0, "source_text_present": 1, "title_present": 0, "title_equals_source_text": 0, "action_classes": map[string]any{"other": 1}, "action_shapes": []any{}, "action_shapes_truncated": false, "text_projection_classes": map[string]any{"unsupported": 1}}
	// Act / Assert: only pinned format types and fixed diagnostic fields are accepted.
	if err := schema.Validate(map[string]any{"20": diagnostic}); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(map[string]any{"999": diagnostic}); err == nil {
		t.Fatal("unknown type accepted")
	}
	diagnostic["text"] = "PRIVATE-SOURCE-BODY"
	if err := schema.Validate(map[string]any{"20": diagnostic}); err == nil {
		t.Fatal("raw source body accepted")
	}
}
