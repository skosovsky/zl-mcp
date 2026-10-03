package collector

import (
	"context"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

type conversationListener interface {
	ListenConversations(context.Context, func(domain.Message) error, func(domain.ConversationRef, string) error, func() error) error
}

func listenConversations(client ListenerUpstream, ctx context.Context, message func(domain.Message) error, deletion func(domain.ConversationRef, string) error, connected func() error) error {
	if typed, ok := client.(conversationListener); ok {
		return typed.ListenConversations(ctx, message, deletion, connected)
	}
	return client.Listen(ctx, message, func(group, id string) error {
		return deletion(domain.ConversationRef{Type: domain.ConversationGroup, ID: group}, id)
	}, connected)
}

func (g *sessionGuard) ListenConversations(parent context.Context, message func(domain.Message) error, deletion func(domain.ConversationRef, string) error, connected func() error) error {
	ctx, stop, err := g.request(parent)
	if err != nil {
		return err
	}
	defer stop()
	return g.observe(listenConversations(g.upstream, ctx, message, deletion, connected))
}
