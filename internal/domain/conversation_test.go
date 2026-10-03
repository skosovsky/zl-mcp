package domain

import "testing"

func TestCollectionPolicySeparatesNamespacesAndDiscoversNewPeers(t *testing.T) {
	// Arrange
	group := ConversationRef{Type: ConversationGroup, ID: "same"}
	direct := ConversationRef{Type: ConversationDirect, ID: "same"}
	selected := CollectionPolicy{Selected: map[ConversationRef]bool{group: true}}
	all := CollectionPolicy{All: true}
	// Act / Assert
	if !selected.Allows(group) || selected.Allows(direct) || !all.Allows(direct) || !all.Allows(ConversationRef{Type: ConversationDirect, ID: "new-peer"}) {
		t.Fatal("policy crossed namespace or failed dynamic discovery")
	}
	if all.Allows(ConversationRef{Type: "unsupported", ID: "x"}) || all.Allows(ConversationRef{Type: ConversationDirect}) {
		t.Fatal("invalid reference admitted")
	}
}
