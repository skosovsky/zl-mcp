package main

import (
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
)

func TestRecallCommandRoutesThroughExistingOwnerSocket(t *testing.T) {
	// Arrange: an isolated owner socket; no Zalo client, credentials or corpus.
	dir, err := os.MkdirTemp("/tmp", "zl-cli-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	configPath := filepath.Join(dir, "config.toml")
	if err = os.WriteFile(configPath, []byte("state_dir = "+`"`+dir+`"`+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", filepath.Join(dir, "collector.sock"))
	if err != nil {
		t.Fatal(err)
	}
	requests := make(chan map[string]any, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if r.Method != "POST" || r.URL.Path != "/rpc" || json.NewDecoder(r.Body).Decode(&request) != nil {
			http.Error(w, "bad request", 400)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"state": "reserved"})
	})}
	go server.Serve(listener)
	t.Cleanup(func() { server.Close() })
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previousArgs, previousOutput, previousLogger := os.Args, os.Stdout, slog.Default()
	t.Cleanup(func() {
		os.Args = previousArgs
		os.Stdout = previousOutput
		slog.SetDefault(previousLogger)
		read.Close()
		write.Close()
	})
	id := "00000000-0000-4000-8000-000000000031"
	os.Args = []string{"zl-mcp", "-config", configPath, "probe-recall", id}
	os.Stdout = write
	// Act: exercise the actual command dispatcher, rather than just its argument parser.
	err = run()
	write.Close()
	output, readErr := io.ReadAll(read)
	// Assert: exact executable owner contract and receipt, without another listener/login.
	if err != nil || readErr != nil {
		t.Fatal("recall command did not reach owner control", err, readErr)
	}
	select {
	case request := <-requests:
		schema, compileErr := contracts.Compile("cli_probe_recall", "input")
		if compileErr != nil || schema.Validate(request) != nil || request["method"] != "cli_probe_recall" || request["arguments"].(map[string]any)["send_request_id"] != id {
			t.Fatal("wrong owner request")
		}
	default:
		t.Fatal("owner request missing")
	}
	var receipt map[string]any
	if json.Unmarshal(output, &receipt) != nil || receipt["state"] != "reserved" {
		t.Fatal("owner receipt lost")
	}
}
