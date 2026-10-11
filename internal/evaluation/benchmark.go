package evaluation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/skosovsky/zl-mcp/internal/evalfixture"
)

type Measurement struct {
	Kind             string  `json:"kind"`
	Calls            int     `json:"calls"`
	Warmup           int     `json:"warmup"`
	Errors           int     `json:"errors"`
	Bytes            int     `json:"bytes"`
	P95Seconds       float64 `json:"p95_seconds"`
	AllocationsPerOp float64 `json:"allocations_per_op"`
	AllocBytesPerOp  float64 `json:"alloc_bytes_per_op"`
	ElapsedSeconds   float64 `json:"elapsed_seconds"`
	Go               string  `json:"go"`
	OS               string  `json:"os"`
	Arch             string  `json:"arch"`
	Concurrency      int     `json:"concurrency"`
	PacingMillis     int     `json:"pacing_millis"`
	Admission        bool    `json:"admission_enabled"`
	Exporters        bool    `json:"exporters_enabled"`
	Cleanup          bool    `json:"cleanup"`
}

// Measure times complete real SDK calls; admission pacing is outside each sample.
func Measure(ctx context.Context, root, state, kind string, count int) (result Measurement, err error) {
	result = Measurement{Kind: kind, Warmup: 50, Go: runtime.Version(), OS: runtime.GOOS, Arch: runtime.GOARCH, Concurrency: 1, PacingMillis: 110, Admission: true, Exporters: false}
	if count < 1000 {
		return result, fmt.Errorf("at least1000 measurements required")
	}
	name := ""
	args := map[string]any{}
	switch kind {
	case "read":
		name = "zalo_list_groups"
	case "search":
		name = "zalo_search_messages"
		args["query"] = "ремонт"
	case "context":
		name = "zalo_get_message_context"
		args["group_id"] = "g-1"
		args["message_id"] = "m-101"
		args["before"] = 0
		args["after"] = 0
	default:
		return result, fmt.Errorf("unknown measurement kind")
	}
	effects := filepath.Join(state, "effects.jsonl")
	if err = os.WriteFile(effects, nil, 0600); err != nil {
		return result, err
	}
	f, err := evalfixture.New(ctx, evalfixture.Options{Offline: true, StateDir: filepath.Join(state, "state"), EffectsFile: effects, Fixtures: filepath.Join(root, "docs/evals/fixtures.json")})
	if err != nil {
		return result, err
	}
	c, err := connect(ctx, f)
	if err != nil {
		f.Close()
		return result, err
	}
	defer func() {
		c.close()
		e := f.Close()
		result.Cleanup = e == nil
		if err == nil {
			err = e
		}
	}()
	samples := make([]float64, count)
	var allocations, allocatedBytes uint64
	start := time.Now()
	for i := -result.Warmup; i < count; i++ {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(110 * time.Millisecond):
		}
		var before, after runtime.MemStats
		runtime.ReadMemStats(&before)
		began := time.Now()
		response, e := c.session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		wire := marshaled(response)
		duration := time.Since(began).Seconds()
		runtime.ReadMemStats(&after)
		if e != nil || response == nil || response.IsError {
			result.Errors++
			return result, fmt.Errorf("benchmark MCP failure: %v", e)
		}
		if i >= 0 {
			result.Calls++
			samples[i] = duration
			result.Bytes += len(wire)
			allocations += after.Mallocs - before.Mallocs
			allocatedBytes += after.TotalAlloc - before.TotalAlloc
		}
	}
	sort.Float64s(samples)
	result.P95Seconds = samples[(95*count+99)/100-1]
	result.AllocationsPerOp = float64(allocations) / float64(count)
	result.AllocBytesPerOp = float64(allocatedBytes) / float64(count)
	result.ElapsedSeconds = time.Since(start).Seconds()
	return result, nil
}
