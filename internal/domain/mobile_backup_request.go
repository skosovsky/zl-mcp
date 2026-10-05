package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type MobileBackupRequest struct {
	RequestID        string `json:"request_id"`
	ConversationType string `json:"conversation_type"`
	ConversationID   string `json:"conversation_id"`
	Since            string `json:"since"`
	Until            string `json:"until"`
	MaxMessages      int    `json:"max_messages"`
	MaxArchiveBytes  int64  `json:"max_archive_bytes"`
}

func (r MobileBackupRequest) Ref() ConversationRef {
	return ConversationRef{Type: r.ConversationType, ID: r.ConversationID}
}
func (r MobileBackupRequest) Normalize() (MobileBackupRequest, error) {
	h, err := (HistoryImportRequest{RequestID: r.RequestID, ConversationType: r.ConversationType, ConversationID: r.ConversationID, Since: r.Since, Until: r.Until, MaxMessages: r.MaxMessages}).Normalize()
	if err != nil {
		return r, err
	}
	r.RequestID, r.Since, r.Until, r.MaxMessages = h.RequestID, h.Since, h.Until, h.MaxMessages
	if r.MaxArchiveBytes == 0 {
		r.MaxArchiveBytes = 64 << 20
	}
	if r.MaxArchiveBytes < 1 || r.MaxArchiveBytes > 512<<20 {
		return r, Invalid("Mobile archive byte budget exceeds the contract.")
	}
	return r, nil
}
func (r MobileBackupRequest) Fingerprint() string {
	r.RequestID = ""
	b, _ := json.Marshal(r)
	h := sha256.Sum256(append([]byte("mobile_backup:"), b...))
	return hex.EncodeToString(h[:])
}
