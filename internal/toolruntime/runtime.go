// Package toolruntime owns the host's scoped toolsy execution boundary.
package toolruntime

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/skosovsky/toolsy"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/app/archives"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
	"github.com/skosovsky/zl-mcp/internal/app/history"
	"github.com/skosovsky/zl-mcp/internal/app/membership"
	"github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/app/shared"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

//nolint:gochecknoglobals // Private immutable read-capability allowlist is shared by admission and dispatch.
var readNames = []string{
	toolSearchMessages,
	toolSearchConversationMessages,
	toolGetMessageContext,
	toolGetConversationMessageContext,
}

type Authority interface {
	shared.IdentityPort
	search.AccessPort
	conversations.AccessPort
}

// Binding is minted only by the trusted host around its borrowed authority.
// Unexported ownership makes zero/foreign bindings invalid; it is not JSON data.
type Binding struct {
	owner    *Runtime
	identity shared.ReadIdentity
}
type subject struct {
	owner              *Runtime
	principal, account string
}
type scope struct {
	owner    *Runtime
	revision string
}

// Capabilities names the fixed read capabilities; each native owner keeps its ledger.
type Capabilities struct {
	Events     *eventaccess.Handler
	Archives   *archives.Handler
	Messaging  *messaging.Handler
	History    *history.Handler
	Membership *membership.Handler
}

type Runtime struct {
	operations    Capabilities
	view          *toolsy.RegistryView
	authority     Authority
	search        *search.Handler
	conversations *conversations.Handler
	schemas       map[string]*jsonschema.Schema
}

// Result retains invocation-owned continuations; transport projects business
// values after tool execution without executing another handler to shrink them.
type Result struct {
	Acknowledgement     *eventaccess.Acknowledgement
	Subscriptions       *eventaccess.Subscriptions
	Events              *eventaccess.Page
	Directory           *conversations.DirectoryExecution
	ConversationDetail  *conversations.Detail
	Browse              *conversations.Browse
	ArchiveSources      *archives.Sources
	ArchiveDirectory    *archives.DirectoryExecution
	ArchiveBrowse       *archives.BrowseExecution
	GroupSearch         *search.GroupExecution
	ConversationSearch  *search.ConversationExecution
	GroupContext        *conversations.GroupContext
	ConversationContext *conversations.ConversationContext
	CollectorStatus     *conversations.CollectorStatus
	GroupDetail         *conversations.GroupDetail
	GroupList           *conversations.GroupExecution
	SendStatus          *domain.SendOperation
	ImportStatus        *domain.HistoryImportStatus
	JoinStatus          *membership.JoinStatus
	Invite              *membership.Invite
}

func (r *Result) Value() any { return r.value() }

func (r *Result) value() any {
	switch {
	case r.Acknowledgement != nil:
		return *r.Acknowledgement
	case r.Subscriptions != nil:
		return *r.Subscriptions
	case r.Events != nil:
		return *r.Events
	case r.Directory != nil:
		return r.Directory.Result
	case r.ConversationDetail != nil:
		return *r.ConversationDetail
	case r.Browse != nil:
		return *r.Browse
	case r.ArchiveSources != nil:
		return *r.ArchiveSources
	case r.ArchiveDirectory != nil:
		return r.ArchiveDirectory.Result
	case r.ArchiveBrowse != nil:
		return r.ArchiveBrowse.Result
	case r.GroupSearch != nil:
		return r.GroupSearch.Result
	case r.ConversationSearch != nil:
		return r.ConversationSearch.Result
	case r.GroupContext != nil:
		return *r.GroupContext
	case r.ConversationContext != nil:
		return *r.ConversationContext
	case r.CollectorStatus != nil:
		return *r.CollectorStatus
	case r.GroupDetail != nil:
		return *r.GroupDetail
	case r.GroupList != nil:
		return r.GroupList.Result
	case r.SendStatus != nil:
		return *r.SendStatus
	case r.ImportStatus != nil:
		return *r.ImportStatus
	case r.JoinStatus != nil:
		return *r.JoinStatus
	case r.Invite != nil:
		return *r.Invite
	default:
		return nil
	}
}

//nolint:cyclop,funlen,gocognit,gocyclo,nestif // This declarative registry binds the complete fixed capability/schema set; keep registration and required-port checks together.
func New(
	searchHandler *search.Handler,
	conversationHandler *conversations.Handler,
	authority Authority,
	extra ...Capabilities,
) (*Runtime, error) {
	if searchHandler == nil || conversationHandler == nil || authority == nil || searchHandler.Reads == nil ||
		conversationHandler.Reads == nil {
		return nil, errors.New("typed handlers and read authority are required")
	}
	//nolint:exhaustruct_v5 // Runtime preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
	r := &Runtime{
		authority:     authority,
		search:        searchHandler,
		conversations: conversationHandler,
		schemas:       map[string]*jsonschema.Schema{},
	}
	names := append([]string(nil), readNames...)
	if conversationHandler.Catalog != nil {
		names = append(names, "zalo_get_status", "zalo_list_groups")
		if conversationHandler.LiveGroups != nil {
			names = append(names, "zalo_get_group")
		}
	}
	if conversationHandler.DirectoryReads != nil {
		names = append(names, "zalo_get_conversation", "zalo_list_conversations", "zalo_list_conversation_messages")
	}
	if len(extra) > 1 {
		return nil, errors.New("one operation read bundle is permitted")
	}
	if len(extra) == 1 {
		r.operations = extra[0]
		v := r.operations
		if v.Events != nil {
			if v.Events.Journal == nil {
				return nil, errors.New("typed Events journal port is required")
			}
			names = append(names, "zalo_list_event_subscriptions", "zalo_read_subscription_events")
		}
		if v.Archives != nil {
			if v.Archives.Reads == nil {
				return nil, errors.New("typed archive port is required")
			}
			names = append(names, "zalo_list_archive_sources")
		}
		if v.Messaging != nil || v.History != nil || v.Membership != nil {
			if v.Messaging == nil || v.History == nil || v.Membership == nil || v.Messaging.Reads == nil ||
				v.History.Reads == nil ||
				v.Membership.Reads == nil {
				return nil, errors.New("all typed operation read ports are required")
			}
			names = append(
				names,
				"zalo_get_send_status",
				"zalo_get_history_import_status",
				"zalo_get_join_status",
				"zalo_inspect_invite",
			)
		}
		if commandsPresent(v) {
			if v.Messaging == nil || v.History == nil || v.Membership == nil || v.Events == nil ||
				v.Messaging.Commands == nil ||
				v.History.Commands == nil ||
				v.Membership.Commands == nil ||
				v.Events.Commands == nil {
				return nil, errors.New("all typed command ports are required")
			}
			names = append(
				names,
				toolSendDirectMessage,
				toolJoinGroup,
				toolImportConversationHistory,
				toolCancelHistoryImport,
				toolAckSubscriptionEvents,
			)
		}
	}
	builder := toolsy.NewRegistryBuilder(toolsy.WithPolicy("zl-local-read-v1", toolsy.PolicyFunc(r.decide)))
	for _, name := range names {
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
		options := []toolsy.ToolOption{toolsy.WithOutputSchema(output), toolsy.WithIdempotent()}
		if !isCommand(name) {
			options = append(options, toolsy.WithReadOnly())
		}
		tool, err := toolsy.NewProxyTool(
			name,
			"Local capability with current account policy",
			raw,
			func(ctx context.Context, _ *toolsy.RunEnv, rawArgs []byte, yield func(toolsy.Chunk) error) error {
				args, bindArgumentsErr := BindArguments(rawArgs)
				if bindArgumentsErr != nil {
					return bindArgumentsErr
				}
				result, bindArgumentsErr := r.dispatch(ctx, name, args)
				if bindArgumentsErr != nil {
					return bindArgumentsErr
				}
				data, bindArgumentsErr := json.Marshal(result.value())
				if bindArgumentsErr != nil {
					return bindArgumentsErr
				}
				return yield(
					//nolint:exhaustruct_v5 // Chunk preserves optional upstream fields absent from this request/projection.
					toolsy.Chunk{
						Event:       toolsy.EventResult,
						MimeType:    toolsy.MimeTypeJSON,
						Data:        data,
						TypedResult: result,
					},
				)
			},
			options...)
		if err != nil {
			return nil, err
		}
		builder.Add(tool)
	}
	registry, err := builder.Build()
	if err != nil {
		return nil, err
	}
	r.view, err = registry.View(
		//nolint:exhaustruct_v5 // RegistryViewSpec preserves optional upstream fields absent from this request/projection.
		toolsy.RegistryViewSpec{
			ToolNames:         names,
			RequiredToolNames: names,
			Owner:             "zl-local-host",
			Reason:            "typed read capabilities",
		},
	)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Runtime) Supports(name string) bool { _, ok := r.schemas[name]; return ok }

// Bind belongs to the trusted composition/transport side. Never expose it as a
// tool or let JSON arguments determine its principal/account/policy values.
func (r *Runtime) Bind(ctx context.Context) (Binding, error) {
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
	//nolint:exhaustruct_v5 // Omitted error fields retain their non-retryable/absent defaults in the existing response contract.
	return &domain.Error{
		Code:    "PERMISSION_DENIED",
		Message: "Local read authority is unavailable.",
		//nolint:exhaustruct_v5 // Optional tool/arguments stay absent until an actionable follow-up exists; preserve the response contract.
		NextAction: domain.NextAction{Instruction: "Use the authorized local MCP transport."},
		Details:    map[string]any{},
	}
}
func invalidSchema() error {
	return domain.Invalid(
		"Arguments do not match the tool schema; check types, required fields, ranges and RFC3339 dates.",
	)
}

func (r *Runtime) Execute(ctx context.Context, binding Binding, name string, raw []byte) (*Result, error) {
	observer := shared.Telemetry(ctx)
	if observer == nil {
		return r.execute(ctx, binding, name, raw)
	}
	var result *Result
	err := observer.Execute(ctx, name, func(ctx context.Context) (domain.Outcome, error) {
		var err error
		result, err = r.execute(ctx, binding, name, raw)
		if err != nil {
			return domain.OutcomeError, err
		}
		result.remember(ctx, name, observer)
		return result.Outcome(), nil
	})
	return result, err
}

func (r *Runtime) execute(ctx context.Context, binding Binding, name string, raw []byte) (*Result, error) {
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
	call := toolsy.ToolCall{
		ToolName: name,
		//nolint:exhaustruct_v5 // ToolInput preserves optional upstream fields absent from this request/projection.
		Input: toolsy.ToolInput{ArgsJSON: normalized},
		Env:   toolsy.NewRunEnv(nil),
		CallContext: toolsy.NewCallContext(
			subject{r, binding.identity.Principal, binding.identity.Account},
			scope{r, binding.identity.PolicyRevision},
		),
	}
	err = r.view.Execute(ctx, call, func(chunk toolsy.Chunk) error {
		value, ok := chunk.TypedResult.(*Result)
		if chunk.Event != toolsy.EventResult || chunk.IsError || !ok || result != nil {
			return errors.New("unexpected read execution result")
		}
		result = value
		return nil
	})
	if err != nil {
		return nil, mapError(err)
	}
	if result == nil {
		return nil, errors.New("missing read execution result")
	}
	return result, nil
}

func (r *Runtime) decide(ctx context.Context, req toolsy.PolicyRequest) toolsy.Decision {
	identity, ok := req.CallContext.Subject.(subject)
	capability, scopeOK := req.CallContext.Scope.(scope)
	if !ok || !scopeOK || identity.owner != r || capability.owner != r {
		return deny(permission())
	}
	current, err := r.authority.ReadIdentity(ctx)
	if err != nil {
		return deny(err)
	}
	if current.Principal != identity.principal || current.Account != identity.account ||
		current.PolicyRevision != capability.revision {
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
	case toolSearchMessages:
		err = r.authority.CheckGroupSearch(ctx, groupRequest(args))
	case toolSearchConversationMessages:
		err = r.authority.CheckConversationSearch(ctx, conversationRequest(args))
	case toolGetMessageContext:
		err = r.authority.CheckGroupContext(ctx, contextRequest(args, true))
	case toolGetConversationMessageContext:
		err = r.authority.CheckConversationContext(ctx, contextRequest(args, false))
	case toolSendDirectMessage,
		toolJoinGroup,
		toolImportConversationHistory,
		toolCancelHistoryImport,
		toolAckSubscriptionEvents,
		"zalo_list_event_subscriptions",
		"zalo_read_subscription_events",
		"zalo_get_conversation",
		"zalo_list_conversations",
		"zalo_list_conversation_messages",
		"zalo_list_archive_sources",
		"zalo_get_status",
		"zalo_list_groups",
		"zalo_get_group",
		"zalo_get_send_status",
		"zalo_get_history_import_status",
		"zalo_get_join_status",
		"zalo_inspect_invite":
		// Native owners recheck operation/account authority; schema and host scope
		// have already been checked without invoking a command.
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
	if domainError, ok := errors.AsType[*domain.Error](err); ok {
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

//nolint:cyclop,funlen,gocognit,gocyclo // The closed typed operation dispatch keeps each BYOT request/result pair visible and invokes exactly one consumer-owned operation.
func (r *Runtime) dispatch(ctx context.Context, name string, args map[string]any) (*Result, error) {
	//nolint:exhaustruct_v5 // Result preserves omitted optional DTO fields and zero counters in the existing wire/persistence projection.
	out := &Result{}
	var err error
	switch name {
	case toolSendDirectMessage:
		var reply *string
		if text, ok := args["reply_to_message_id"].(string); ok {
			reply = &text
		}
		var v domain.SendOperation
		v, err = r.operations.Messaging.Send(
			ctx,
			domain.SendRequest{
				RecipientID: stringArg(args, "recipient_id"),
				Text:        stringArg(args, "text"),
				RequestID:   stringArg(args, "request_id"),
				ReplyTo:     reply,
			},
		)
		out.SendStatus = &v
	case toolJoinGroup:
		var v membership.JoinStatus
		v, err = r.operations.Membership.Join(
			ctx,
			membership.JoinRequest{PlanToken: stringArg(args, "plan_token"), RequestID: stringArg(args, "request_id")},
		)
		out.JoinStatus = &v
	case toolImportConversationHistory:
		var v domain.HistoryImportStatus
		v, err = r.operations.History.Start(
			ctx,
			history.StartRequest{
				Source:           stringArg(args, "source"),
				ConversationType: stringArg(args, "conversation_type"),
				ConversationID:   stringArg(args, "conversation_id"),
				RequestID:        stringArg(args, "request_id"),
				Since:            stringArg(args, "since"),
				Until:            stringArg(args, "until"),
				PageSize:         optionalInt(args, "page_size"),
				MaxPages:         optionalInt(args, "max_pages"),
				MaxMessages:      optionalInt(args, "max_messages"),
			},
		)
		out.ImportStatus = &v
	case toolCancelHistoryImport:
		var v domain.HistoryImportStatus
		v, err = r.operations.History.Cancel(ctx, stringArg(args, "operation_id"))
		out.ImportStatus = &v
	case toolAckSubscriptionEvents:
		identity, e := r.authority.ReadIdentity(ctx)
		if e != nil {
			return nil, e
		}
		var v eventaccess.Acknowledgement
		v, err = r.operations.Events.Acknowledge(
			ctx,
			identity.Principal,
			stringArg(args, "subscription_id"),
			stringArg(args, "receipt"),
		)
		out.Acknowledgement = &v
	case "zalo_list_event_subscriptions":
		identity, e := r.authority.ReadIdentity(ctx)
		if e != nil {
			return nil, e
		}
		var v eventaccess.Subscriptions
		v, err = r.operations.Events.Subscriptions(ctx, identity.Principal)
		out.Subscriptions = &v
	case "zalo_read_subscription_events":
		identity, e := r.authority.ReadIdentity(ctx)
		if e != nil {
			return nil, e
		}
		var v eventaccess.Page
		v, err = r.operations.Events.ReadEvents(
			ctx,
			identity.Principal,
			stringArg(args, "subscription_id"),
			intArg(args, "limit", defaultReadLimit),
		)
		out.Events = &v
	case toolSearchMessages:
		out.GroupSearch, err = r.search.Group(ctx, groupRequest(args))
	case toolSearchConversationMessages:
		out.ConversationSearch, err = r.search.Conversation(ctx, conversationRequest(args))
	case toolGetMessageContext:
		var v conversations.GroupContext
		v, err = r.conversations.GroupContext(ctx, contextRequest(args, true))
		out.GroupContext = &v
	case toolGetConversationMessageContext:
		var v conversations.ConversationContext
		v, err = r.conversations.ConversationContext(ctx, contextRequest(args, false))
		out.ConversationContext = &v
	case "zalo_get_conversation":
		var v conversations.Detail
		v, err = r.conversations.Conversation(ctx, ref(args))
		out.ConversationDetail = &v
	case "zalo_list_conversations":
		if source := stringArg(args, "source_id"); source != "" {
			if r.operations.Archives == nil {
				return nil, permission()
			}
			out.ArchiveDirectory, err = r.operations.Archives.Directory(
				ctx,
				archives.DirectoryRequest{
					SourceID: source,
					Type:     stringArg(args, "conversation_type"),
					Query:    stringArg(args, "query"),
					Cursor:   stringArg(args, "cursor"),
					Limit:    intArg(args, "limit", defaultReadLimit),
				},
			)
		} else {
			out.Directory, err = r.conversations.Directory(
				ctx,
				conversations.DirectoryRequest{
					Type:   stringArg(args, "conversation_type"),
					Query:  stringArg(args, "query"),
					Cursor: stringArg(args, "cursor"),
					Limit:  intArg(args, "limit", defaultReadLimit),
				},
			)
		}
	case "zalo_list_conversation_messages":
		if source := stringArg(args, "source_id"); source != "" {
			if r.operations.Archives == nil {
				return nil, permission()
			}
			out.ArchiveBrowse, err = r.operations.Archives.Browse(
				ctx,
				archives.BrowseRequest{
					SourceID:         source,
					ConversationType: stringArg(args, "conversation_type"),
					ConversationID:   stringArg(args, "conversation_id"),
					Since:            stringArg(args, "since"),
					Until:            stringArg(args, "until"),
					Order:            stringArg(args, "order"),
					Cursor:           stringArg(args, "cursor"),
					Limit:            intArg(args, "limit", defaultReadLimit),
				},
			)
		} else {
			var v conversations.Browse
			v, err = r.conversations.Browse(
				ctx,
				conversations.BrowseRequest{
					Conversation: ref(args),
					Since:        stringArg(args, "since"),
					Until:        stringArg(args, "until"),
					Order:        stringArg(args, "order"),
					Cursor:       stringArg(args, "cursor"),
					Limit:        intArg(args, "limit", defaultReadLimit),
				},
			)
			out.Browse = &v
		}
	case "zalo_list_archive_sources":
		var v archives.Sources
		v, err = r.operations.Archives.Sources(ctx)
		out.ArchiveSources = &v
	case "zalo_get_status":
		var v conversations.CollectorStatus
		v, err = r.conversations.Status(ctx)
		out.CollectorStatus = &v
	case "zalo_list_groups":
		out.GroupList, err = r.conversations.Groups(
			ctx,
			conversations.GroupRequest{
				Query:  stringArg(args, "query"),
				Limit:  intArg(args, "limit", defaultReadLimit),
				Cursor: stringArg(args, "cursor"),
			},
		)
	case "zalo_get_group":
		var v conversations.GroupDetail
		v, err = r.conversations.Group(ctx, stringArg(args, "group_id"))
		out.GroupDetail = &v
	case "zalo_get_send_status":
		var v domain.SendOperation
		v, err = r.operations.Messaging.Status(ctx, stringArg(args, "request_id"))
		out.SendStatus = &v
	case "zalo_get_history_import_status":
		var v domain.HistoryImportStatus
		v, err = r.operations.History.Status(ctx, stringArg(args, "operation_id"))
		out.ImportStatus = &v
	case "zalo_get_join_status":
		var v membership.JoinStatus
		v, err = r.operations.Membership.Status(ctx, stringArg(args, "operation_id"))
		out.JoinStatus = &v
	case "zalo_inspect_invite":
		var v membership.Invite
		v, err = r.operations.Membership.Inspect(ctx, stringArg(args, "invite_url"))
		out.Invite = &v
	default:
		err = permission()
	}
	return out, err
}

func isCommand(name string) bool {
	switch name {
	case toolSendDirectMessage,
		toolJoinGroup,
		toolImportConversationHistory,
		toolCancelHistoryImport,
		toolAckSubscriptionEvents:
		return true
	}
	return false
}
func commandsPresent(v Capabilities) bool {
	return v.Messaging != nil && v.Messaging.Commands != nil || v.History != nil && v.History.Commands != nil ||
		v.Membership != nil && v.Membership.Commands != nil ||
		v.Events != nil && v.Events.Commands != nil
}
func optionalInt(args map[string]any, key string) *int {
	if _, ok := args[key]; !ok {
		return nil
	}
	n := intArg(args, key, 0)
	return &n
}
