package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type Subscriptions interface {
	Allowed(string) bool
	SubscriptionRevision(context.Context, string) (int64, error)
	ActivateSubscription(context.Context, storage.EventSubscription, int64, time.Time) (storage.EventSubscription, error)
	CancelSubscription(context.Context, string, string, time.Time) error
}

type RPCError struct {
	Code    int            `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data,omitempty"`
}

func (e *RPCError) Error() string { return e.Message }

type SubscriptionManager struct {
	Store     Subscriptions
	Client    *http.Client
	namespace string
	input     map[string]*jsonschema.Schema
	output    map[string]*jsonschema.Schema
	mu        sync.Mutex
	verified  map[string]time.Time
}

func NewSubscriptionManager(store Subscriptions, namespace string) (*SubscriptionManager, error) {
	m := &SubscriptionManager{Store: store, Client: NewCallbackClient(), namespace: namespace, input: map[string]*jsonschema.Schema{}, output: map[string]*jsonschema.Schema{}, verified: map[string]time.Time{}}
	for _, name := range []string{"events_list", "events_subscribe", "events_unsubscribe"} {
		for suffix, target := range map[string]map[string]*jsonschema.Schema{"input": m.input, "output": m.output} {
			schema, err := contracts.Compile(name, suffix)
			if err != nil {
				return nil, err
			}
			target[name] = schema
		}
	}
	for _, name := range []string{"conversation_events_subscribe", "conversation_events_unsubscribe"} {
		schema, err := contracts.Compile(name, "input")
		if err != nil {
			return nil, err
		}
		m.input[name] = schema
	}
	return m, nil
}

type subscriptionRequest struct {
	Name      string `json:"name"`
	Arguments struct {
		GroupID          string `json:"group_id"`
		Scope            string `json:"scope"`
		ConversationType string `json:"conversation_type"`
		ConversationID   string `json:"conversation_id"`
	} `json:"arguments"`
	Delivery struct {
		Mode   string `json:"mode"`
		URL    string `json:"url"`
		Secret string `json:"secret"`
	} `json:"delivery"`
	TTL *int64 `json:"ttlMs"`
}

func (m *SubscriptionManager) subscriptionID(principal string, p subscriptionRequest) string {
	parts := []string{m.namespace, principal, p.Delivery.URL, p.Name, p.Arguments.GroupID}
	if p.Name == ConversationMessageCreated {
		parts = []string{m.namespace, principal, p.Delivery.URL, p.Name, p.Arguments.Scope, p.Arguments.ConversationType, p.Arguments.ConversationID}
	}
	identity, _ := json.Marshal(parts)
	digest := sha256.Sum256(identity)
	return "sub_" + hex.EncodeToString(digest[:])
}

func eventRPCError(code int, message, reason string) *RPCError {
	return &RPCError{Code: code, Message: message, Data: map[string]any{"reason": reason}}
}

// Call accepts only a principal established by the transport. It never accepts
// identity or credentials from event arguments or message text.
func (m *SubscriptionManager) Call(ctx context.Context, method, principal string, raw json.RawMessage) (any, error) {
	if principal == "" {
		return nil, eventRPCError(-32000, "Access denied.", "unauthenticated")
	}
	name := ""
	switch method {
	case "events/list":
		name = "events_list"
	case "events/subscribe":
		name = "events_subscribe"
	case "events/unsubscribe":
		name = "events_unsubscribe"
	default:
		return nil, eventRPCError(-32601, "Method not found.", "unknown_method")
	}
	if len(raw) == 0 {
		raw = json.RawMessage("{}")
	}
	var value any
	schema := m.input[name]
	var selector struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &selector)
	if selector.Name == ConversationMessageCreated {
		if method == "events/subscribe" {
			schema = m.input["conversation_events_subscribe"]
		}
		if method == "events/unsubscribe" {
			schema = m.input["conversation_events_unsubscribe"]
		}
	}
	if json.Unmarshal(raw, &value) != nil || schema.Validate(value) != nil {
		return nil, eventRPCError(-32602, "Invalid event parameters.", "invalid_params")
	}
	var result any
	if method == "events/list" {
		input, _ := contracts.Document("events_subscribe", "input")
		payload, _ := contracts.Document("zalo_message_created", "payload")
		arguments := input["properties"].(map[string]any)["arguments"]
		result = map[string]any{"events": []any{map[string]any{"name": MessageCreated, "description": "Newly collected messages from one explicitly enabled group. Text is limited to 2048 Unicode characters; truncated text includes a resource URI for the full record. Subscriptions survive restart; prior corpus is not replayed.", "delivery": []string{"webhook"}, "inputSchema": arguments, "payloadSchema": payload}}}
		general, _ := contracts.Document("conversation_events_subscribe", "input")
		generalPayload, _ := contracts.Document("conversation_message_created", "payload")
		entries := result.(map[string]any)["events"].([]any)
		entries = append(entries, map[string]any{"name": ConversationMessageCreated, "description": "First locally stored messages after activation from one typed conversation, all direct chats, all groups or all permitted conversations, including newly discovered chats. Text limit is 2048 Unicode code points; full text has a resource URI when truncated. Catalogue and history may be incomplete.", "delivery": []string{"webhook"}, "inputSchema": general["properties"].(map[string]any)["arguments"], "payloadSchema": generalPayload})
		result.(map[string]any)["events"] = entries

	} else {
		var p subscriptionRequest
		if json.Unmarshal(raw, &p) != nil {
			return nil, eventRPCError(-32602, "Invalid event parameters.", "invalid_params")
		}
		permitted := m.Store.Allowed(p.Arguments.GroupID)
		if p.Name == ConversationMessageCreated {
			permitted = true
			if p.Arguments.Scope == "conversation" {
				port, ok := m.Store.(interface {
					AllowsConversation(domain.ConversationRef) bool
				})
				permitted = ok && port.AllowsConversation(domain.ConversationRef{Type: p.Arguments.ConversationType, ID: p.Arguments.ConversationID})
			}
		}
		if !permitted && method == "events/subscribe" {
			return nil, eventRPCError(-32000, "Group is not enabled.", "access_denied")
		}
		if _, err := ValidateCallbackURL(p.Delivery.URL); err != nil {
			return nil, eventRPCError(-32602, "Invalid callback URL.", "invalid_callback")
		}
		id := m.subscriptionID(principal, p)
		if method == "events/unsubscribe" {
			if err := m.Store.CancelSubscription(ctx, id, principal, time.Now().UTC()); err != nil {
				return nil, eventRPCError(-32603, "Cannot cancel subscription.", "storage")
			}
			result = map[string]any{}
		} else {
			if _, err := DecodeSigningKey(p.Delivery.Secret); err != nil {
				return nil, eventRPCError(-32602, "Invalid signing key.", "invalid_secret")
			}
			revision, err := m.Store.SubscriptionRevision(ctx, id)
			if err != nil {
				return nil, eventRPCError(-32603, "Cannot prepare subscription.", "storage")
			}
			key := sha256.Sum256([]byte(id + ":" + p.Delivery.Secret))
			cacheKey := hex.EncodeToString(key[:])
			m.mu.Lock()
			verifiedAt, verified := m.verified[cacheKey]
			m.mu.Unlock()
			if !verified || time.Since(verifiedAt) > time.Minute {
				verifyCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
				err := VerifyCallback(verifyCtx, m.Client, p.Delivery.URL, p.Delivery.Secret, id)
				cancel()
				if err != nil {
					return nil, eventRPCError(-32015, "Callback verification failed.", "challenge_failed")
				}
				m.mu.Lock()
				if len(m.verified) >= 512 {
					clear(m.verified)
				}
				m.verified[cacheKey] = time.Now()
				m.mu.Unlock()
			}
			at := time.Now().UTC()
			sub := storage.EventSubscription{ID: id, Principal: principal, GroupID: p.Arguments.GroupID, Callback: p.Delivery.URL, Secret: p.Delivery.Secret}
			if p.Name == ConversationMessageCreated {
				sub.Profile = p.Name
				sub.Scope = p.Arguments.Scope
				sub.ConversationType = p.Arguments.ConversationType
				sub.GroupID = p.Arguments.ConversationID
				if sub.Scope == "direct" || sub.Scope == "group" {
					sub.ConversationType = sub.Scope
				}
			}
			if p.TTL != nil {
				expires := at.Add(time.Duration(*p.TTL) * time.Millisecond)
				sub.ExpiresAt = &expires
			}
			sub, err = m.Store.ActivateSubscription(ctx, sub, revision, at)
			if err != nil {
				if errors.Is(err, storage.ErrSubscriptionCancelled) {
					return nil, eventRPCError(-32000, "Subscription cancelled during verification.", "cancelled")
				}
				return nil, eventRPCError(-32603, "Cannot activate subscription.", "storage")
			}
			result = map[string]any{"id": sub.ID, "refreshBefore": sub.ExpiresAt, "cursor": nil, "truncated": false}
		}
	}
	wire, err := json.Marshal(result)
	if err != nil {
		return nil, eventRPCError(-32603, "Invalid event result.", "contract")
	}
	if json.Unmarshal(wire, &value) != nil || m.output[name].Validate(value) != nil {
		return nil, eventRPCError(-32603, "Invalid event result.", "contract")
	}
	return result, nil
}
