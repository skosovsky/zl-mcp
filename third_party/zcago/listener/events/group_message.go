package events

import (
	"encoding/json"
	"github.com/amrakk/zcago/errs"

	"github.com/amrakk/zcago/model"
)

type GroupMessageEventData struct {
	GroupMsgs []groupMessageOrUndo `json:"groupMsgs"`
}

type groupMessageOrUndo struct {
	Message *model.TGroupMessage
	Undo    *model.TUndo
}

func (m *groupMessageOrUndo) UnmarshalJSON(data []byte) error {
	m.Message, m.Undo = nil, nil
	var tu model.TUndo
	if err := json.Unmarshal(data, &tu); err == nil && tu.MsgID != "" && tu.MsgType == "chat.undo" {
		m.Undo = &tu
		return nil
	}
	var tm model.TGroupMessage
	if err := json.Unmarshal(data, &tm); err != nil {
		return err
	}
	if tm.MsgID != "" {
		m.Message = &tm
		return nil
	}

	return errs.NewZCA("group message has no message identifier", "groupMessageOrUndo.UnmarshalJSON")
}
