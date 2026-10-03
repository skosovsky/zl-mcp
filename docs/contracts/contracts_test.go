package contracts

import "testing"

func TestAllSchemasCompile(t *testing.T) {
	// Arrange
	names := Names()
	if len(names) != 12 {
		t.Fatalf("expected twelve tools, got %d", len(names))
	}
	for _, name := range names {
		for _, suffix := range []string{"input", "output"} {
			t.Run(name+"."+suffix, func(t *testing.T) {
				// Act
				_, err := Compile(name, suffix)
				// Assert
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
func TestInputsRejectAmbiguousAndUnknownArguments(t *testing.T) {
	// Arrange
	cases := []struct {
		name string
		args map[string]any
	}{
		{"zalo_get_status", map[string]any{"credentials": "secret"}},
		{"zalo_search_messages", map[string]any{"query": "repair", "limit": "10"}},
		{"zalo_search_messages", map[string]any{"query": "repair", "limit": 51}},
		{"zalo_search_messages", map[string]any{"query": "repair", "since": "01/02/2026"}},
		{"zalo_join_group", map[string]any{"invite_url": "https://zalo.me/g/x", "request_id": "not-uuid"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			schema, err := Compile(tc.name, "input")
			if err != nil {
				t.Fatal(err)
			}
			// Act
			err = schema.Validate(tc.args)
			// Assert
			if err == nil {
				t.Fatal("invalid arguments accepted")
			}
		})
	}
}

func TestControlContractsCompile(t *testing.T) {
	// Arrange
	names := []struct{ name, suffix string }{{"control", "input"}, {"control", "output"}, {"cli_preview", "input"}, {"cli_preview", "output"}, {"cli_approve", "input"}, {"cli_approve", "output"}}
	for _, v := range names {
		t.Run(v.name+v.suffix, func(t *testing.T) {
			// Act
			_, err := Compile(v.name, v.suffix)
			// Assert
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}
