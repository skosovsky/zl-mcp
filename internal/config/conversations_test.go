package config

import (
	"github.com/skosovsky/zl-mcp/internal/domain"
	"testing"
)

func TestCollectionConfigPreservesLegacyAndRejectsConflicts(t *testing.T) {
	// Arrange
	var c Config
	c.Collection.GroupIDs = []string{"same"}
	// Act / Assert
	if err := c.validateCollection(); err != nil {
		t.Fatal(err)
	}
	if !c.Allowed("same") || c.Policy().Allows(domain.ConversationRef{Type: domain.ConversationDirect, ID: "same"}) {
		t.Fatal("legacy policy widened")
	}
	c.Collection.Mode = "all"
	if c.validateCollection() == nil {
		t.Fatal("legacy and new policy combined")
	}
	c.Collection.GroupIDs = nil
	if err := c.validateCollection(); err != nil {
		t.Fatal(err)
	}
	if !c.Policy().Allows(domain.ConversationRef{Type: domain.ConversationDirect, ID: "new"}) {
		t.Fatal("all does not admit new direct chat")
	}
	c.Collection.Mode = "selected"
	if c.validateCollection() == nil {
		t.Fatal("selected omitted explicit list")
	}
	c.Collection.Conversations = []domain.ConversationRef{}
	if err := c.validateCollection(); err != nil {
		t.Fatal(err)
	}
	c.Collection.Conversations = []domain.ConversationRef{{Type: domain.ConversationDirect, ID: "x"}, {Type: domain.ConversationDirect, ID: "x"}}
	if c.validateCollection() == nil {
		t.Fatal("duplicate selected reference accepted")
	}
}
