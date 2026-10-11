package mcpserver

import (
	storeadapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/storage"
	"github.com/skosovsky/zl-mcp/internal/toolruntime"
)

func migrationReadPorts(owner *storage.Store) ReadPorts {
	reader := storeadapter.New(owner)
	return ReadPorts{Search: &search.Handler{Reads: reader}, Conversations: &conversations.Handler{Reads: reader}, Authority: reader}
}
func withMigrationReads(service *Service) *Service {
	reads := migrationReadPorts(service.Store)
	var err error
	service.runtime, err = toolruntime.NewReads(reads.Search, reads.Conversations, reads.Authority)
	if err != nil {
		panic(err)
	}
	service.conversations = reads.Conversations
	return service
}
