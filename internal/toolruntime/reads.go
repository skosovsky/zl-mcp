// Package toolruntime owns the host's scoped toolsy execution boundary.
package toolruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/skosovsky/toolsy"
	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

var readNames = []string{"zalo_search_messages", "zalo_search_conversation_messages", "zalo_get_message_context", "zalo_get_conversation_message_context"}

type Authority interface {
	shared.IdentityPort
	search.AccessPort
	conversations.AccessPort
}

// Binding is minted only by the trusted host around its borrowed authority.
// Unexported ownership makes zero/foreign bindings invalid; it is not JSON data.
type Binding struct {
	owner    *Reads
	identity shared.ReadIdentity
}
type subject struct {
	owner              *Reads
	principal, account string
}
type scope struct {
	owner    *Reads
	revision string
}

type Reads struct {
	view          *toolsy.RegistryView
	authority     Authority
	search        *search.Handler
	conversations *conversations.Handler
	schemas       map[string]*jsonschema.Schema
}

// Result retains invocation-owned continuations; transport projects business
// values after tool execution without executing another handler to shrink them.
type Result struct {
	GroupSearch         *search.GroupExecution
	ConversationSearch  *search.ConversationExecution
	GroupContext        *conversations.GroupContext
	ConversationContext *conversations.ConversationContext
}

func (r *Result) value() any {
	switch {
	case r.GroupSearch != nil:
		return r.GroupSearch.Result
	case r.ConversationSearch != nil:
		return r.ConversationSearch.Result
	case r.GroupContext != nil:
		return *r.GroupContext
	case r.ConversationContext != nil:
		return *r.ConversationContext
	default:
		return nil
	}
}

func NewReads(searchHandler *search.Handler, conversationHandler *conversations.Handler, authority Authority) (*Reads, error) {
	if searchHandler == nil || conversationHandler == nil || authority == nil || searchHandler.Reads == nil || conversationHandler.Reads == nil {
		return nil, fmt.Errorf("typed handlers and read authority are required")
	}
	r := &Reads{authority: authority, search: searchHandler, conversations: conversationHandler, schemas: map[string]*jsonschema.Schema{}}
	builder := toolsy.NewRegistryBuilder(toolsy.WithPolicy("zl-local-read-v1", toolsy.PolicyFunc(r.decide)))
	for _, name := range readNames {
		schema, err := contracts.Compile(name, "input")
		if err != nil {
			return nil, err
		}
		r.schemas[name] = schema
		input, err := contracts.Document(name, "input")
		if err != nil {
			return nil, err
		}
		output, err := contracts.Document(name, "output")
		if err != nil {
			return nil, err
		}
		raw, err := json.Marshal(input)
		if err != nil {
			return nil, err
		}
		tool, err := toolsy.NewProxyTool(name, "Local read with current account policy", raw, func(ctx context.Context, _ *toolsy.RunEnv, rawArgs []byte, yield func(toolsy.Chunk) error) error {
			args, err := BindArguments(rawArgs)
			if err != nil {
				return err
			}
			result, err := r.dispatch(ctx, name, args)
			if err != nil {
				return err
			}
			data, err := json.Marshal(result.value())
			if err != nil {
				return err
			}
			return yield(toolsy.Chunk{Event: toolsy.EventResult, MimeType: toolsy.MimeTypeJSON, Data: data, TypedResult: result})
		}, toolsy.WithOutputSchema(output), toolsy.WithReadOnly(), toolsy.WithIdempotent())
		if err != nil {
			return nil, err
		}
		builder.Add(tool)
	}
	registry, err := builder.Build()
	if err != nil {
		return nil, err
	}
	r.view, err = registry.View(toolsy.RegistryViewSpec{ToolNames: readNames, RequiredToolNames: readNames, Owner: "zl-local-host", Reason: "first typed read slice"})
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Reads) Supports(name string) bool { _, ok := r.schemas[name]; return ok }

// Bind belongs to the trusted composition/transport side. Never expose it as a
// tool or let JSON arguments determine its principal/account/policy values.
func (r *Reads) Bind(ctx context.Context) (Binding, error) {
	identity, err := r.authority.ReadIdentity(ctx)
	if err != nil {
		return Binding{}, err
	}
	if identity.Principal == "" || identity.PolicyRevision == "" {
		return Binding{}, permission()
	}
	return Binding{owner: r, identity: identity}, nil
}
func permission() error {
	return &domain.Error{Code: "PERMISSION_DENIED", Message: "Local read authority is unavailable.", NextAction: domain.NextAction{Instruction: "Use the authorized local MCP transport."}, Details: map[string]any{}}
}
func invalidSchema() error {
	return domain.Invalid("Arguments do not match the tool schema; check types, required fields, ranges and RFC3339 dates.")
}

func (r *Reads) Execute(ctx context.Context, binding Binding, name string, raw []byte) (*Result, error) {
	if binding.owner != r {
		return nil, permission()
	}
	schema, ok := r.schemas[name]
	if !ok {
		return nil, permission()
	}
	args, err := BindArguments(raw)
	if err != nil {
		return nil, err
	}
	if err = schema.Validate(args); err != nil {
		return nil, invalidSchema()
	}
	normalized, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	var result *Result
	call := toolsy.ToolCall{ToolName: name, Input: toolsy.ToolInput{ArgsJSON: normalized}, Env: toolsy.NewRunEnv(nil), CallContext: toolsy.NewCallContext(subject{r, binding.identity.Principal, binding.identity.Account}, scope{r, binding.identity.PolicyRevision})}
	err = r.view.Execute(ctx, call, func(chunk toolsy.Chunk) error {
		value, ok := chunk.TypedResult.(*Result)
		if chunk.Event != toolsy.EventResult || chunk.IsError || !ok || result != nil {
			return fmt.Errorf("unexpected read execution result")
		}
		result = value
		return nil
	})
	if err != nil {
		return nil, mapError(err)
	}
	if result == nil {
		return nil, fmt.Errorf("missing read execution result")
	}
	return result, nil
}

func (r *Reads) decide(ctx context.Context, req toolsy.PolicyRequest) toolsy.Decision {
	identity, ok := req.CallContext.Subject.(subject)
	capability, scopeOK := req.CallContext.Scope.(scope)
	if !ok || !scopeOK || identity.owner != r || capability.owner != r {
		return deny(permission())
	}
	current, err := r.authority.ReadIdentity(ctx)
	if err != nil {
		return deny(err)
	}
	if current.Principal != identity.principal || current.Account != identity.account || current.PolicyRevision != capability.revision {
		return deny(permission())
	}
	args, err := BindArguments(req.Input.ArgsJSON)
	if err != nil {
		return deny(err)
	}
	schema, ok := r.schemas[req.Manifest.Name]
	if !ok {
		return deny(permission())
	}
	if err = schema.Validate(args); err != nil {
		return deny(invalidSchema())
	}
	switch req.Manifest.Name {
	case "zalo_search_messages":
		err = r.authority.CheckGroupSearch(ctx, groupRequest(args))
	case "zalo_search_conversation_messages":
		err = r.authority.CheckConversationSearch(ctx, conversationRequest(args))
	case "zalo_get_message_context":
		err = r.authority.CheckGroupContext(ctx, contextRequest(args, true))
	case "zalo_get_conversation_message_context":
		err = r.authority.CheckConversationContext(ctx, contextRequest(args, false))
	default:
		err = permission()
	}
	if err != nil {
		return deny(err)
	}
	return toolsy.AllowDecision()
}
func deny(err error) toolsy.Decision {
	d := toolsy.DenyDecision("current local read authority rejected the call")
	d.Err = err
	return d
}
func mapError(err error) error {
	var domainError *domain.Error
	if errors.As(err, &domainError) {
		return domainError
	}
	if errors.Is(err, toolsy.ErrValidation) {
		return invalidSchema()
	}
	if errors.Is(err, toolsy.ErrCapabilityDenied) {
		return permission()
	}
	return err // Existing wire projection keeps storage/internal details private.
}
func (r *Reads) dispatch(ctx context.Context, name string, args map[string]any) (*Result, error) {
	out := &Result{}
	var err error
	switch name {
	case "zalo_search_messages":
		out.GroupSearch, err = r.search.Group(ctx, groupRequest(args))
	case "zalo_search_conversation_messages":
		out.ConversationSearch, err = r.search.Conversation(ctx, conversationRequest(args))
	case "zalo_get_message_context":
		var v conversations.GroupContext
		v, err = r.conversations.GroupContext(ctx, contextRequest(args, true))
		out.GroupContext = &v
	case "zalo_get_conversation_message_context":
		var v conversations.ConversationContext
		v, err = r.conversations.ConversationContext(ctx, contextRequest(args, false))
		out.ConversationContext = &v
	default:
		err = permission()
	}
	return out, err
}
