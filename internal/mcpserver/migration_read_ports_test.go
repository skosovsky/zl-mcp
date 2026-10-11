package mcpserver

import (
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

func migrationReadPorts(owner *storage.Store, backend ...ControlPort) ReadPorts {
	reader := storeadapter.New(owner)
	ports := ReadPorts{Search: &search.Handler{Reads: reader}, Conversations: &conversations.Handler{Reads: reader, Catalog: reader, DirectoryReads: reader}, Authority: reader, Admission: reader, Events: &eventaccess.Handler{Diagnostics: reader, Journal: reader}}
	selected := ControlPort(control.New(""))
	if len(backend) == 1 && backend[0] != nil {
		selected = backend[0]
	}
	{
		operations := operationsadapter.NewReader(selected)
		ports.Archives = &archives.Handler{Reads: operationsadapter.NewArchiveReader(selected)}
		ports.Conversations.LiveGroups = operations
		ports.Operations = toolruntime.OperationReads{Messaging: &messaging.Handler{Reads: operations}, History: &history.Handler{Reads: operations}, Membership: &membership.Handler{Reads: operations}}
	}
	return ports
}
func withMigrationReads(service *Service) *Service {
	reads := migrationReadPorts(service.Store, service.Control)
	var err error
	service.runtime, err = toolruntime.NewReads(reads.Search, reads.Conversations, reads.Authority, reads.operationReads())
	if err != nil {
		panic(err)
	}
	service.conversations = reads.Conversations
	service.admission = reads.Admission
	service.events = reads.Events
	service.archives = reads.Archives
	return service
}
