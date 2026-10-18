package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/events"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

const pluginActionID = "a0000000-0000-4000-8000-000000000001"

type pluginInstance struct {
	config, state, endpoint, token, text, actionState string
	command                                           *exec.Cmd
	finished                                          chan error
}

// TestNativePluginInstanceIsolation exercises the shipped binary without a Zalo
// session: auth_required is expected, while the retained corpus remains usable.
func TestNativePluginInstanceIsolation(t *testing.T) {
	binary := os.Getenv("ZL_MCP_ACCEPTANCE_BINARY")
	if binary == "" {
		t.Skip("set ZL_MCP_ACCEPTANCE_BINARY to test the built native service and bridges")
	}
	// Arrange: independent persistent instances deliberately share public IDs.
	a := pluginArrange(t, "a", "alpha-isolated", "accepted")
	b := pluginArrange(t, "b", "beta-isolated", "rejected")
	pluginStart(t, binary, a)
	pluginStart(t, binary, b)
	// Act / Assert: the same IDs resolve within the selected credential/config.
	for _, instance := range []*pluginInstance{a, b} {
		pluginAssertCorpus(t, instance)
		pluginAssertBridge(t, binary, instance)
	}
	for _, pair := range [][2]*pluginInstance{{a, b}, {b, a}} {
		status, _ := pluginRPC(t, pair[0], pair[1].token, "zalo_get_status", map[string]any{})
		if status != http.StatusUnauthorized {
			t.Fatalf("neighbor token accepted: HTTP %d", status)
		}
	}
	// A colliding listener and duplicate state owner must fail without affecting
	// either running instance. These are real service subprocesses, not port mocks.
	collision := pluginArrange(t, "collision", "collision-isolated", "rejected")
	configData, err := os.ReadFile(collision.config)
	if err != nil {
		t.Fatal(err)
	}
	configData = bytes.ReplaceAll(
		configData,
		[]byte(strings.TrimPrefix(collision.endpoint, "http://")),
		[]byte(strings.TrimPrefix(a.endpoint, "http://")),
	)
	if err = os.WriteFile(collision.config, configData, 0600); err != nil {
		t.Fatal(err)
	}
	pluginExpectStartupFailure(t, binary, collision.config)
	pluginExpectStartupFailure(t, binary, a.config)
	pluginAssertCorpus(t, a)
	pluginAssertCorpus(t, b)
	// Stopping/restarting one account must preserve both durable checkpoints and
	// leave its neighbor serving throughout.
	pluginStop(t, a)
	pluginAssertCorpus(t, b)
	pluginStart(t, binary, a)
	pluginAssertCorpus(t, a)
	pluginAssertCorpus(t, b)
	pluginAssertCheckpointIsolation(t, binary, a, b)
}

func pluginArrange(t *testing.T, name, text, actionState string) *pluginInstance {
	t.Helper()
	//nolint:usetesting // Short paths keep the service's macOS Unix socket within its bound.
	dir, err := os.MkdirTemp("/tmp", "zl-plugin-"+name+"-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().String()
	if err = listener.Close(); err != nil {
		t.Fatal(err)
	}
	token := fmt.Sprintf("%x", sha256.Sum256([]byte("synthetic-plugin-token:"+name)))
	instance := &pluginInstance{
		config:      filepath.Join(dir, "config.toml"),
		state:       dir,
		endpoint:    "http://" + address,
		token:       token,
		text:        text,
		actionState: actionState,
	}
	configuration := fmt.Sprintf(
		"state_dir = %q\n[collection]\nmode = \"all\"\n[mcp]\nlisten = %q\ntoken_file = %q\n[logging]\nfile = %q\n",
		dir,
		address,
		filepath.Join(dir, "token"),
		filepath.Join(dir, "service.log"),
	)
	if err = os.WriteFile(instance.config, []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, "token"), []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	pluginSeed(t, instance, name)
	return instance
}

func pluginSeed(t *testing.T, instance *pluginInstance, account string) {
	t.Helper()
	ctx := t.Context()
	store, err := storage.OpenWithPolicy(
		ctx,
		filepath.Join(instance.state, "messages.sqlite"),
		domain.CollectionPolicy{All: true},
		90,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.BindAccount(ctx, "synthetic-plugin-"+account); err != nil {
		t.Fatal(err)
	}
	sub := storage.EventSubscription{
		ID:        "same-subscription",
		Principal: "local:" + strconv.Itoa(os.Getuid()),
		Profile:   domain.ConversationMessageCreatedV2,
		Scope:     "all",
		Callback:  "https://127.0.0.1/events",
		Secret:    "synthetic",
	}
	revision, err := store.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ActivateSubscription(ctx, sub, revision, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	ref := domain.ConversationRef{Type: "direct", ID: "same-peer"}
	message := domain.Message{
		Conversation: ref,
		ID:           "same-message",
		SenderID:     ref.ID,
		Direction:    "incoming",
		Text:         instance.text,
		SentAt:       time.Now().UTC(),
		Source:       "live",
	}
	if err = store.Put(ctx, message); err != nil {
		t.Fatal(err)
	}
	encoder, err := events.NewMessageEncoder()
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.FanoutProfileEvents(
		ctx,
		time.Now().UTC(),
		storage.DefaultDeliveryPolicy(),
		encoder.EncodeProfile,
	); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PrepareMessageAction(
		ctx,
		pluginActionID,
		"read",
		"read",
		ref,
		strings.Repeat("a", 64),
	); err != nil {
		t.Fatal(err)
	}
	if claimed, claimErr := store.ClaimMessageAction(ctx, pluginActionID); claimErr != nil ||
		!claimed {
		t.Fatalf("action claim: %v %v", claimed, claimErr)
	}
	reason := "upstream_rejected"
	if instance.actionState == "accepted" {
		reason = ""
		if err = store.BeginMessageActionStep(ctx, pluginActionID, "read", 0); err != nil {
			t.Fatal(err)
		}
		if err = store.ConfirmMessageActionStep(ctx, pluginActionID, "read", 0); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.CompleteMessageAction(ctx, pluginActionID, instance.actionState, reason); err != nil {
		t.Fatal(err)
	}
}

func pluginStart(t *testing.T, binary string, instance *pluginInstance) {
	t.Helper()
	instance.command = exec.Command(binary, "-config", instance.config, "service")
	instance.finished = make(chan error, 1)
	if err := instance.command.Start(); err != nil {
		t.Fatal(err)
	}
	command, finished := instance.command, instance.finished
	go func() { finished <- command.Wait() }()
	t.Cleanup(func() {
		if instance.command == command {
			pluginStop(t, instance)
		}
	})
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		connection, err := net.DialTimeout(
			"tcp",
			strings.TrimPrefix(instance.endpoint, "http://"),
			100*time.Millisecond,
		)
		if err == nil {
			_ = connection.Close()
			return
		}
		select {
		case err = <-finished:
			t.Fatalf("native service exited before ready: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatal("native service startup timeout")
}

func pluginStop(t *testing.T, instance *pluginInstance) {
	t.Helper()
	if instance.command == nil {
		return
	}
	if err := instance.command.Process.Signal(os.Interrupt); err != nil {
		t.Error(err)
	}
	select {
	case err := <-instance.finished:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(15 * time.Second):
		_ = instance.command.Process.Kill()
		<-instance.finished
		t.Error("native service shutdown timeout")
	}
	instance.command = nil
}

func pluginRPC(
	t *testing.T,
	instance *pluginInstance,
	token, tool string,
	arguments map[string]any,
) (int, map[string]any) {
	t.Helper()
	body, err := json.Marshal(
		map[string]any{
			"jsonrpc": "2.0",
			"id":      1,
			"method":  "tools/call",
			"params": map[string]any{
				"name":      tool,
				"arguments": arguments,
				"_meta": map[string]any{
					"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
					"io.modelcontextprotocol/clientCapabilities": map[string]any{},
				},
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	request, err := http.NewRequestWithContext(
		t.Context(),
		http.MethodPost,
		instance.endpoint+"/mcp",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	request.Header.Set("Mcp-Protocol-Version", "2026-07-28")
	request.Header.Set("Mcp-Method", "tools/call")
	request.Header.Set("Mcp-Name", tool)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, response.Body)
		return response.StatusCode, nil
	}
	var reply struct {
		Result map[string]any `json:"result"`
		Error  any            `json:"error"`
	}
	if err = json.NewDecoder(response.Body).Decode(&reply); err != nil || reply.Error != nil {
		t.Fatalf("native RPC decode: %v %v", err, reply.Error)
	}
	return response.StatusCode, reply.Result
}

func pluginCall(
	t *testing.T,
	instance *pluginInstance,
	tool string,
	arguments map[string]any,
) map[string]any {
	t.Helper()

	for attempt := range 3 {
		status, result := pluginRPC(t, instance, instance.token, tool, arguments)
		if status == http.StatusOK && result["isError"] != true {
			return result["structuredContent"].(map[string]any)
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if attempt == 2 || !bytes.Contains(encoded, []byte("RATE_LIMITED")) {
			t.Fatalf("%s: HTTP %d result=%v", tool, status, result)
		}
		// Respect only the server's explicit retryable admission denial; do not
		// retry authorization, account-binding, operation, or transport failures.
		select {
		case <-t.Context().Done():
			t.Fatal(t.Context().Err())
		case <-time.After(time.Second):
		}
	}
	t.Fatal("unreachable exhausted admission retries")
	return nil
}

func pluginAssertCorpus(t *testing.T, instance *pluginInstance) {
	t.Helper()
	status := pluginCall(t, instance, "zalo_get_status", map[string]any{})
	if status["stored_message_count"] != float64(1) {
		t.Fatalf("unexpected corpus count: %v", status)
	}

	search := pluginCall(
		t,
		instance,
		"zalo_search_conversation_messages",
		map[string]any{"query": "isolated", "conversation_type": "direct", "conversation_id": "same-peer"},
	)
	if len(search["messages"].([]any)) != 1 {
		t.Fatal("search crossed instance boundary", search)
	}
	contextResult := pluginCall(
		t,
		instance,
		"zalo_get_conversation_message_context",
		map[string]any{
			"conversation_type": "direct",
			"conversation_id":   "same-peer",
			"message_id":        "same-message",
		},
	)
	if contextResult["anchor"].(map[string]any)["text"] != instance.text {
		t.Fatal("same message ID crossed instance boundary")
	}
	action := pluginCall(
		t,
		instance,
		"zalo_get_message_action_status",
		map[string]any{"request_id": pluginActionID},
	)
	if action["status"] != instance.actionState {
		t.Fatal("same action UUID crossed instance boundary", action)
	}
	journal := pluginCall(
		t,
		instance,
		"zalo_read_subscription_events",
		map[string]any{"subscription_id": "same-subscription", "readiness_mode": "persisted"},
	)
	entries := journal["events"].([]any)
	if len(entries) != 1 ||
		entries[0].(map[string]any)["event"].(map[string]any)["data"].(map[string]any)["text"] != instance.text {
		t.Fatal("same subscription ID crossed instance boundary", journal)
	}
}

func pluginAssertCheckpointIsolation(t *testing.T, binary string, a, b *pluginInstance) {
	t.Helper()
	page := pluginCall(
		t,
		a,
		"zalo_read_subscription_events",
		map[string]any{"subscription_id": "same-subscription", "readiness_mode": "persisted"},
	)
	receipt := page["events"].([]any)[0].(map[string]any)["receipt"]
	arguments := map[string]any{"subscription_id": "same-subscription", "receipt": receipt}
	status, rejected := pluginRPC(t, b, b.token, "zalo_ack_subscription_events", arguments)
	if status != http.StatusOK || rejected["isError"] != true {
		t.Fatal("foreign journal receipt accepted", status, rejected)
	}
	pluginCall(t, a, "zalo_ack_subscription_events", arguments)
	pluginStop(t, a)
	pluginAssertCorpus(t, b)
	pluginStart(t, binary, a)
	empty := pluginCall(
		t,
		a,
		"zalo_read_subscription_events",
		map[string]any{"subscription_id": "same-subscription", "readiness_mode": "persisted"},
	)
	if len(empty["events"].([]any)) != 0 {
		t.Fatal("own checkpoint did not advance")
	}
	pluginAssertCorpus(t, b)
}

func pluginAssertBridge(t *testing.T, binary string, instance *pluginInstance) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "plugin-isolation", Version: "1"}, nil)
	session, err := client.Connect(
		ctx,
		&mcp.CommandTransport{Command: exec.Command(binary, "-config", instance.config, "serve")},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	result, err := session.CallTool(
		ctx,
		&mcp.CallToolParams{
			Name: "zalo_get_conversation_message_context",
			Arguments: map[string]any{
				"conversation_type": "direct",
				"conversation_id":   "same-peer",
				"message_id":        "same-message",
			},
		},
	)
	if err != nil || result.IsError {
		t.Fatalf("bridge context: %v %v", result, err)
	}
	data, err := json.Marshal(result.StructuredContent)
	if err != nil || !bytes.Contains(data, []byte(instance.text)) {
		t.Fatal("bridge routed to wrong instance", err)
	}
}

func pluginExpectStartupFailure(t *testing.T, binary, config string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary, "-config", config, "service")
	if err := command.Run(); err == nil || ctx.Err() != nil {
		t.Fatalf("conflicting service failed to reject promptly: %v", err)
	}
}

// TestNativePluginSharedBinaryReplacement proves the shared-file update and
// rollback procedure with different native builds. It does not claim schema
// migration compatibility between arbitrary product versions.
func TestNativePluginSharedBinaryReplacement(t *testing.T) {
	baseline := os.Getenv("ZL_MCP_ACCEPTANCE_BINARY")
	candidate := os.Getenv("ZL_MCP_UPGRADE_BINARY")
	if baseline == "" || candidate == "" {
		t.Skip("set both native binary environment variables to test shared replacement and rollback")
	}
	// Arrange: two stopped-instance backups surround one shared binary path.
	baselineData := pluginReadFile(t, baseline)
	candidateData := pluginReadFile(t, candidate)
	if sha256.Sum256(baselineData) == sha256.Sum256(candidateData) {
		t.Fatal("replacement candidate must have different binary bytes")
	}
	shared := filepath.Join(t.TempDir(), "zl-mcp")
	pluginReplaceBinary(t, shared, baselineData)
	a := pluginArrange(t, "upgrade-a", "alpha-isolated", "accepted")
	b := pluginArrange(t, "upgrade-b", "beta-isolated", "rejected")
	for _, instance := range []*pluginInstance{a, b} {
		pluginStart(t, shared, instance)
		pluginAssertCorpus(t, instance)
	}
	for _, instance := range []*pluginInstance{a, b} {
		pluginStop(t, instance)
	}
	backups := []string{filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")}
	for i, instance := range []*pluginInstance{a, b} {
		pluginCopyState(t, instance.state, backups[i])
	}
	// Act: atomically replace the common executable only while both owners stop.
	pluginReplaceBinary(t, shared, candidateData)
	if sha256.Sum256(pluginReadFile(t, shared)) != sha256.Sum256(candidateData) {
		t.Fatal("shared candidate was not installed")
	}
	for _, instance := range []*pluginInstance{a, b} {
		pluginStart(t, shared, instance)
		pluginAssertCorpus(t, instance)
		pluginAssertBridge(t, shared, instance)
	}
	// Assert rollback restores the executable and each account's matching private
	// backup before either previous-version process is allowed to start.
	for _, instance := range []*pluginInstance{a, b} {
		pluginStop(t, instance)
	}
	pluginReplaceBinary(t, shared, baselineData)
	if sha256.Sum256(pluginReadFile(t, shared)) != sha256.Sum256(baselineData) {
		t.Fatal("shared baseline was not restored")
	}
	for i, instance := range []*pluginInstance{a, b} {
		if err := os.RemoveAll(instance.state); err != nil {
			t.Fatal(err)
		}
		pluginCopyState(t, backups[i], instance.state)
		pluginStart(t, shared, instance)
		pluginAssertCorpus(t, instance)
	}
	// Removing one stopped installation preserves the shared executable and B.
	pluginStop(t, a)
	if err := os.RemoveAll(a.state); err != nil {
		t.Fatal(err)
	}
	pluginAssertCorpus(t, b)
	pluginAssertBridge(t, shared, b)
	if sha256.Sum256(pluginReadFile(t, shared)) != sha256.Sum256(baselineData) {
		t.Fatal("removing one instance changed shared executable")
	}
}

func pluginReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func pluginReplaceBinary(t *testing.T, path string, data []byte) {
	t.Helper()
	pending := path + ".pending"
	if err := os.WriteFile(pending, data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(pending, path); err != nil {
		t.Fatal(err)
	}
}

// pluginCopyState runs only after the service has joined. Copying the complete
// directory includes any SQLite WAL/SHM files and account-owned journal secrets.
func pluginCopyState(t *testing.T, source, target string) {
	t.Helper()
	if err := os.MkdirAll(target, 0700); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		from, to := filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())
		if entry.IsDir() {
			pluginCopyState(t, from, to)
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			t.Fatal(infoErr)
		}
		if !info.Mode().IsRegular() {
			t.Fatalf("stopped instance contains unexpected nonregular file: %s", entry.Name())
		}
		if err = os.WriteFile(to, pluginReadFile(t, from), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
