package mcpserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	syntheticWorkflowRuleID           = "synthetic-notify-incoming"
	syntheticWorkflowChannel          = "local-test-counter"
	syntheticWorkflowNotificationRule = "Notify only the local synthetic counter for each new exact incoming event; never send anything to Zalo."
)

// Synthetic saved workflow owned by this test client, not an Ann/Codex task store
// or a model evaluation. It deliberately contains no callback/MCP credentials.
type savedReadinessWorkflow struct {
	RuleID                 string         `json:"rule_id"`
	ReadTool               string         `json:"read_tool"`
	AckTool                string         `json:"ack_tool"`
	ReadArguments          map[string]any `json:"read_arguments"`
	BusinessSubscriptionID string         `json:"business_subscription_id"`
	GenerationReference    string         `json:"generation_reference"`
	BusinessFilter         map[string]any `json:"business_filter"`
	NotificationChannel    string         `json:"notification_channel"`
	NotificationRule       string         `json:"notification_rule"`
	Language               string         `json:"language"`
	AllowZaloWrites        bool           `json:"allow_zalo_writes"`
}

func readSavedReadinessWorkflow(path string) (savedReadinessWorkflow, error) {
	var workflow savedReadinessWorkflow
	raw, err := os.ReadFile(path)
	if err != nil {
		return workflow, err
	}
	err = json.Unmarshal(raw, &workflow)
	return workflow, err
}
func saveReadinessWorkflow(path string, workflow savedReadinessWorkflow) error {
	raw, err := json.MarshalIndent(workflow, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}
func advertisedPersistedReadiness(tools []*mcp.Tool) bool {
	for _, tool := range tools {
		if tool.Name != "zalo_read_subscription_events" {
			continue
		}
		raw, err := json.Marshal(tool.InputSchema)
		if err != nil {
			return false
		}
		var schema map[string]any
		if json.Unmarshal(raw, &schema) != nil {
			return false
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return false
		}
		readiness, ok := properties["readiness_mode"].(map[string]any)
		if !ok {
			return false
		}
		values, ok := readiness["enum"].([]any)
		if !ok {
			return false
		}
		for _, value := range values {
			if value == "persisted" {
				return true
			}
		}
	}
	return false
}

// migrateSavedReadinessWorkflow is only this documented synthetic client's
// save/readback adapter. Production server/skills never mutate an external task.
func migrateSavedReadinessWorkflow(path string, tools []*mcp.Tool) error {
	if !advertisedPersistedReadiness(tools) {
		return errors.New("persisted readiness is not advertised; saved workflow unchanged")
	}
	workflow, err := readSavedReadinessWorkflow(path)
	if err != nil {
		return err
	}
	if workflow.ReadTool != "zalo_read_subscription_events" || workflow.ReadArguments == nil {
		return errors.New("unsupported synthetic workflow")
	}
	workflow.ReadArguments["readiness_mode"] = "persisted"
	return saveReadinessWorkflow(path, workflow)
}
func executeSavedReadinessWorkflow(h *recoveryReadyHarness, path string) (map[string]any, error) {
	workflow, err := readSavedReadinessWorkflow(path)
	if err != nil {
		return nil, err
	}
	// This client's existing rule is exact and explicit. Migration grants no new
	// direction, recipient, action permission or fallback lookup by recency.
	if !savedWorkflowAuthorityMatches(h, workflow) {
		return nil, errors.New("saved rule authority mismatch")
	}
	page, err := h.call(workflow.ReadTool, workflow.ReadArguments)
	if err != nil {
		return nil, err
	}
	for _, value := range page["events"].([]any) {
		entry := value.(map[string]any)
		if entry["processing_state"] != "ready" || entry["event"] == nil {
			return nil, errors.New("saved rule does not authorize guessing or skipping gaps")
		}
		event := entry["event"].(map[string]any)
		data := event["data"].(map[string]any)
		if data["conversation_type"] != workflow.BusinessFilter["conversation_type"] ||
			data["conversation_id"] != workflow.BusinessFilter["conversation_id"] ||
			data["direction"] != workflow.BusinessFilter["direction"] {
			return nil, errors.New("event outside saved rule")
		}
		h.mu.Lock()
		h.actions[event["eventId"].(string)]++
		h.mu.Unlock()
		if _, err = h.call(
			workflow.AckTool,
			map[string]any{"subscription_id": workflow.BusinessSubscriptionID, "receipt": entry["receipt"]},
		); err != nil {
			return nil, err
		}
	}
	return page, nil
}
func savedWorkflowAuthorityMatches(h *recoveryReadyHarness, workflow savedReadinessWorkflow) bool {
	return !workflow.AllowZaloWrites && workflow.Language == "ru" && workflow.BusinessSubscriptionID == h.business &&
		workflow.GenerationReference == h.generation &&
		workflow.ReadArguments["subscription_id"] == h.business &&
		workflow.RuleID == syntheticWorkflowRuleID &&
		workflow.ReadTool == "zalo_read_subscription_events" &&
		workflow.AckTool == "zalo_ack_subscription_events" &&
		workflow.NotificationChannel == syntheticWorkflowChannel &&
		workflow.NotificationRule == syntheticWorkflowNotificationRule
}
func syntheticReadinessWorkflow(h *recoveryReadyHarness) savedReadinessWorkflow {
	return savedReadinessWorkflow{
		RuleID:                 syntheticWorkflowRuleID,
		ReadTool:               "zalo_read_subscription_events",
		AckTool:                "zalo_ack_subscription_events",
		ReadArguments:          map[string]any{"subscription_id": h.business, "limit": float64(20)},
		BusinessSubscriptionID: h.business,
		GenerationReference:    h.generation,
		BusinessFilter: map[string]any{
			"scope":               "conversation",
			"conversation_type":   "direct",
			"conversation_id":     "peer",
			"direction":           "incoming",
			"first_incoming_only": false,
		},
		NotificationChannel: syntheticWorkflowChannel, NotificationRule: syntheticWorkflowNotificationRule,
		Language:        "ru",
		AllowZaloWrites: false,
	}
}

type savedWorkflowObservation struct {
	page, legacy map[string]any
	state        string
	err          error
}

//nolint:cyclop,gocognit,gocyclo // One synthetic adapter AAA scenario verifies discovery, save/readback, both facts during sending and durable checkpoint after reload.
func TestSyntheticClientSavedWorkflowMigrationPreservesRuleAndProcessesBefore2xx(t *testing.T) {
	// Arrange: a generic client owns its temporary saved workflow and exact native
	// subscription. No real host task, Ann automation or production service is used.
	h := newRecoveryReadyHarness(t)
	h.assertNoNativeActions()
	path := filepath.Join(t.TempDir(), "workflow.json")
	original := syntheticReadinessWorkflow(h)
	if err := saveReadinessWorkflow(path, original); err != nil {
		t.Fatal(err)
	}
	discovered, err := h.client.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	// Act: actual SDK discovery gates this client's explicit one-field migration.
	if err = migrateSavedReadinessWorkflow(path, discovered.Tools); err != nil {
		t.Fatal(err)
	}
	saved, err := readSavedReadinessWorkflow(path)
	if err != nil || saved.ReadArguments["readiness_mode"] != "persisted" {
		t.Fatal("saved migration not read back", err)
	}
	delete(saved.ReadArguments, "readiness_mode")
	if !reflect.DeepEqual(original, saved) {
		t.Fatal("migration changed subscription/filter/direction/rule/language/permissions")
	}
	migratedBytes, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	received := make(chan savedWorkflowObservation, 1)
	h.mu.Lock()
	h.businessHook = func(w http.ResponseWriter, _ *http.Request, _ map[string]any, _ []byte) {
		observation := savedWorkflowObservation{}
		observation.legacy, observation.err = defaultJournal(h)
		if observation.err == nil {
			observation.page, observation.err = executeSavedReadinessWorkflow(h, path)
		}
		if observation.err == nil {
			observation.state, _, observation.err = processingDeliveryState(h)
		}
		received <- observation
		if observation.err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
	h.mu.Unlock()
	h.put("saved-rule-test")
	barrier := &recoveryFinishBarrier{Store: h.store, entered: make(chan struct{}), release: make(chan struct{})}
	h.worker.Queue = barrier
	t.Cleanup(func() {
		select {
		case <-barrier.release:
		default:
			close(barrier.release)
		}
	})
	finished := make(chan error, 1)
	go func() {
		handled, dispatchErr := h.worker.DeliverOne(t.Context(), time.Now())
		if dispatchErr == nil && !handled {
			dispatchErr = errors.New("synthetic event not dispatched")
		}
		finished <- dispatchErr
	}()
	var observed savedWorkflowObservation
	select {
	case observed = <-received:
	case <-time.After(5 * time.Second):
		t.Fatal("saved workflow callback timeout")
	}
	if observed.err != nil {
		t.Fatal(observed.err)
	}
	// Assert: the callback loaded the saved fixture and processed it before2xx;
	// delivery is still sending, and the unchanged default view still blocks.
	if observed.state != "sending" || observed.legacy["blocked_on_delivery"] != true ||
		observed.page["readiness_mode"] != "persisted" ||
		observed.page["blocked_on_delivery"] != false {
		t.Fatal("saved workflow did not separate processing from callback receipt")
	}
	entries := observed.page["events"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["processing_state"] != "ready" {
		t.Fatal("saved rule did not consume exact event")
	}
	select {
	case <-barrier.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("receipt commit barrier not reached")
	}
	state, size, err := processingDeliveryState(h)
	if err != nil || state != "sending" || size == 0 {
		t.Fatal("saved processing ack modified unfinished delivery", err)
	}
	close(barrier.release)
	if err = <-finished; err != nil {
		t.Fatal(err)
	}
	// Reload from disk and reopen real temporary DB/service/SDK/worker: the same
	// saved rule uses the shared durable checkpoint, producing no duplicate action.
	h.stop()
	h.open()
	afterRestart, err := executeSavedReadinessWorkflow(h, path)
	if err != nil || len(afterRestart["events"].([]any)) != 0 {
		t.Fatal("reloaded rule replayed acknowledged event", err)
	}
	assertProcessingCheckpointEmpty(t, h)
	reloadedBytes, readErr := os.ReadFile(path)
	if readErr != nil || !bytes.Equal(migratedBytes, reloadedBytes) {
		t.Fatal("restart or consumer changed saved rules/permissions", readErr)
	}
	h.mu.Lock()
	actions := 0
	for _, count := range h.actions {
		actions += count
	}
	h.mu.Unlock()
	if actions != 1 {
		t.Fatal("saved workflow duplicated or lost its authorized local action")
	}
	h.assertNoNativeActions()
}

func TestSyntheticClientWorkflowMigrationDeclinesMissingCapabilityWithoutChangingSave(t *testing.T) {
	// Arrange: real discovery is copied as an old-schema capability witness; only
	// its readiness property is removed, never sent as an unknown argument.
	h := newRecoveryReadyHarness(t)
	path := filepath.Join(t.TempDir(), "workflow.json")
	if err := saveReadinessWorkflow(path, syntheticReadinessWorkflow(h)); err != nil {
		t.Fatal(err)
	}
	discovered, err := h.client.ListTools(t.Context(), &mcp.ListToolsParams{})
	if err != nil {
		t.Fatal(err)
	}
	var oldCatalog []*mcp.Tool
	for _, tool := range discovered.Tools {
		if tool.Name != "zalo_read_subscription_events" {
			continue
		}
		raw, marshalErr := json.Marshal(tool.InputSchema)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		var schema map[string]any
		if unmarshalErr := json.Unmarshal(raw, &schema); unmarshalErr != nil {
			t.Fatal(unmarshalErr)
		}
		delete(schema["properties"].(map[string]any), "readiness_mode")
		oldCatalog = append(oldCatalog, &mcp.Tool{Name: tool.Name, InputSchema: schema})
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Act: a client with a proven old catalogue must leave the task untouched.
	migrationErr := migrateSavedReadinessWorkflow(path, oldCatalog)
	// Assert: no optimistic argument injection or widened workflow on an old host.
	after, err := os.ReadFile(path)
	if err != nil || migrationErr == nil || !bytes.Equal(before, after) {
		t.Fatal("unsupported migration changed saved workflow", err)
	}
	workflow, err := readSavedReadinessWorkflow(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := workflow.ReadArguments["readiness_mode"]; present {
		t.Fatal("unsupported readiness argument injected")
	}
	h.assertNoNativeActions()
}
