package mcpserver

import (
	"github.com/santhosh-tekuri/jsonschema/v6"
	operationsadapter "github.com/skosovsky/zl-mcp/internal/adapters/operations"
	storeadapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/archives"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/eventaccess"
	"github.com/skosovsky/zl-mcp/internal/app/history"
	"github.com/skosovsky/zl-mcp/internal/app/membership"
	"github.com/skosovsky/zl-mcp/internal/app/messaging"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/control"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/toolruntime"
)

func migrationReadPorts(owner *storage.Store, backend ...operationsadapter.Backend) Ports {
	reader := storeadapter.New(owner)
	ports := Ports{Search: &search.Handler{Reads: reader}, Conversations: &conversations.Handler{Reads: reader, Catalog: reader, DirectoryReads: reader}, Authority: reader, Admission: reader, Events: &eventaccess.Handler{Diagnostics: reader, Journal: reader, Commands: reader}}
	selected := operationsadapter.Backend(control.New(""))
	if len(backend) == 1 && backend[0] != nil {
		selected = backend[0]
	}
	{
		operations := operationsadapter.NewReader(selected)
		ports.Archives = &archives.Handler{Reads: operationsadapter.NewArchiveReader(selected)}
		ports.Conversations.LiveGroups = operations
		ports.Operations = toolruntime.Capabilities{Messaging: &messaging.Handler{Reads: operations, Commands: operations}, History: &history.Handler{Reads: operations, Commands: operations}, Membership: &membership.Handler{Reads: operations, Commands: operations}}
	}
	return ports
}

type migrationService struct {
	Store         *storage.Store
	Control       operationsadapter.Backend
	input, output map[string]*jsonschema.Schema
}

func withMigrationReads(fixture *migrationService) *Service {
	reads := migrationReadPorts(fixture.Store, fixture.Control)
	service := &Service{input: fixture.input, output: fixture.output}
	var err error
	service.runtime, err = toolruntime.New(reads.Search, reads.Conversations, reads.Authority, reads.capabilities())
	if err != nil {
		panic(err)
	}
	service.conversations = reads.Conversations
	service.admission = reads.Admission
	service.events = reads.Events
	service.archives = reads.Archives
	return service
}
