// Synthetic worker for process-boundary acceptance; no service/model/account access.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

func main() {
	if filepath.Base(os.Args[0]) == "uname" {
		time.Sleep(time.Minute)
		return
	}
	root := flag.String("root", "", "root")
	kind := flag.String("measure-kind", "read", "kind")
	flag.Int("measure-calls", 1000, "calls")
	flag.String("state-dir", "", "state")
	pid := flag.String("pidfile", "", "pid")
	flag.Parse()
	_ = root
	mode := filepath.Base(os.Args[0])
	switch mode {
	case "partial":
		fmt.Print("{")
		time.Sleep(time.Minute)
		return
	case "flood":
		fmt.Fprint(os.Stderr, strings.Repeat("x", 100000))
		time.Sleep(time.Minute)
		return
	case "exit":
		os.Exit(7)
	case "child", "descendant-child":
		time.Sleep(time.Minute)
		return
	case "descendant":
		cmd := exec.Command(os.Args[0] + "-child")
		if e := cmd.Start(); e != nil {
			panic(e)
		}
		if e := os.WriteFile(*pid, []byte(fmt.Sprint(cmd.Process.Pid)), 0600); e != nil {
			panic(e)
		}
	}
	for _, key := range []string{"HOME", "ZL_ACCOUNT", "OPENAI_API_KEY", "ANTHROPIC_API_KEY"} {
		if os.Getenv(key) != "" {
			panic("worker environment leaked " + key)
		}
	}
	_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"kind": *kind, "calls": 1000, "warmup": 50, "errors": 0, "bytes": 1000, "p95_seconds": .001, "allocations_per_op": 100, "alloc_bytes_per_op": 100, "elapsed_seconds": 1, "go": runtime.Version(), "os": runtime.GOOS, "arch": runtime.GOARCH, "concurrency": 1, "pacing_millis": 110, "admission_enabled": true, "exporters_enabled": false, "cleanup": true})
}
