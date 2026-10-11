package mcpserver

import (
	storeadapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

func migrationReadPorts(owner *storage.Store) ReadPorts {
	reader := storeadapter.New(owner)
	return ReadPorts{Search: &search.Handler{Reads: reader}, Conversations: &conversations.Handler{Reads: reader}}
}
func withMigrationReads(service *Service) *Service {
	reads := migrationReadPorts(service.Store)
	service.search = reads.Search
	service.conversations = reads.Conversations
	return service
}
