package contracts

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func compilePublication(t *testing.T, doc map[string]any) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	const location = "https://zl-mcp.local/publication-test.json"
	if err := compiler.AddResource(location, doc); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(location)
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func TestPublishedDefinitionsTransitiveCyclesAndEscapedPointers(t *testing.T) {
	// Arrange: composition, mutually recursive schemas, and both JSON Pointer escapes.
	raw := `{"type":"object","properties":{"value":{"$ref":"#/$defs/a~1b~0c"}},"required":["value"],"$defs":{"a/b~c":{"anyOf":[{"type":"integer"},{"$ref":"#/$defs/list"}]},"list":{"type":"array","items":{"$ref":"#/$defs/a~1b~0c"}},"unreachable":{"type":"boolean"}}}`
	var original, publication map[string]any
	if err := json.Unmarshal([]byte(raw), &original); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(raw), &publication); err != nil {
		t.Fatal(err)
	}
	before := compilePublication(t, original)
	// Act
	pruneDefinitions(publication)
	after := compilePublication(t, publication)
	// Assert
	defs, ok := publication["$defs"].(map[string]any)
	if !ok || len(defs) != 2 || defs["unreachable"] != nil {
		t.Fatalf("wrong closure: %+v", publication)
	}
	for _, value := range []any{map[string]any{"value": 1}, map[string]any{"value": []any{1, []any{2}}}, map[string]any{"value": true}, map[string]any{"value": "wrong"}, map[string]any{}} {
		if (before.Validate(value) == nil) != (after.Validate(value) == nil) {
			t.Fatalf("semantics changed for %#v", value)
		}
	}
}

func TestPublishedPointerSubtreeAndUnsupportedScopes(t *testing.T) {
	cases := []struct {
		name, raw string
		changed   bool
	}{
		{
			"subtree",
			`{"$ref":"#/$defs/a/properties/x","$defs":{"a":{"properties":{"x":{"type":"string"}}},"unused":{}}}`,
			true,
		},
		{"percent-escape", `{"$ref":"#/$defs/a%20b","$defs":{"a b":{"type":"string"},"unused":{}}}`, true},
		{"array-pointer", `{"$ref":"#/$defs/a/anyOf/0","$defs":{"a":{"anyOf":[{"type":"string"}]},"unused":{}}}`, true},
		{"anchor", `{"$ref":"#named","$defs":{"a":{"$anchor":"named","type":"string"},"unused":{}}}`, false},
		{"external", `{"$ref":"https://example.invalid/schema","$defs":{"unused":{}}}`, false},
		{"dynamic", `{"$dynamicRef":"#node","$defs":{"unused":{}}}`, false},
		{"nested-id", `{"properties":{"x":{"$id":"child","$ref":"#/$defs/x"}},"$defs":{"unused":{}}}`, false},
		{"bad-escape", `{"$ref":"#/$defs/a~2b","$defs":{"unused":{}}}`, false},
		{"bad-index", `{"$ref":"#/$defs/a/anyOf/00","$defs":{"a":{"anyOf":[true]},"unused":{}}}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var doc map[string]any
			if err := json.Unmarshal([]byte(tc.raw), &doc); err != nil {
				t.Fatal(err)
			}
			before, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			// Act
			pruneDefinitions(doc)
			after, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			// Assert
			if (string(before) != string(after)) != tc.changed {
				t.Fatalf("unexpected projection: %s", after)
			}
		})
	}
}

//nolint:gocognit // AAA compares original/published validators and caller ownership for every executable contract.
func TestPublishedCatalogCompilesShrinksAndPreservesValidation(t *testing.T) {
	// Arrange
	beforeBytes, afterBytes := 0, 0
	for _, name := range Names() {
		for _, suffix := range []string{"input", "output"} {
			t.Run(name+"."+suffix, func(t *testing.T) {
				original, err := Document(name, suffix)
				if err != nil {
					t.Fatal(err)
				}
				// Act
				publication, err := PublishedDocument(name, suffix)
				if err != nil {
					t.Fatal(err)
				}
				before := compilePublication(t, original)
				after := compilePublication(t, publication)
				// Assert: diverse candidates keep validation decisions identical for all contracts.
				candidates := []any{
					nil,
					true,
					"text",
					float64(0),
					[]any{},
					map[string]any{},
					map[string]any{"unknown": true},
				}
				for _, candidate := range candidates {
					if (before.Validate(candidate) == nil) != (after.Validate(candidate) == nil) {
						t.Fatalf("validation changed: %#v", candidate)
					}
				}
				a, err := json.Marshal(original)
				if err != nil {
					t.Fatal(err)
				}
				b, err := json.Marshal(publication)
				if err != nil {
					t.Fatal(err)
				}
				beforeBytes += len(a)
				afterBytes += len(b)
				fresh, err := Document(name, suffix)
				if err != nil || !reflect.DeepEqual(original, fresh) {
					t.Fatal("publication mutated embedded document")
				}
				twice, err := PublishedDocument(name, suffix)
				if err != nil || !reflect.DeepEqual(publication, twice) {
					t.Fatal("publication not deterministic")
				}
				// Every retained root definition must be reachable: a second projection is idempotent.
				pruneDefinitions(twice)
				if !reflect.DeepEqual(publication, twice) {
					t.Fatal("not a closed reachable projection")
				}
			})
		}
	}
	if afterBytes >= beforeBytes {
		t.Fatalf("catalog did not shrink: %d -> %d", beforeBytes, afterBytes)
	}
	t.Logf("published schema bytes %d -> %d (removed %d)", beforeBytes, afterBytes, beforeBytes-afterBytes)
}

func TestLocalPointerDoesNotAcceptPartialNumericOrMalformedFragments(t *testing.T) {
	// Arrange
	doc := map[string]any{"items": []any{true}}
	for _, ref := range []string{"#/items/0junk", "#/items/-1", "#/items/01", "#/%zz", "#/unknown", "#anchor"} {
		// Act
		_, _, ok := resolveLocalPointer(doc, ref)
		// Assert
		if ok {
			t.Fatalf("invalid pointer accepted: %s", ref)
		}
	}
	_, _, ok := resolveLocalPointer(doc, "#/items/0")
	if !ok {
		t.Fatal("valid pointer rejected")
	}
}
