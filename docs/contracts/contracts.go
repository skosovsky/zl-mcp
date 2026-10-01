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

func Names() []string {
	entries, _ := Files.ReadDir(".")
	names := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".input.json") && strings.HasPrefix(e.Name(), "zalo_") {
			names = append(names, strings.TrimSuffix(e.Name(), ".input.json"))
		}
	}
	sort.Strings(names)
	return names
}
func Document(name, suffix string) (map[string]any, error) {
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
