package collector

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/skosovsky/zl-mcp/docs/contracts"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"github.com/skosovsky/zl-mcp/internal/storage"
)

type preloadPage func(context.Context) (domain.PreloadSnapshot, error)

func (f preloadPage) ConversationPreload(ctx context.Context) (domain.PreloadSnapshot, error) {
	return f(ctx)
}

func TestPreloadRefreshOnlyMergesPermittedMetadataAndKeepsFailureEvidenceSafe(t *testing.T) {
	// Arrange
	ctx := context.Background()
	ref := domain.ConversationRef{Type: "direct", ID: "allowed"}
	store, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{Selected: map[domain.ConversationRef]bool{ref: true}}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.BindAccount(ctx, "synthetic-owner"); err != nil {
		t.Fatal(err)
	}
	source := preloadPage(func(context.Context) (domain.PreloadSnapshot, error) {
		return domain.PreloadSnapshot{Entries: []domain.PreloadEntry{{Conversation: ref}, {Conversation: domain.ConversationRef{Type: "direct", ID: "excluded"}}}, Messages: []domain.Message{{Conversation: ref, ID: "source-message", Text: "private-source-body"}}}, nil
	})
	// Act
	err = refreshPreloadCatalog(ctx, store, source)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.PreloadStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status["status"] != "observed" || status["observed_count"] != float64(2) || status["permitted_count"] != float64(1) || status["catalog_complete"] != false || status["messages_imported"] != false {
		t.Fatal("false catalogue evidence")
	}
	var count int
	if err = store.DB.QueryRow("SELECT count(*) FROM conversations").Scan(&count); err != nil || count != 1 {
		t.Fatal("selected policy expanded")
	}
	for _, table := range []string{"messages", "message_identities", "peer_first_incoming", "message_events", "event_deliveries", "send_operations", "event_subscriptions"} {
		if err = store.DB.QueryRow("SELECT count(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatal("metadata refresh changed " + table)
		}
	}
	success := status["last_success_at"]
	// Act: failed refresh keeps previous metadata and success time.
	err = refreshPreloadCatalog(ctx, store, preloadPage(func(context.Context) (domain.PreloadSnapshot, error) {
		return domain.PreloadSnapshot{}, errors.New("private-error-and-token-marker")
	}))
	// Assert
	if err == nil {
		t.Fatal("source failure hidden")
	}
	status, err = store.PreloadStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status["status"] != "partial" || status["stop_reason"] != "upstream_unavailable" || status["last_success_at"] != success {
		t.Fatal("failure overwrote previous evidence")
	}
	body, _ := json.Marshal(status)
	if strings.Contains(string(body), "private-") {
		t.Fatal("diagnostic leaked source")
	}
	schema, err := contracts.Compile("catalog_diagnostics", "output")
	if err != nil {
		t.Fatal(err)
	}
	outer, err := store.ContactStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	outer["preload"] = status
	if err = schema.Validate(outer); err != nil {
		t.Fatal(err)
	}
	if err = store.DB.QueryRow("SELECT count(*) FROM conversations").Scan(&count); err != nil || count != 1 {
		t.Fatal("failure deleted previous metadata")
	}
}

func TestPreloadRefreshRecordsTypedAuthAndUnsupportedStates(t *testing.T) {
	for _, tc := range []struct {
		source        error
		state, reason string
	}{{domain.ErrAuthenticationRequired, "partial", "auth_required"}, {domain.ErrConversationPreloadUnsupported, "unsupported", "source_unsupported"}, {domain.NewHistoryPageFailure("invalid preload metadata"), "partial", "invalid_source_page"}} {
		// Arrange
		ctx := context.Background()
		store, err := storage.OpenWithPolicy(ctx, filepath.Join(t.TempDir(), "messages.sqlite"), domain.CollectionPolicy{All: true}, 90)
		if err != nil {
			t.Fatal(err)
		}
		// Act
		err = refreshPreloadCatalog(ctx, store, preloadPage(func(context.Context) (domain.PreloadSnapshot, error) { return domain.PreloadSnapshot{}, tc.source }))
		// Assert
		if !errors.Is(err, tc.source) {
			t.Fatal("typed failure lost")
		}
		status, err := store.PreloadStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if status["status"] != tc.state || status["stop_reason"] != tc.reason {
			t.Fatal("source failure misclassified")
		}
		store.Close()
	}
}
