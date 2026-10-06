package mobilebackup

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func TestNativeBinaryTerminalStartupAcceptance(t *testing.T) {
	binary := os.Getenv("ZL_MCP_ACCEPTANCE_BINARY")
	if binary == "" {
		t.Skip("set ZL_MCP_ACCEPTANCE_BINARY to explicitly test a built native binary")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("native acceptance binary must be absolute")
	}
	// Arrange: isolated synthetic terminal state and immutable source ownership.
	source, pending, db, dir, request := historyWorkerFixture(t)
	ctx := context.Background()
	s := source.session.store
	op, err := s.ClaimHistoryOperation(ctx, pending.Status.OperationID, pending.Revision)
	if err == nil {
		op, err = s.ReserveMobileHistoryWork(ctx, op.Status.OperationID, op.Revision, 180*time.Second)
	}
	if err == nil {
		_, err = s.PrepareMobileHistoryAcquisition(ctx, op.Status.OperationID, op.Revision, 0)
	}
	if err == nil {
		err = source.session.snapshots.Save(ctx, source.session.selected, request, "10")
	}
	if err == nil {
		_, err = s.StopHistoryOperation(ctx, op.Status.OperationID, op.Revision, "partial", "source_unavailable", 0)
	}
	if err != nil {
		t.Fatal(err)
	}
	if err = source.session.snapshots.Close(); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	state, err := os.MkdirTemp("/tmp", "zl-native-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(state) })
	bytes, err := os.ReadFile(db)
	if err != nil {
		t.Fatal(err)
	}
	db = filepath.Join(state, "messages.sqlite")
	if err = os.WriteFile(db, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	snapshots := filepath.Join(state, "mobile-snapshots")
	if err = os.Mkdir(snapshots, 0700); err != nil {
		t.Fatal(err)
	}
	sourceName, _ := snapshotName(request.RequestID)
	// /tmp and the fixture temp directory may live on different filesystems.
	for _, file := range []string{snapshotKeyName, snapshotClaimsName, sourceName} {
		data, e := os.ReadFile(filepath.Join(dir, file))
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(filepath.Join(snapshots, file), data, 0600); e != nil {
			t.Fatal(e)
		}
	}
	identities := make(map[string][32]byte)
	for _, file := range []string{snapshotKeyName, snapshotClaimsName} {
		data, err := os.ReadFile(filepath.Join(snapshots, file))
		if err != nil {
			t.Fatal(err)
		}
		identities[file] = sha256.Sum256(data)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	listener.Close()
	config := filepath.Join(state, "config.toml")
	content := fmt.Sprintf("state_dir = %q\n[collection]\nmode = \"all\"\n[logging]\nfile = %q\n[mcp]\nlisten = %q\n", state, filepath.Join(state, "service.log"), address)
	if err = os.WriteFile(config, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(binary, "-config", config, "service")
	command.Env = append(os.Environ(), "HOME="+state)
	output, err := os.OpenFile(filepath.Join(state, "startup.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()
	command.Stdout = output
	command.Stderr = output
	// Act: run the exact selected native executable without an authenticated session.
	if err = command.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	defer func() {
		command.Process.Signal(os.Interrupt)
		select {
		case err := <-done:
			if err != nil {
				t.Error("native shutdown failed", err)
			}
		case <-time.After(5 * time.Second):
			command.Process.Kill()
			<-done
			t.Error("native shutdown timeout")
		}
	}()
	name, _ := snapshotName(request.RequestID)
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err = os.Stat(filepath.Join(snapshots, name))
		if os.IsNotExist(err) {
			break
		}
		if err != nil || time.Now().After(deadline) {
			t.Fatal("native startup retained terminal image", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for {
		connection, e := net.DialTimeout("tcp", address, 100*time.Millisecond)
		if e == nil {
			connection.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("native service did not become ready", e)
		}
		time.Sleep(10 * time.Millisecond)
	}
	// Assert: native startup removed only staging data and retained journal identity.
	reopened, err := storage.OpenWithPolicy(ctx, db, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if op, err := reopened.HistoryOperation(ctx, pending.Status.OperationID); err != nil || op.Status.State != "partial" {
		t.Fatal("startup changed terminal history state", err)
	}
	assertSilentWorker(t, reopened, 0)
	for _, file := range []string{snapshotKeyName, snapshotClaimsName} {
		data, err := os.ReadFile(filepath.Join(snapshots, file))
		if err != nil || sha256.Sum256(data) != identities[file] {
			t.Fatal("startup changed private snapshot identity", err)
		}
	}
}
