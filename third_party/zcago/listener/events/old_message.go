package events

import (
	"encoding/json"
	"github.com/amrakk/zcago/model"
)

type OldMessagesEventData struct {
	Msgs         []model.TMessage      `json:"msgs"`
	GroupMsgs    []model.TGroupMessage `json:"groupMsgs"`
	More         json.RawMessage       `json:"more"`
	LastActionID json.RawMessage       `json:"lastActionId"`
}
