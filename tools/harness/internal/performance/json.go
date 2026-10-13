package performance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Object = map[string]any

func measurementKinds() []string { return []string{"read", "search", "context"} }

const MeasurementRevision = "2026-10-12-objects-and-bytes-v2"
const AcceptanceRevision = "2026-10-12-candidate-latency-plus10-v3"

const (
	readP95LimitSeconds    = .0078683
	searchP95LimitSeconds  = .0070726337
	contextP95LimitSeconds = .0069967799
)

func p95Limits() map[string]float64 {
	return map[string]float64{
		"read":    readP95LimitSeconds,
		"search":  searchP95LimitSeconds,
		"context": contextP95LimitSeconds,
	}
}

func digest(b []byte) string              { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func fileDigest(p string) (string, error) { b, e := os.ReadFile(p); return digest(b), e }
func Decode(b []byte) (Object, error) {
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m Object
	if e := d.Decode(&m); e != nil {
		return nil, e
	}
	if m == nil {
		return nil, errors.New("expected JSON object")
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return nil, errors.New("trailing JSON data")
	}
	return m, nil
}
func readObject(p string) (Object, []byte, error) {
	b, e := os.ReadFile(p)
	if e != nil {
		return nil, nil, e
	}
	m, e := Decode(b)
	return m, b, e
}
func Write(p string, m Object) error {
	b, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	//nolint:gosec // G301: this directory holds shareable synthetic evidence/executables, never account state.
	if e = os.MkdirAll(filepath.Dir(p), 0755); e != nil {
		return e
	}
	//nolint:gosec // G306: this synthetic evidence contains build/host provenance, never account credentials.
	return os.WriteFile(p, append(b, '\n'), 0644)
}
func object(v any) Object { m, _ := v.(map[string]any); return m }
func array(v any) []any   { a, _ := v.([]any); return a }
func str(v any) string    { s, _ := v.(string); return s }
func number(v any) (float64, bool) {
	var f float64
	var e error
	switch n := v.(type) {
	case json.Number:
		f, e = strconv.ParseFloat(string(n), 64)
	case float64:
		f = n
	case int:
		f = float64(n)
	default:
		return 0, false
	}
	return f, e == nil && !math.IsNaN(f) && !math.IsInf(f, 0) && f >= 0
}
func integer(v any) (int64, bool) {
	switch n := v.(type) {
	case json.Number:
		i, e := strconv.ParseInt(string(n), 10, 64)
		return i, e == nil && i >= 0
	case int:
		return int64(n), n >= 0
	default:
		return 0, false
	}
}
func value(v any) float64 { f, _ := number(v); return f }
func validDigest(v any) bool {
	s := str(v)
	if len(s) != sha256.Size*2 {
		return false
	}
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
func equal(a, b any) bool {
	if x, ok := a.(bool); ok {
		y, ok := b.(bool)
		return ok && x == y
	}
	if x, ok := a.(string); ok {
		y, ok := b.(string)
		return ok && x == y
	}
	if x, ok := integer(a); ok {
		y, ok := integer(b)
		return ok && x == y
	}
	return false
}
func median(a []float64) float64 {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j] < a[j-1]; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
	return a[len(a)/2]
}
func nonempty(v any) bool { return strings.TrimSpace(str(v)) != "" }
