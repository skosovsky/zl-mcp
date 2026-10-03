package zalo

import (
	"github.com/amrakk/zcago/model"
	"testing"
)

func TestContactMetadataOnlyUsesDirectoryFields(t *testing.T) {
	// Arrange
	user := model.User{UserId: "peer", DisplayName: "Display", ZaloName: "Alias", PhoneNumber: "private", Avatar: "private", IsFr: 1}
	// Act
	c := contactMetadata(user)
	// Assert
	if c.ID != "peer" || c.Name != "Display" || c.Friendship != "friend" || len(c.Aliases) != 2 || c.Aliases[1] != "Alias" {
		t.Fatal("directory mapping lost")
	}
	// Act / Assert: missing zero-value relationship fields must remain unknown.
	user.DisplayName = ""
	user.IsFr = 0
	c = contactMetadata(user)
	if c.Name != "Alias" || c.Friendship != "unknown" {
		t.Fatal("zero-value profile misclassified")
	}
}
