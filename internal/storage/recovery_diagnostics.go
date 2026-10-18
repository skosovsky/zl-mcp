package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
)

// RecoveryDeliveryReference identifies one durable control delivery without
// disclosing a row, subscription or event ID. Its HMAC domain is separate from
// business deliveries, whose numeric IDs come from a different table.
func (s *Store) RecoveryDeliveryReference(id int64) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("recovery-delivery-diagnostics:" + strconv.FormatInt(id, 10)))
	return "rdlv_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
