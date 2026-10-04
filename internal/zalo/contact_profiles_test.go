package zalo

import (
	"context"
	"github.com/amrakk/zcago"
	"testing"

	"github.com/amrakk/zcago/api"
	"github.com/amrakk/zcago/model"
)

func TestKnownProfileMetadataAndUnchangedOmissions(t *testing.T) {
	// Arrange
	response := &api.GetUserInfoResponse{ChangedProfiles: map[string]model.User{"peer_0": {DisplayName: "Synthetic", ZaloName: "Alias", IsFr: 1, PhoneNumber: "private", Avatar: "private", Dob: 1}}, UnchangedProfiles: map[string]any{"missing": "version"}}
	// Act
	contacts, err := profileContacts([]string{"peer", "missing"}, response)
	// Assert
	if err != nil {
		t.Fatal(err)
	}
	if len(contacts) != 1 || contacts[0].ID != "peer" || contacts[0].Name != "Synthetic" || contacts[0].Friendship != "friend" || len(contacts[0].Aliases) != 2 {
		t.Fatal("known profile identity or metadata lost")
	}
}

func TestKnownProfilesRejectForeignConflictingAndDuplicateIDs(t *testing.T) {
	for _, records := range []map[string]model.User{
		{"foreign": {UserId: "foreign"}},
		{"peer": {UserId: "foreign"}},
		{"peer": {UserId: "peer"}, "peer_0": {UserId: "peer"}},
	} {
		// Arrange / Act
		contacts, err := profileContacts([]string{"peer"}, &api.GetUserInfoResponse{ChangedProfiles: records})
		// Assert
		if err == nil || contacts != nil {
			t.Fatal("unrequested/ambiguous identity accepted")
		}
	}
	// Arrange / Act / Assert: never interpret invalid ID requests as enumeration.
	for _, ids := range [][]string{nil, {""}, {"peer", "peer"}, {"peer", "peer_0"}, make([]string, 101)} {
		if err := validateProfileIDs(ids); err == nil {
			t.Fatal("invalid profile request accepted")
		}
	}
}

// Embedding the SDK interface avoids implementing unrelated endpoint types.
type profileAPIStub struct {
	zcago.API
	response *api.GetUserInfoResponse
	calls    int
}

func (s *profileAPIStub) GetUserInfo(ctx context.Context, ids ...string) (*api.GetUserInfoResponse, error) {
	s.calls++
	for i := range ids {
		ids[i] += "_0"
	}
	return s.response, nil
}

func TestProfileReadProtectsCallerIDsFromSDKMutation(t *testing.T) {
	// Arrange
	source := &profileAPIStub{response: &api.GetUserInfoResponse{ChangedProfiles: map[string]model.User{"peer_0": {UserId: "peer", DisplayName: "Synthetic"}}}}
	c := &Client{api: source}
	ids := []string{"peer"}
	// Act
	contacts, err := c.ContactProfiles(context.Background(), ids)
	// Assert
	if err != nil || len(contacts) != 1 || ids[0] != "peer" || source.calls != 1 {
		t.Fatal("SDK mutated caller identity", err)
	}
	// Act / Assert: ambiguity is rejected before issuing a request.
	_, err = c.ContactProfiles(context.Background(), []string{"peer", "peer_0"})
	if err == nil || source.calls != 1 {
		t.Fatal("ambiguous IDs crossed profile boundary")
	}
}
