package messaging

import (
	"context"
	"encoding/json"
	"errors"
	operationsadapter "github.com/skosovsky/zl-mcp/internal/adapters/operations"
	storeadapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
	"github.com/skosovsky/zl-mcp/internal/app/history"
	"github.com/skosovsky/zl-mcp/internal/app/membership"
	messagingapp "github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/local"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/toolruntime"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This helper only runs in explicitly selected synthetic child processes.
func TestCommandOwnerCrashHelper(t *testing.T) {
	mode := os.Getenv("ZL_T07_CRASH_MODE")
	if mode == "" {
		t.Skip("subprocess helper")
	}
	dir := os.Getenv("ZL_T07_CRASH_DIR")
	ctx := context.Background()
	unlock, err := local.Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	owner, err := storage.OpenWithPolicy(ctx, filepath.Join(dir, "state.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if err = owner.BindAccount(ctx, "synthetic-subprocess-account"); err != nil {
		t.Fatal(err)
	}
	if err = owner.RecoverInterruptedSends(ctx); err != nil {
		t.Fatal(err)
	}
	sender := senderFunc(func(ctx context.Context, _ string, _ string, _ *domain.SendQuote) (string, error) {
		counter, err := os.OpenFile(filepath.Join(dir, "effects"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return "", err
		}
		_, err = counter.WriteString("send\n")
		if err == nil {
			err = counter.Sync()
		}
		closeErr := counter.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			return "", closeErr
		}
		if mode == "crash" {
			if err = os.WriteFile(filepath.Join(dir, "ready"), []byte("native send entered after durable claim"), 0600); err != nil {
				return "", err
			}
			<-ctx.Done()
			return "", ctx.Err()
		}
		return "unexpected-resend", nil
	})
	manager := &Manager{Store: owner, Sender: sender, Enabled: true, Recipients: map[string]bool{"synthetic-peer": true}}
	reader := storeadapter.New(owner)
	operations := operationsadapter.NewReader(manager)
	runtime, err := toolruntime.New(&search.Handler{Reads: reader}, &conversations.Handler{Reads: reader}, reader, toolruntime.Capabilities{
		Messaging: &messagingapp.Handler{Reads: operations, Commands: operations}, History: &history.Handler{Reads: operations, Commands: operations}, Membership: &membership.Handler{Reads: operations, Commands: operations}, Events: &eventaccess.Handler{Journal: reader, Commands: reader},
	})
	if err != nil {
		t.Fatal(err)
	}
	binding, err := runtime.Bind(ctx)
	if err != nil {
		t.Fatal(err)
	}
	request := []byte(`{"recipient_id":"synthetic-peer","text":"synthetic Cà phê 🍀","request_id":"00000000-0000-4000-8000-000000000007"}`)
	result, err := runtime.Execute(ctx, binding, "zalo_send_direct_message", request)
	if err != nil {
		t.Fatal(err)
	}
	if mode == "crash" {
		t.Fatal("crash helper returned before abrupt termination")
	}
	if result.SendStatus == nil || result.SendStatus.Status != "unknown" || result.SendStatus.Reason == nil || *result.SendStatus.Reason != "interrupted" || result.SendStatus.RetrySafe {
		t.Fatalf("restart changed ambiguous send: %+v", result.SendStatus)
	}
	conflicting := []byte(strings.Replace(string(request), "synthetic Cà phê 🍀", "different synthetic text", 1))
	_, err = runtime.Execute(ctx, binding, "zalo_send_direct_message", conflicting)
	var failure *domain.Error
	if !errors.As(err, &failure) || failure.Code != "REQUEST_CONFLICT" {
		t.Fatal("conflicting request lost durable identity", err)
	}
	data, err := json.Marshal(result.Value())
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "recovered.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestScopedCommandOwnerSurvivesAbruptSubprocessCrashWithoutResend(t *testing.T) {
	// Arrange: two distinct OS processes use the same real native ledger; the
	// synthetic upstream records effects independently before blocking its reply.
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	child := func(mode string) *exec.Cmd {
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCommandOwnerCrashHelper$", "-test.count=1")
		command.Env = append(os.Environ(), "ZL_T07_CRASH_MODE="+mode, "ZL_T07_CRASH_DIR="+dir)
		return command
	}
	crashing := child("crash")
	if err := crashing.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = crashing.Process.Kill()
			_ = crashing.Wait()
		}
	}()
	readyDeadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(dir, "ready")); err == nil {
			break
		}
		if ctx.Err() != nil || time.Now().After(readyDeadline) {
			t.Fatal("native owner never reached durable send claim")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Act: kill the owner while upstream acceptance is ambiguous, with no defer
	// cleanup, graceful shutdown or completion write; reopen in another OS process.
	if err := crashing.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	err := crashing.Wait()
	waited = true
	if err == nil {
		t.Fatal("expected abrupt subprocess termination")
	}
	recovery := child("recover")
	if output, err := recovery.CombinedOutput(); err != nil {
		t.Fatalf("restart: %v %s", err, output)
	}
	// Assert: independent effect counter and native durable operation agree.
	effects, err := os.ReadFile(filepath.Join(dir, "effects"))
	if err != nil {
		t.Fatal(err)
	}
	if string(effects) != "send\n" {
		t.Fatalf("owner resent after restart: %q", effects)
	}
	wire, err := os.ReadFile(filepath.Join(dir, "recovered.json"))
	if err != nil {
		t.Fatal(err)
	}
	var operation domain.SendOperation
	if err = json.Unmarshal(wire, &operation); err != nil {
		t.Fatal(err)
	}
	if operation.RequestID != "00000000-0000-4000-8000-000000000007" || operation.Status != "unknown" || operation.MessageID != nil || operation.RetrySafe {
		t.Fatal(operation)
	}
}
