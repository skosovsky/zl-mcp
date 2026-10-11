package shared

// ConversationInfo preserves optional metadata presence: empty aliases and
// explicit false are distinct from fields omitted by an archive catalogue.
type ConversationInfo struct {
	ConversationType  string    `json:"conversation_type"`
	ConversationID    string    `json:"conversation_id"`
	Name              *string   `json:"name"`
	MetadataSource    string    `json:"metadata_source"`
	Availability      string    `json:"availability"`
	FirstDiscoveredAt string    `json:"first_discovered_at"`
	UpdatedAt         string    `json:"updated_at"`
	CollectionEnabled bool      `json:"collection_enabled"`
	Aliases           *[]string `json:"aliases,omitempty"`
	Friendship        *string   `json:"friendship,omitempty"`
	HasStoredMessages *bool     `json:"has_stored_messages,omitempty"`
	MetadataSources   *[]string `json:"metadata_sources,omitempty"`
}
type GroupCoverageSummary struct {
	GroupCount          int  `json:"group_count"`
	HistoryComplete     bool `json:"history_complete"`
	GroupsWithKnownGaps int  `json:"groups_with_known_gaps"`
}
