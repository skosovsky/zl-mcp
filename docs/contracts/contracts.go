// Package contracts embeds the executable MCP contracts shipped with the server.
package contracts

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed *.json
var Files embed.FS

// Shared history schemas stay flat for MCP clients while avoiding three copies
// of the same operation status contract.
var toolAliases = map[string][2]string{
	"zalo_import_conversation_history": {"history_import_request", "history_import_operation"},
	"zalo_get_history_import_status":   {"history_import_operation", "history_import_operation"},
	"zalo_cancel_history_import":       {"history_import_operation", "history_import_operation"},
}

func Names() []string {
	entries, _ := Files.ReadDir(".")
	names := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".input.json") && strings.HasPrefix(e.Name(), "zalo_") {
			names = append(names, strings.TrimSuffix(e.Name(), ".input.json"))
		}
	}
	for name := range toolAliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func Document(name, suffix string) (map[string]any, error) {
	if alias, ok := toolAliases[name]; ok {
		if suffix == "input" {
			name = alias[0]
		} else if suffix == "output" {
			name = alias[1]
		}
	}
	b, e := Files.ReadFile(name + "." + suffix + ".json")
	if e != nil {
		return nil, e
	}
	var doc map[string]any
	e = json.Unmarshal(b, &doc)
	return doc, e
}
func Compile(name, suffix string) (*jsonschema.Schema, error) {
	doc, e := Document(name, suffix)
	if e != nil {
		return nil, e
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	loc := "https://zl-mcp.local/" + name + "." + suffix + ".json"
	if e = c.AddResource(loc, doc); e != nil {
		return nil, e
	}
	schema, e := c.Compile(loc)
	if e != nil {
		return nil, fmt.Errorf("%s: %w", name, e)
	}
	return schema, nil
}
