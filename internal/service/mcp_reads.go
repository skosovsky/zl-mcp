package service

import (
	storeadapter "github.com/skosovsky/zl-mcp/internal/adapters/store"
	"github.com/skosovsky/zl-mcp/internal/app/conversations"
	"github.com/skosovsky/zl-mcp/internal/app/search"
	"github.com/skosovsky/zl-mcp/internal/mcpserver"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

// mcpReadPorts composes stateless capabilities around the existing borrowed owner.
func mcpReadPorts(owner *storage.Store) mcpserver.ReadPorts {
	reader := storeadapter.New(owner)
	return mcpserver.ReadPorts{Search: &search.Handler{Reads: reader}, Conversations: &conversations.Handler{Reads: reader}}
}
