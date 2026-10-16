package domain

// ContentShape is bounded diagnostic metadata. Public message JSON and
// persistence exclude it; it contains no native param values, text or URLs.
type ContentShape struct {
	ActionPresent           bool                         `json:"action_present"`
	TypePresent             bool                         `json:"type_present"`
	UnknownKeyCount         int                          `json:"unknown_key_count"`
	ActionToken             *string                      `json:"action_token"`
	TypeToken               *string                      `json:"type_token"`
	DescriptionPresent      bool                         `json:"description_present"`
	ParamsShape             string                       `json:"params_shape"`
	ParamsKeys              []string                     `json:"params_keys"`
	KeysTruncated           bool                         `json:"keys_truncated"`
	ParamShapes             map[string]ContentParamShape `json:"param_shapes,omitempty"`
	DescriptionMatchesTitle bool                         `json:"description_matches_title"`
	ParamDescMatchesTitle   bool                         `json:"param_desc_matches_title"`
}

// ContentParamShape reveals a JSON type and emptiness, never its value or size.
type ContentParamShape struct {
	Type  string `json:"type"`
	Empty *bool  `json:"empty"`
}
