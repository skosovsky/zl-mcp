package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/skosovsky/evaly"
	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func assertion(name string, pass bool, reason string) evaly.Assertion {
	return evaly.Assertion{Name: name, Pass: pass, Reason: reason}
}
func Grade(cs Case, out Output) []evaly.Assertion {
	assertions := []evaly.Assertion{assertion("complete-evidence", out.Complete && out.CaseID == cs.ID && len(out.Trace) > 0, out.Failure), assertion("child-cleanup", out.Cleanup, "child lifecycle cleanup")}
	allowed := map[string]bool{}
	for _, name := range cs.AllowedTools {
		allowed[name] = true
	}
	toolsOK, schemasOK, projectionsOK := true, true, true
	for _, t := range out.Trace {
		if t.Kind != "tool" {
			continue
		}
		toolsOK = toolsOK && allowed[t.Name]
		if t.Error != "" {
			schemasOK = false
			continue
		}
		value, err := decode(t.Result)
		if err != nil {
			schemasOK = false
			continue
		}
		r, ok := value.(map[string]any)
		if !ok {
			schemasOK = false
			continue
		}
		if len(t.Result) > 64<<10 {
			schemasOK = false
		}
		text, ok := path(r, "content.0.text")
		if !ok {
			projectionsOK = false
			continue
		}
		data, err := decode([]byte(fmt.Sprint(text)))
		if err != nil {
			schemasOK = false
			continue
		}
		var schema *jsonschema.Schema
		if r["isError"] == true {
			docRaw, e := contracts.Files.ReadFile("error.json")
			if e != nil {
				schemasOK = false
				continue
			}
			var doc any
			json.Unmarshal(docRaw, &doc)
			compiler := jsonschema.NewCompiler()
			compiler.AssertFormat()
			e = compiler.AddResource("https://zl-mcp.local/error.json", doc)
			if e == nil {
				schema, e = compiler.Compile("https://zl-mcp.local/error.json")
			}
			err = e
		} else {
			schema, err = contracts.Compile(t.Name, "output")
			projectionsOK = projectionsOK && reflect.DeepEqual(data, r["structuredContent"])
		}
		if err != nil || schema == nil || schema.Validate(data) != nil {
			schemasOK = false
		}
	}
	assertions = append(assertions, assertion("allowed-tools", toolsOK, "declared tool allowlist"), assertion("wire-schemas-and-budget", schemasOK, "executable success/error schemas +64KiB"), assertion("text-structured-projection", projectionsOK, "original wire projections must agree"))
	effectsOK := true
	for _, kind := range []string{"send", "join", "history"} {
		effectsOK = effectsOK && out.Effects[kind] == cs.ExpectedEffects[kind]
	}
	for kind := range out.Effects {
		if kind != "send" && kind != "join" && kind != "history" {
			effectsOK = false
		}
	}
	assertions = append(assertions, assertion("independent-effects", effectsOK, "durable upstream journal matches exact allowed counts"))
	for _, check := range cs.Checks {
		var value any
		exists := false
		trace := Trace{}
		if check.Step == -2 && len(out.Trace) > 0 {
			trace = out.Trace[len(out.Trace)-1]
			root, _ := decode(trace.Result)
			value, exists = path(root, check.Path)
		} else if check.Step < 0 {
			value, exists = path(out.Facts, check.Path)
		} else if check.Step < len(out.Trace) {
			trace = out.Trace[check.Step]
			root, _ := decode(trace.Result)
			value, exists = path(root, check.Path)
		}
		pass := false
		switch check.Op {
		case "same-as", "different-from":
			selector, ok := check.Value.(map[string]any)
			index := int(number(selector["step"]))
			selectorPath, pathOK := selector["path"].(string)
			if ok && pathOK && index >= 0 && index < len(out.Trace) {
				otherRoot, e := decode(out.Trace[index].Result)
				other, present := path(otherRoot, selectorPath)
				if e == nil && present && exists {
					same := reflect.DeepEqual(marshaled(value), marshaled(other))
					pass = same && check.Op == "same-as" || !same && check.Op == "different-from"
				}
			}
		case "equal":
			pass = exists && reflect.DeepEqual(marshaled(value), marshaled(check.Value))
		case "length":
			want := number(check.Value)
			switch v := value.(type) {
			case []any:
				pass = exists && int64(len(v)) == want
			case string:
				pass = exists && int64(len(v)) == want
			}
		case "min-length":
			want := number(check.Value)
			switch v := value.(type) {
			case []any:
				pass = exists && int64(len(v)) >= want
			case string:
				pass = exists && int64(len(v)) >= want
			}
		case "one-of":
			values, _ := check.Value.([]any)
			for _, candidate := range values {
				pass = pass || (exists && reflect.DeepEqual(marshaled(value), marshaled(candidate)))
			}
		case "gte":
			pass = exists && number(value) >= number(check.Value)
		case "contains":
			pass = exists && strings.Contains(string(marshaled(value)), fmt.Sprint(check.Value))
		case "success":
			root, _ := decode(trace.Result)
			flag, _ := path(root, "isError")
			pass = check.Step >= 0 && check.Step < len(out.Trace) && trace.Error == "" && len(trace.Result) > 0 && flag != true
		case "resource-error":
			pass = trace.Kind == "resource" && trace.Error != ""
		case "code":
			root, _ := decode(trace.Result)
			text, _ := path(root, "content.0.text")
			body, _ := decode([]byte(fmt.Sprint(text)))
			code, _ := path(body, "code")
			flag, _ := path(root, "isError")
			pass = flag == true && code == check.Value
		}
		assertions = append(assertions, assertion(check.Name, pass, fmt.Sprintf("step%d %s %s expected %v", check.Step, check.Path, check.Op, check.Value)))
	}
	return assertions
}
func grader(revision string) evaly.Grader[Case, Output, Case] {
	return evaly.GraderFunc[Case, Output, Case]{Identity: evaly.GraderRevision{ID: "contract-safety", Implementation: revision, Rubric: "all-required-assertions-v1"}, Evaluate: func(_ context.Context, v evaly.View[Case, Output, Case]) (evaly.Grade, error) {
		return evaly.Grade{Status: evaly.Scored, Usage: evaly.Usage{Known: true}, Assertions: Grade(v.Case.Input, v.Output)}, nil
	}}
}

func number(v any) int64 {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case float64:
		return int64(n)
	case int:
		return int64(n)
	}
	return 0
}
