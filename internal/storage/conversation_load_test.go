package storage

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func BenchmarkManyConversations(b *testing.B) {
	// Arrange: exercise real persistence, discovery, FTS and gaps for 2,000 peers
	// and 2,000 groups with colliding IDs, each holding two synthetic messages.
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, filepath.Join(b.TempDir(), "corpus.sqlite"), domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	at := time.Now().UTC()
	start := time.Now()
	for n := range 2000 {
		for _, kind := range []string{"direct", "group"} {
			ref := domain.ConversationRef{Type: kind, ID: fmt.Sprintf("peer-%05d", n)}
			for m := range 2 {
				if err := s.Put(ctx, domain.Message{Conversation: ref, ID: fmt.Sprint(m), SenderID: "synthetic-author", SentAt: at.Add(time.Duration(m) * time.Second), Text: "synthetic corpus searchable message", Source: "replay"}); err != nil {
					b.Fatal(err)
				}
			}
		}
	}
	b.Logf("persisted 8000 messages in 4000 conversations in %s", time.Since(start))
	if err := s.BeginGap(ctx, "synthetic_restart"); err != nil {
		b.Fatal(err)
	}
	var indexBytes int64
	if err := s.DB.QueryRowContext(ctx, "SELECT COALESCE(sum(pgsize),0) FROM dbstat WHERE name IN ('message_order','message_events_conversation','collection_gap_conversation')").Scan(&indexBytes); err != nil {
		b.Fatal(err)
	}
	b.Logf("typed ordering/event/gap index pages: %d bytes", indexBytes)
	b.Run("catalogue_page", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			// Act.
			page, err := s.Conversations(ctx, "", "", 50, "")
			// Assert: page allocation is bounded and pagination remains available.
			if err != nil || len(page["conversations"].([]map[string]any)) != 50 || page["has_more"] != true {
				b.Fatalf("catalogue error=%v", err)
			}
		}
	})
	b.Run("collection_status", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			// Act.
			status, err := s.CollectionStatus(ctx)
			// Assert.
			if err != nil {
				b.Fatal(err)
			}
			if status["conversation_counts"].(map[string]int)["direct"] != 2000 || status["message_counts"].(map[string]int)["group"] != 4000 {
				b.Fatal("incorrect aggregate counts")
			}
		}
	})
	b.Run("search_with_coverage", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			// Act.
			q := Search{General: true, Query: "searchable", Limit: 50}
			page, err := s.Page(ctx, q)
			if err != nil {
				b.Fatal(err)
			}
			result, err := s.ConversationSearchResult(ctx, q, page)
			// Assert.
			if err != nil || len(result["messages"].([]map[string]any)) != 50 || result["coverage_summary"].(map[string]any)["conversations_with_known_gaps"] != 4000 {
				b.Fatalf("search error=%v", err)
			}
		}
	})
	b.Run("context", func(b *testing.B) {
		b.ReportAllocs()
		for range b.N {
			// Act.
			result, err := s.ConversationContext(ctx, domain.ConversationRef{Type: "direct", ID: "peer-00000"}, "0", 5, 5)
			// Assert.
			if err != nil || len(result["after"].([]map[string]any)) != 1 {
				b.Fatalf("context error=%v", err)
			}
		}
	})
}
