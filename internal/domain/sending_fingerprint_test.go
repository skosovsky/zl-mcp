package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestSendFingerprintPreservesSerializedKeysAndNull(t *testing.T) {
	// Arrange: frozen serialized forms used by the existing idempotency contract.
	empty := ""
	cases := []struct {
		name  string
		reply *string
		wire  string
	}{
		{"missing reply", nil, `{"Recipient":"9007199254740993","Text":"cà phê \u003c\u0026\u003e","Reply":null}`},
		{"explicit empty reply", &empty, `{"Recipient":"9007199254740993","Text":"cà phê \u003c\u0026\u003e","Reply":""}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			request := SendRequest{RecipientID: "9007199254740993", Text: "cà phê <&>", ReplyTo: tc.reply}
			expected := sha256.Sum256([]byte(tc.wire))
			// Act.
			got := request.Fingerprint()
			// Assert: tags must not rename keys, omit null or alter numeric-string/Unicode bytes.
			if got != hex.EncodeToString(expected[:]) {
				t.Fatalf("fingerprint changed: %s", got)
			}
		})
	}
}
