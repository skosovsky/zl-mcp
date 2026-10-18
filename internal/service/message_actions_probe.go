package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/messageactions"
)

const operationMessageActionProbe = "cli_message_action_probe"

// This diagnostic route is owner-only Unix IPC, never a discoverable MCP tool.
// It submits ONE explicitly selected primitive to establish native semantics;
// no automatic clear/add replacement is exposed before acceptance.
func (p *membershipPort) messageActionProbeControl(w http.ResponseWriter, r *http.Request, body []byte) {
	w.Header().Set("Content-Type", "application/json")
	fail := func() {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).
			Encode(map[string]string{"code": "ACTION_PROBE_UNAVAILABLE", "message": "Invalid, unavailable or unauthorized local message action probe."})
	}
	var request struct {
		Method    string          `json:"method"`
		Arguments json.RawMessage `json:"arguments"`
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if r.Method != http.MethodPost || r.URL.Path != controlRPCPath || !uniqueActionProbeJSON(body) ||
		decoder.Decode(&request) != nil ||
		request.Method != operationMessageActionProbe {
		fail()
		return
	}
	schema, err := contracts.Compile("message_action_probe", "input")
	var args map[string]any
	if err != nil || json.Unmarshal(request.Arguments, &args) != nil || schema.Validate(args) != nil {
		fail()
		return
	}
	var input struct {
		RequestID        string   `json:"request_id"`
		ConversationType string   `json:"conversation_type"`
		ConversationID   string   `json:"conversation_id"`
		MessageIDs       []string `json:"message_ids"`
		Plan             string   `json:"plan"`
		Reaction         string   `json:"reaction"`
	}
	if json.Unmarshal(request.Arguments, &input) != nil {
		fail()
		return
	}
	kind := "reaction"
	if input.Plan == messageActionReadKind {
		kind = messageActionReadKind
	}
	intent := domain.MessageActionIntent{
		RequestID:    input.RequestID,
		Kind:         kind,
		Plan:         input.Plan,
		Conversation: domain.ConversationRef{Type: input.ConversationType, ID: input.ConversationID},
		MessageIDs:   input.MessageIDs,
		Reaction:     input.Reaction,
	}
	p.mu.RLock()
	defer p.mu.RUnlock()
	manager := messageactions.Manager{
		Store:         p.store,
		Source:        p.actions,
		ReadChats:     p.readChats,
		ReactionChats: p.reactionChats,
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	for _, parent := range []context.Context{p.lifecycle, p.actionContext} {
		if parent != nil {
			stop := context.AfterFunc(parent, cancel)
			defer stop()
			if parent.Err() != nil {
				cancel()
			}
		}
	}
	result, err := manager.Execute(ctx, intent)
	if err != nil {
		fail()
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}

// Only bounded request JSON is accepted. Credentials/native objects are not CLI
// arguments; the service resolves metadata and enforces per-chat permissions.
func messageActionProbeArguments(in io.Reader) (json.RawMessage, error) {
	body, err := io.ReadAll(io.LimitReader(in, maximumOwnerRequestBytes+1))
	if err != nil || len(body) > maximumOwnerRequestBytes || !uniqueActionProbeJSON(body) {
		return nil, errors.New("invalid message action probe JSON")
	}
	schema, err := contracts.Compile("message_action_probe", "input")
	var args any
	if err != nil || json.Unmarshal(body, &args) != nil || schema.Validate(args) != nil {
		return nil, errors.New("invalid message action probe arguments")
	}
	return body, nil
}

func allowedActionChats(refs []domain.ConversationRef) map[domain.ConversationRef]bool {
	allowed := make(map[domain.ConversationRef]bool, len(refs))
	for _, ref := range refs {
		allowed[ref] = true
	}
	return allowed
}
