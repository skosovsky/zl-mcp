package domain

const HistorySourceMobileArchive = "mobile_archive"

// MobileHistorySnapshot is a trusted private binding, never a public tool input.
type MobileHistorySnapshot struct {
	ID                                           string   `json:"-"`
	Digest                                       [32]byte `json:"-"`
	CreatedMS, ExpiresMS                         int64    `json:"-"`
	SourceRows, PeriodRows, InvalidTimestampRows int64    `json:"-"`
	HasRange                                     bool     `json:"-"`
	EarliestMS, LatestMS                         int64    `json:"-"`
	WALMode                                      bool     `json:"-"`
	ControlRows                                  int      `json:"-"`
	ControlPreludeComplete                       bool     `json:"-"`
}

func (MobileHistorySnapshot) String() string   { return "mobile history snapshot [redacted]" }
func (MobileHistorySnapshot) GoString() string { return "mobile history snapshot [redacted]" }

type MobileHistoryPosition struct {
	TimestampMS, RowID int64 `json:"-"`
}

func (MobileHistoryPosition) String() string   { return "mobile history position [redacted]" }
func (MobileHistoryPosition) GoString() string { return "mobile history position [redacted]" }

// MobileHistoryCounts separates exclusive row gaps from orthogonal metadata gaps.
type MobileHistoryCounts struct {
	Examined              int `json:"examined"`
	Rejected              int `json:"rejected"`
	Expired               int `json:"expired"`
	UnsupportedTypes      int `json:"unsupported_types"`
	UnsupportedContent    int `json:"unsupported_content"`
	MissingMetadata       int `json:"missing_metadata"`
	InvalidMetadata       int `json:"invalid_metadata"`
	DeferredControls      int `json:"deferred_controls"`
	OwnRecalls            int `json:"own_recalls"`
	UnknownMetadataFields int `json:"unknown_metadata_fields"`
	ExpiredQuotes         int `json:"expired_quotes"`
	UnresolvedQuotes      int `json:"unresolved_quotes"`
	UnresolvedMentions    int `json:"unresolved_mentions"`
}

type MobileHistoryCoverage struct {
	MobileHistoryCounts
	SourceRows             int64 `json:"source_rows"`
	PeriodRows             int64 `json:"period_rows"`
	InvalidTimestampRows   int64 `json:"invalid_timestamp_rows"`
	SourceControls         int   `json:"source_controls"`
	SourceRecalls          int   `json:"source_recalls,omitempty"`
	ControlPreludeComplete bool  `json:"control_prelude_complete,omitempty"`
}

// MobileHistoryRecall is a validated source observation, never a live recall request.
type MobileHistoryRecall struct {
	Conversation ConversationRef `json:"-"`
	MessageID    string          `json:"-"`
	SenderID     string          `json:"-"`
	RecordAtMS   int64           `json:"-"`
}

func (MobileHistoryRecall) String() string   { return "historical recall [redacted]" }
func (MobileHistoryRecall) GoString() string { return "historical recall [redacted]" }

// MobileHistoryPage carries one exact source page; persistence is always silent.
type MobileHistoryPage struct {
	ControlPrelude bool                    `json:"-"`
	Snapshot       MobileHistorySnapshot   `json:"-"`
	Records        []ExpiringHistoryRecord `json:"-"`
	Recalls        []MobileHistoryRecall   `json:"-"`
	Counts         MobileHistoryCounts     `json:"-"`
	HasMore        bool                    `json:"-"`
	Next           *MobileHistoryPosition  `json:"-"`
}

func (MobileHistoryPage) String() string   { return "mobile history page [redacted]" }
func (MobileHistoryPage) GoString() string { return "mobile history page [redacted]" }

func (p *MobileHistoryPage) Clear() {
	if p != nil {
		clear(p.Records)
		clear(p.Recalls)
		if p.Next != nil {
			*p.Next = MobileHistoryPosition{}
		}
		*p = MobileHistoryPage{}
	}
}
