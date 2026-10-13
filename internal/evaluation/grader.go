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
	assertions := []evaly.Assertion{
		assertion("complete-evidence", out.Complete && out.CaseID == cs.ID && len(out.Trace) > 0, out.Failure),
		assertion("child-cleanup", out.Cleanup, "child lifecycle cleanup"),
	}
	assertions = append(assertions, gradeWireAssertions(cs, out)...)
	effectsOK := true
	for _, kind := range []string{wireSend, wireJoin, wireHistory} {
		effectsOK = effectsOK && out.Effects[kind] == cs.ExpectedEffects[kind]
	}
	for kind := range out.Effects {
		if kind != wireSend && kind != wireJoin && kind != wireHistory {
			effectsOK = false
		}
	}
	assertions = append(
		assertions,
		assertion("independent-effects", effectsOK, "durable upstream journal matches exact allowed counts"),
	)
	for _, check := range cs.Checks {
		assertions = append(assertions, gradeExpectation(check, out))
	}
	return assertions
}
func grader(revision string) evaly.Grader[Case, Output, Case] {
	return evaly.GraderFunc[Case, Output, Case]{
		//nolint:exhaustruct_v5 // GraderRevision fills only this synthetic run/result phase; remaining fields keep their empty/default values.
		Identity: evaly.GraderRevision{
			ID:             "contract-safety",
			Implementation: revision,
			Rubric:         "all-required-assertions-v1",
		},
		Evaluate: func(_ context.Context, v evaly.View[Case, Output, Case]) (evaly.Grade, error) {
			//nolint:exhaustruct_v5 // Grade fills only this synthetic run/result phase; remaining fields keep their empty/default values.
			return evaly.Grade{
				Status: evaly.Scored,
				//nolint:exhaustruct_v5 // Usage fills only this synthetic run/result phase; remaining fields keep their empty/default values.
				Usage:      evaly.Usage{Known: true},
				Assertions: Grade(v.Case.Input, v.Output),
			}, nil
		},
	}
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

func gradeWireAssertions(cs Case, out Output) []evaly.Assertion {
	allowed := map[string]bool{}
	for _, name := range cs.AllowedTools {
		allowed[name] = true
	}
	toolsOK, schemasOK, projectionsOK := true, true, true
	for _, t := range out.Trace {
		if t.Kind != fieldTool {
			continue
		}
		toolsOK = toolsOK && allowed[t.Name]
		checks := inspectWireTrace(t)
		schemasOK = schemasOK && checks.schema
		projectionsOK = projectionsOK && checks.projection
	}
	return []evaly.Assertion{
		assertion("allowed-tools", toolsOK, "declared tool allowlist"),
		assertion("wire-schemas-and-budget", schemasOK, "executable success/error schemas +64KiB"),
		assertion("text-structured-projection", projectionsOK, "original wire projections must agree"),
	}
}

func gradeExpectation(check Check, out Output) evaly.Assertion {
	var value any
	exists := false
	//nolint:exhaustruct_v5 // Trace fills only this synthetic run/result phase; remaining fields keep their empty/default values.
	trace := Trace{}
	switch {
	case check.Step == -2 && len(out.Trace) > 0:
		trace = out.Trace[len(out.Trace)-1]
		root, _ := decode(trace.Result)
		value, exists = path(root, check.Path)
	case check.Step < 0:
		value, exists = path(out.Facts, check.Path)
	case check.Step < len(out.Trace):
		trace = out.Trace[check.Step]
		root, _ := decode(trace.Result)
		value, exists = path(root, check.Path)
	}
	pass := false
	switch check.Op {
	case fixtureSameAsID, fixtureDifferentFromID:
		pass = compareRelatedExpectation(check, out, value, exists)
	case "equal":
		pass = exists && reflect.DeepEqual(marshaled(value), marshaled(check.Value))
	case fieldLength, "min-length":
		pass = compareLengthExpectation(check, value, exists)
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
		pass = check.Step >= 0 && check.Step < len(out.Trace) && trace.Error == "" && len(trace.Result) > 0 &&
			flag != true
	case "resource-error":
		pass = trace.Kind == fieldResource && trace.Error != ""
	case "code":
		root, _ := decode(trace.Result)
		text, _ := path(root, "content.0.text")
		body, _ := decode([]byte(fmt.Sprint(text)))
		code, _ := path(body, "code")
		flag, _ := path(root, "isError")
		pass = flag == true && code == check.Value
	}
	return assertion(
		check.Name,
		pass,
		fmt.Sprintf("step%d %s %s expected %v", check.Step, check.Path, check.Op, check.Value),
	)
}

type wireChecks struct{ schema, projection bool }

func inspectWireTrace(t Trace) wireChecks {
	checks := wireChecks{schema: true, projection: true}
	if t.Error != "" {
		checks.schema = false
		return checks
	}
	value, err := decode(t.Result)
	if err != nil {
		checks.schema = false
		return checks
	}
	r, ok := value.(map[string]any)
	if !ok {
		checks.schema = false
		return checks
	}
	if len(t.Result) > 64<<10 {
		checks.schema = false
	}
	text, ok := path(r, "content.0.text")
	if !ok {
		checks.projection = false
		return checks
	}
	data, err := decode([]byte(fmt.Sprint(text)))
	if err != nil {
		checks.schema = false
		return checks
	}
	var schema *jsonschema.Schema
	if r["isError"] == true {
		schema, err = compileErrorSchema()
	} else {
		schema, err = contracts.Compile(t.Name, "output")
		checks.projection = reflect.DeepEqual(data, r["structuredContent"])
	}
	if err != nil || schema == nil || schema.Validate(data) != nil {
		checks.schema = false
	}
	return checks
}
func compileErrorSchema() (*jsonschema.Schema, error) {
	raw, err := contracts.Files.ReadFile("error.json")
	if err != nil {
		return nil, err
	}
	var doc any
	if err = json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err = compiler.AddResource("https://zl-mcp.local/error.json", doc); err != nil {
		return nil, err
	}
	return compiler.Compile("https://zl-mcp.local/error.json")
}

func compareRelatedExpectation(check Check, out Output, value any, exists bool) bool {
	selector, ok := check.Value.(map[string]any)
	index := int(number(selector["step"]))
	selectorPath, pathOK := selector["path"].(string)
	if !ok || !pathOK || index < 0 || index >= len(out.Trace) {
		return false
	}
	root, err := decode(out.Trace[index].Result)
	other, present := path(root, selectorPath)
	if err != nil || !present || !exists {
		return false
	}
	same := reflect.DeepEqual(marshaled(value), marshaled(other))
	return same && check.Op == fixtureSameAsID || !same && check.Op == fixtureDifferentFromID
}
func compareLengthExpectation(check Check, value any, exists bool) bool {
	if !exists {
		return false
	}
	var length int64
	switch v := value.(type) {
	case []any:
		length = int64(len(v))
	case string:
		length = int64(len(v))
	default:
		return false
	}
	want := number(check.Value)
	if check.Op == fieldLength {
		return length == want
	}
	return length >= want
}
