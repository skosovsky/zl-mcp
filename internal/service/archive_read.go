package service

import (
	"context"
	"encoding/json"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/mobilebackup"
)

// Archive reads are offline operations owned by the persistent service.
func (p *membershipPort) archiveRead(ctx context.Context, method string, args any) (map[string]any, error) {
	if method == "read_archive_resource" {
		return p.archiveResource(ctx, args)
	}
	schema, err := contracts.Compile(method, "input")
	var input map[string]any
	encoded, marshalErr := json.Marshal(args)
	if err != nil || marshalErr != nil || json.Unmarshal(encoded, &input) != nil || schema.Validate(input) != nil {
		return nil, domain.Invalid("Archive arguments do not match the tool contract.")
	}
	if p.library == nil || p.store == nil {
		return nil, archiveReadUnavailable()
	}
	binding, err := p.store.ArchiveAccountKey(ctx)
	if err != nil {
		return nil, archiveReadUnavailable()
	}
	if method == "zalo_list_conversations" {
		return p.archiveConversations(ctx, input, binding)
	}
	if method == "zalo_list_conversation_messages" {
		return p.archiveMessages(ctx, input, binding)
	}
	if method != "zalo_list_archive_sources" {
		return nil, domain.Invalid("Unknown archive read operation.")
	}
	sources, err := p.library.Sources(ctx, binding)
	if err != nil {
		return nil, archiveReadUnavailable()
	}
	values := make([]map[string]any, 0, len(sources))
	for _, receipt := range sources {
		values = append(values, archiveDescriptor(receipt))
	}
	return map[string]any{"sources": values, "history_complete": false}, nil
}

func archiveDescriptor(receipt mobilebackup.PreservationReceipt) map[string]any {
	source := receipt.Source
	return map[string]any{
		"source_id": source.SourceID, "captured_at": source.CapturedAt,
		"capture_cache_expires_at": source.ExpiresAt, "preserved_at": receipt.PreservedAt,
		"retention": receipt.Retention, "digest": source.Digest, "file_count": source.FileCount,
		"direct_files": source.DirectFiles, "group_files": source.GroupFiles,
		"history_complete": false, "read_scope": "collection_policy",
	}
}

func archiveReadUnavailable() error {
	return &domain.Error{Code: "SOURCE_UNAVAILABLE", Message: "Authenticated local archive storage is unavailable.", Details: map[string]any{}}
}
