package storage

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

// TestLegacyRuntimeReferenceCompatibility is an opt-in cross-version check. The
// reference must be an isolated archive of the documented pre-rich revision.
func TestLegacyRuntimeReferenceCompatibility(t *testing.T) {
	root := os.Getenv("ZL_ROLLBACK_REFERENCE_ROOT")
	if root == "" {
		t.Skip("set ZL_ROLLBACK_REFERENCE_ROOT to an isolated pre-rich checkout")
	}
	allowedRoot := isolatedReferenceRoot(root)
	if !filepath.IsAbs(root) || !allowedRoot {
		t.Fatal("reference must be an isolated absolute temporary path")
	}
	source := filepath.Join(root, "internal/storage/zz_rich_rollback_reference_test.go")
	if _, err := os.Stat(source); !os.IsNotExist(err) {
		t.Fatal("reference test path already exists")
	}
	if err := os.WriteFile(source, []byte(legacyRollbackReferenceTest), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(source) })
	safe := filepath.Join(t.TempDir(), "safe.sqlite")
	unsafe := filepath.Join(t.TempDir(), "unsafe.sqlite")
	prepareRollbackReference(t, safe, false)
	prepareRollbackReference(t, unsafe, true)
	// Act: run the actual old encoder/store/fanout/ack code on new-schema fixtures.
	command := exec.CommandContext(
		t.Context(),
		"go",
		"test",
		"./internal/storage",
		"-run",
		"^TestRichRollbackReference$",
		"-count=1",
	)
	command.Dir = root
	command.Env = append(os.Environ(), "ZL_REFERENCE_SAFE_DB="+safe, "ZL_REFERENCE_UNSAFE_DB="+unsafe)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("reference failed: %s", output)
	}
	// Assert: new code still reads metadata/identities and old processing receipts.
	s, err := OpenWithPolicy(t.Context(), safe, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	m, err := s.ConversationMessage(t.Context(), domain.ConversationRef{Type: "direct", ID: "peer"}, "before")
	if err != nil {
		t.Fatal(err)
	}
	if m.Content == nil || m.Content.TextRole != "none" || m.Content.Kind != "image" {
		t.Fatal("old runtime erased rich metadata")
	}
	var rows int
	if err := s.DB.QueryRow(`SELECT count(*) FROM activity_observations WHERE expired=1 AND payload_json=''`).
		Scan(&rows); err != nil ||
		rows != 1 {
		t.Fatal("old runtime erased activity source", rows, err)
	}
	if report, err := CheckLegacyRollback(t.Context(), s.DB); err != nil || !report.Compatible {
		t.Fatal("old runtime changed supported processing", report, err)
	}
	var alias string
	if err := s.DB.QueryRow(`SELECT client_message_id FROM message_client_ids p JOIN messages m ON m.seq=p.message_seq WHERE m.message_id='before'`).
		Scan(&alias); err != nil ||
		alias != "123" {
		t.Fatal("old runtime erased private alias", err)
	}
	// Old writers do not know the new alias table. Their quote metadata must still
	// resolve a new observation after the candidate is restored.
	a := clientReaction()
	a.ReactionClientMessageIDs = []string{"456"}
	if err := s.PutActivity(t.Context(), a); err != nil {
		t.Fatal(err)
	}
	var body string
	if err := s.DB.QueryRow(`SELECT payload_json FROM activity_observations WHERE expired=0`).Scan(&body); err != nil {
		t.Fatal(err)
	}
	var resolved domain.Activity
	if err := json.Unmarshal([]byte(body), &resolved); err != nil || len(resolved.Reaction.TargetMessageIDs) != 1 ||
		resolved.Reaction.TargetMessageIDs[0] != "old-quoted" {
		t.Fatal("old writer quote fallback lost after upgrade", err)
	}
}

// The reference checkout is supplied by the caller, outside this test's TempDir.
// Require a real descendant rather than accepting a textual directory prefix.
func isolatedReferenceRoot(root string) bool {
	for _, base := range []string{os.TempDir(), "/tmp"} {
		rel, err := filepath.Rel(base, root)
		if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func prepareRollbackReference(t *testing.T, path string, newProfile bool) {
	t.Helper()
	ctx := context.Background()
	s, err := OpenWithPolicy(ctx, path, domain.CollectionPolicy{All: true}, 90)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	profile := domain.ConversationMessageCreatedV2
	if newProfile {
		profile = domain.ConversationMessageCreatedV3
	}
	sub := EventSubscription{
		ID:               "sub",
		Principal:        "owner",
		Profile:          profile,
		Scope:            "direct",
		ConversationType: "direct",
		Callback:         "https://receiver.example",
		Secret:           "synthetic",
	}
	revision, err := s.SubscriptionRevision(ctx, sub.ID)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Now().UTC()
	if _, err = s.ActivateSubscription(ctx, sub, revision, at); err != nil {
		t.Fatal(err)
	}
	content := domain.LegacyContent()
	content.Kind = "image"
	content.TextRole = "none"
	if err = s.Put(
		ctx,
		domain.Message{
			Conversation:            domain.ConversationRef{Type: "direct", ID: "peer"},
			ID:                      "before",
			SenderID:                "peer",
			SentAt:                  at,
			Text:                    "legacy title",
			Direction:               "incoming",
			Source:                  "live",
			Content:                 content,
			ProtocolClientMessageID: "123",
		},
	); err != nil {
		t.Fatal(err)
	}
	if !newProfile {
		activity := domain.Activity{
			Family:           "reaction",
			SchemaVersion:    1,
			ConversationType: "direct",
			ConversationID:   "peer",
			ActorID:          "peer",
			OccurredAt:       at,
			Direction:        "incoming",
			SourceID:         "before",
			IdentityBasis:    "upstream_action_id",
			Source:           "live",
			Reaction:         &domain.ReactionObservation{TargetMessageIDs: []string{}, Action: "unknown"},
		}
		if err = s.PutActivity(ctx, activity); err != nil {
			t.Fatal(err)
		}
		if _, err = s.FanoutActivityEvents(
			ctx,
			at,
			DefaultDeliveryPolicy(),
			func(_, _ string, _ domain.Activity) ([]byte, error) {
				t.Fatal("no reaction subscription")
				return nil, nil
			},
		); err != nil {
			t.Fatal(err)
		}
		// Natural expiry in disposable time leaves only dedup tombstones before downgrade.
		if err = s.PruneActivities(ctx, at.Add(91*24*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	report, err := CheckLegacyRollback(ctx, s.DB)
	if err != nil || report.Compatible == newProfile {
		t.Fatal("rollback guard failed before reference", report, err)
	}
}

const legacyRollbackReferenceTest = `package storage_test
import("context";"os";"testing";"time";"github.com/skosovsky/zl-mcp/internal/domain";"github.com/skosovsky/zl-mcp/internal/events";"github.com/skosovsky/zl-mcp/internal/storage")
func TestRichRollbackReference(t *testing.T){
 ctx:=context.Background();at:=time.Now().UTC()
 encoder,err:=events.NewMessageEncoder();if err!=nil{t.Fatal(err)}
 s,err:=storage.OpenWithPolicy(ctx,os.Getenv("ZL_REFERENCE_SAFE_DB"),domain.CollectionPolicy{All:true},90);if err!=nil{t.Fatal(err)}
 if n,err:=s.FanoutProfileEvents(ctx,at,storage.DefaultDeliveryPolicy(),encoder.EncodeProfile);err!=nil||n!=1{t.Fatal("old fanout",n,err)}
 d,err:=s.ClaimDelivery(ctx,at,storage.DefaultDeliveryPolicy());if err!=nil||d==nil{t.Fatal("old claim",err)}
 if err:=s.FinishDelivery(ctx,*d,storage.DeliveryOutcome{State:"delivered",Reason:"accepted"},at);err!=nil{t.Fatal(err)}
 page,err:=s.ReadSubscriptionEvents(ctx,"owner","sub",20);if err!=nil{t.Fatal(err)}
 entry:=page["events"].([]map[string]any)[0]
 if _,err:=s.AckSubscriptionEvents(ctx,"owner","sub",entry["receipt"].(string));err!=nil{t.Fatal(err)}
 if err:=s.Put(ctx,domain.Message{Conversation:domain.ConversationRef{Type:"direct",ID:"peer"},ID:"old-writer",SenderID:"peer",SentAt:at,Text:"old text",Direction:"incoming",Source:"live"});err!=nil{t.Fatal(err)}
 if _,err:=s.DB.Exec("INSERT INTO message_client_ids SELECT seq,'789' FROM messages WHERE message_id='old-writer'");err!=nil{t.Fatal(err)}
 if err:=s.DeleteConversation(ctx,domain.ConversationRef{Type:"direct",ID:"peer"},"old-writer");err!=nil{t.Fatal(err)}
 var aliases int
 if err:=s.DB.QueryRow("SELECT count(*) FROM message_client_ids WHERE client_message_id='789'").Scan(&aliases);err!=nil||aliases!=0{t.Fatal("old runtime FK cascade failed",aliases,err)}
 if err:=s.Put(ctx,domain.Message{Conversation:domain.ConversationRef{Type:"direct",ID:"peer"},ID:"old-quoted",SenderID:"peer",SentAt:at,Text:"old quote text",Direction:"incoming",Source:"live",QuoteMetadata:&domain.QuoteMetadata{ClientMessageID:"456",MessageType:"webchat",Timestamp:"1"}});err!=nil{t.Fatal(err)}
 if err:=s.Close();err!=nil{t.Fatal(err)}
 // Deliberate negative control on a disposable fixture: demonstrate why guard must reject v3.
 bad,err:=storage.OpenWithPolicy(ctx,os.Getenv("ZL_REFERENCE_UNSAFE_DB"),domain.CollectionPolicy{All:true},90);if err!=nil{t.Fatal(err)};defer bad.Close()
 if _,err:=bad.FanoutProfileEvents(ctx,at,storage.DefaultDeliveryPolicy(),encoder.EncodeProfile);err!=nil{t.Fatal(err)}
 var failed int
 if err:=bad.DB.QueryRow("SELECT count(*) FROM event_deliveries WHERE state='failed' AND last_reason='invalid_payload'").Scan(&failed);err!=nil||failed!=1{t.Fatal("negative control did not reproduce unsupported profile consumption",failed,err)}
}
`
