// Package evaluation is an offline host harness, never a production dependency.
package evaluation

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

type Check struct {
	Name  string `json:"name"`
	Step  int    `json:"step"`
	Path  string `json:"path"`
	Op    string `json:"op"`
	Value any    `json:"value,omitempty"`
}
type Case struct {
	ID              string         `json:"id"`
	TimeoutSeconds  int            `json:"timeout_seconds"`
	AllowedTools    []string       `json:"allowed_tools"`
	ExpectedEffects map[string]int `json:"expected_effects"`
	Checks          []Check        `json:"checks"`
}
type Dataset struct {
	Revision       string `json:"revision"`
	GraderRevision string `json:"grader_revision"`
	Cases          []Case `json:"cases"`
}
type Trace struct {
	Kind   string          `json:"kind"`
	Name   string          `json:"name"`
	Input  json.RawMessage `json:"input,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}
type Output struct {
	CaseID         string         `json:"case_id"`
	Trace          []Trace        `json:"trace"`
	Facts          map[string]any `json:"facts"`
	Cleanup        bool           `json:"cleanup"`
	Effects        map[string]int `json:"effects"`
	Complete       bool           `json:"complete"`
	Failure        string         `json:"failure,omitempty"`
	ElapsedSeconds float64        `json:"elapsed_seconds"`
	Bytes          int            `json:"bytes"`
}

func decode(raw []byte) (any, error) {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var value any
	if err := d.Decode(&value); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("trailing JSON content")
	}
	return value, nil
}
func path(value any, selector string) (any, bool) {
	for part := range strings.SplitSeq(selector, ".") {
		if part == "" {
			continue
		}
		switch v := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = v[part]
			if !ok {
				return nil, false
			}
		case []any:
			index, err := strconv.Atoi(part)
			if err != nil || index < 0 || index >= len(v) {
				return nil, false
			}
			value = v[index]
		default:
			return nil, false
		}
	}
	return value, true
}
func marshaled(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("synthetic encoding: %v", err))
	}
	return b
}
