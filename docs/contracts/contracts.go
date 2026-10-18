// Package contracts embeds the executable MCP contracts shipped with the server.
package contracts

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

//go:embed *.json
var Files embed.FS

// Shared history schemas stay flat for MCP clients while avoiding three copies
// of the same operation status contract.
//
//nolint:gochecknoglobals // Closed schema aliases are read-only after package initialization.
var toolAliases = map[string][2]string{
	"zalo_import_conversation_history": {"history_import_request", historyImportOperationSchema},
	"zalo_get_history_import_status":   {historyImportOperationSchema, historyImportOperationSchema},
	"zalo_cancel_history_import":       {historyImportOperationSchema, historyImportOperationSchema},
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
		switch suffix {
		case "input":
			name = alias[0]
		case "output":
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

type compiledContract struct {
	once   sync.Once
	schema *jsonschema.Schema
	err    error
}

//nolint:gochecknoglobals // Bounded by existing immutable embedded contract files; entries compile once and validators are read-only.
var compiledContracts sync.Map

// Compile returns an immutable validator shared by consumers. Document continues
// to return caller-owned maps. Keep every validation boundary; avoid compiling
// the same embedded contract again for transport, runtime and owner on each call.
func Compile(name, suffix string) (*jsonschema.Schema, error) {
	resolved := name
	if alias, ok := toolAliases[name]; ok {
		switch suffix {
		case "input":
			resolved = alias[0]
		case "output":
			resolved = alias[1]
		}
	}
	// Do not cache arbitrary caller-controlled names or missing contracts.
	if _, err := Files.ReadFile(resolved + "." + suffix + ".json"); err != nil {
		return nil, err
	}
	value, _ := compiledContracts.LoadOrStore(name+"."+suffix, new(compiledContract))
	entry, ok := value.(*compiledContract)
	if !ok {
		return nil, errors.New("invalid compiled contract entry")
	}
	entry.once.Do(func() { entry.schema, entry.err = compileContract(name, suffix) })
	return entry.schema, entry.err
}

func compileContract(name, suffix string) (*jsonschema.Schema, error) {
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
