package evalfixture

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
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/toolruntime"
)

func fixtureReadPorts(owner *storage.Store, backend operationsadapter.Backend) mcpserver.ReadPorts {
	reader := storeadapter.New(owner)
	operations := operationsadapter.NewReader(backend)
	return mcpserver.ReadPorts{Archives: &archives.Handler{Reads: operationsadapter.NewArchiveReader(backend)}, Search: &search.Handler{Reads: reader}, Conversations: &conversations.Handler{Reads: reader, Catalog: reader, DirectoryReads: reader, LiveGroups: operations}, Authority: reader, Admission: reader, Events: &eventaccess.Handler{Diagnostics: reader, Journal: reader, Commands: reader}, Operations: toolruntime.OperationReads{Messaging: &messaging.Handler{Reads: operations, Commands: operations}, History: &history.Handler{Reads: operations, Commands: operations}, Membership: &membership.Handler{Reads: operations, Commands: operations}}}
}
