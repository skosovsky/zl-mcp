package model

// MobileSyncEvent is sensitive in-memory transport data, never public MCP output.
// Individual fields must not be logged; formatted/JSON representations redact them.
type MobileSyncEvent struct {
	Action       string `json:"-"`
	PublicKey    string `json:"-"`
	PCName       string `json:"-"`
	UserAction   *int   `json:"-"`
	UID          string `json:"-"`
	FromSequence string `json:"-"`
	URL          string `json:"-"`
	EncryptedKey string `json:"-"`
	FileSize     uint64 `json:"-"`
	DatabaseInfo string `json:"-"`
	ErrorCode    *int   `json:"-"`
	Status       *int   `json:"-"`
}

func (MobileSyncEvent) String() string   { return "mobile sync control [redacted]" }
func (MobileSyncEvent) GoString() string { return "mobile sync control [redacted]" }
