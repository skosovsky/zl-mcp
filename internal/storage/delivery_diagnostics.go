package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strconv"
)

// DeliveryReference is stable across retries/restarts of this state, without
// exposing its row ID, subscription, event ID, callback or private payload.
func (s *Store) DeliveryReference(id int64) string {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte("delivery-diagnostics:" + strconv.FormatInt(id, 10)))
	return "dlv_" + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

const (
	deliveryReasonAccepted = "accepted"
	deliveryReasonCapacity = "queue_capacity"
	deliveryReasonDeadline = "deadline"
	deliveryReasonRevoked  = "access_revoked"
)

func safeDeliveryReason(reason string) string {
	switch reason {
	case deliveryReasonAccepted, deliveryReasonCapacity, deliveryReasonDeadline, "attempts", "payload_too_large",
		"http_rejected", "http_transient", "network", "subscription_inactive",
		deliveryReasonRevoked, "callback_gone", "profile_removed":
		return reason
	default:
		return stateUnknownText
	}
}
