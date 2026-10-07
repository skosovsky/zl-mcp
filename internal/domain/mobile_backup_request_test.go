package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestMobileBackupAccountScopeDoesNotChangeLegacyFingerprint(t *testing.T) {
	// Arrange: exact historical serialized normal form before account scope existed.
	r := MobileBackupRequest{RequestID: "00000000-0000-4000-8000-000000000001", ConversationType: "direct", ConversationID: "12", Since: "2026-09-01T00:00:00Z", Until: "2026-10-01T00:00:00Z"}
	old := `mobile_backup:{"request_id":"","conversation_type":"direct","conversation_id":"12","since":"2026-09-01T00:00:00Z","until":"2026-10-01T00:00:00Z","max_messages":1000,"max_archive_bytes":67108864}`
	hash := sha256.Sum256([]byte(old))
	// Act: normalize the old request and an explicit whole-account request.
	legacy, err := r.Normalize()
	account, e := (MobileBackupRequest{RequestID: r.RequestID, ArchiveScope: "account"}).Normalize()
	// Assert: existing fingerprints stay byte-identical; acquisition has no fictitious chat/date.
	if err != nil || e != nil || legacy.Fingerprint() != hex.EncodeToString(hash[:]) || account.ConversationID != "" || account.Since != "" || account.MaxMessages != 0 || account.MaxArchiveBytes != 512<<20 || account.RetentionHours != 168 || account.Fingerprint() == legacy.Fingerprint() {
		t.Fatal("scope normalization changed legacy identity")
	}
	for _, bad := range []MobileBackupRequest{{RequestID: r.RequestID, ArchiveScope: "account", ConversationID: "12"}, {RequestID: r.RequestID, ArchiveScope: "account", Since: r.Since}, {RequestID: r.RequestID, ArchiveScope: "account", RetentionHours: 721}, {RequestID: r.RequestID, ArchiveScope: "unknown"}} {
		if _, e := bad.Normalize(); e == nil {
			t.Fatal("mixed acquisition scope accepted")
		}
	}
}
