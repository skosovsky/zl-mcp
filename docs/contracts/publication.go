package contracts

import (
	"net/url"
	"strconv"
	"strings"
)

const definitionsKey = "$defs"

// PublishedDocument returns a caller-owned projection for tools/list. Validation
// continues to use the full embedded contract. Only unreachable root definitions
// are removed; unsupported reference scopes retain the original document.
func PublishedDocument(name, suffix string) (map[string]any, error) {
	doc, err := Document(name, suffix)
	if err != nil {
		return nil, err
	}
	pruneDefinitions(doc)
	return doc, nil
}

type definitionGraph struct {
	document map[string]any
	keep     map[string]bool
	visited  map[string]bool
}

// pruneDefinitions follows only the reachable graph. Unknown reference scopes
// retain the original definitions rather than change validation semantics.
func pruneDefinitions(doc map[string]any) {
	defs, ok := doc[definitionsKey].(map[string]any)
	if !ok {
		return
	}
	graph := definitionGraph{document: doc, keep: make(map[string]bool), visited: make(map[string]bool)}
	if !graph.walk(doc, true) {
		return
	}
	for name := range defs {
		if !graph.keep[name] {
			delete(defs, name)
		}
	}
	if len(defs) == 0 {
		delete(doc, definitionsKey)
	}
}

func (graph *definitionGraph) walk(value any, root bool) bool {
	switch node := value.(type) {
	case map[string]any:
		return graph.walkMap(node, root)
	case []any:
		for _, child := range node {
			if !graph.walk(child, false) {
				return false
			}
		}
	}
	return true
}

func (graph *definitionGraph) walkMap(node map[string]any, root bool) bool {
	for key, child := range node {
		switch key {
		case definitionsKey:
			if !root {
				return false
			}
			continue
		case "$id":
			if !root {
				return false
			}
		case "$dynamicRef", "$recursiveRef", "$anchor", "$dynamicAnchor":
			return false
		case "$ref":
			ref, ok := child.(string)
			if !ok || !graph.reference(ref) {
				return false
			}
		}
		if !graph.walk(child, false) {
			return false
		}
	}
	return true
}

func (graph *definitionGraph) reference(ref string) bool {
	target, tokens, ok := resolveLocalPointer(graph.document, ref)
	if !ok || len(tokens) == 1 && tokens[0] == definitionsKey {
		return false
	}
	if len(tokens) >= 2 && tokens[0] == definitionsKey {
		graph.keep[tokens[1]] = true
	}
	if graph.visited[ref] {
		return true
	}
	graph.visited[ref] = true
	return graph.walk(target, len(tokens) == 0)
}

func resolveLocalPointer(doc map[string]any, ref string) (any, []string, bool) {
	if !strings.HasPrefix(ref, "#") {
		return nil, nil, false
	}
	fragment, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
	if err != nil {
		return nil, nil, false
	}
	if fragment == "" {
		return doc, nil, true
	}
	if !strings.HasPrefix(fragment, "/") {
		return nil, nil, false
	}
	parts := strings.Split(fragment[1:], "/")
	var node any = doc
	for index, part := range parts {
		decoded, ok := decodePointerToken(part)
		if !ok {
			return nil, nil, false
		}
		parts[index] = decoded
		node, ok = pointerChild(node, decoded)
		if !ok {
			return nil, nil, false
		}
	}
	return node, parts, true
}

func decodePointerToken(part string) (string, bool) {
	for index := 0; index < len(part); index++ {
		if part[index] != '~' {
			continue
		}
		if index+1 >= len(part) || (part[index+1] != '0' && part[index+1] != '1') {
			return "", false
		}
		index++
	}
	return strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~"), true
}

func pointerChild(node any, part string) (any, bool) {
	switch value := node.(type) {
	case map[string]any:
		child, ok := value[part]
		return child, ok
	case []any:
		index, err := strconv.Atoi(part)
		if err != nil || strconv.Itoa(index) != part || index < 0 || index >= len(value) {
			return nil, false
		}
		return value[index], true
	default:
		return nil, false
	}
}
