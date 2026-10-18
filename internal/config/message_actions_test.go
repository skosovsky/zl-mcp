package config

import (
	"testing"

	"github.com/skosovsky/zl-mcp/internal/domain"
)

func TestMessageActionPermissionsAreExplicitTypedAndIndependent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		refs    []domain.ConversationRef
		invalid bool
	}{
		{"default", nil, false},
		{"typed collision", []domain.ConversationRef{{Type: "direct", ID: "peer"}, {Type: "group", ID: "peer"}}, false},
		{"duplicate", []domain.ConversationRef{{Type: "direct", ID: "peer"}, {Type: "direct", ID: "peer"}}, true},
		{"unknown type", []domain.ConversationRef{{Type: "channel", ID: "peer"}}, true},
		{"empty ID", []domain.ConversationRef{{Type: "direct"}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange.
			var c Config
			c.Permissions.ReadReceiptChats = tc.refs
			// Act.
			err := c.validatePermissions()
			// Assert: granting receipts never grants reactions, sends or collection.
			if (err != nil) != tc.invalid {
				t.Fatal("unexpected permission validation", err)
			}
			if len(c.Permissions.ReactionChats) != 0 || c.Permissions.AllowSend || c.Policy().All ||
				len(c.Policy().Selected) != 0 {
				t.Fatal("action permission widened another capability")
			}
		})
	}
}
