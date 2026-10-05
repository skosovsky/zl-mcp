package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"github.com/amrakk/zcago/model"
)

var ErrMobileSyncControl = errors.New("invalid mobile sync control")

func decodeMobileSync(action string, payload []byte) (*model.MobileSyncEvent, error) {
	if action != "user_confirm" && action != "syncmsg_info" && action != "transfer_error" {
		return nil, nil
	}
	if len(payload) > 64<<10 {
		return nil, ErrMobileSyncControl
	}
	var w struct {
		PublicKey    string          `json:"public_key"`
		PCName       string          `json:"pc_name"`
		UserAction   *int            `json:"user_action"`
		UID          json.RawMessage `json:"uid"`
		FromSequence json.RawMessage `json:"from_seq_id"`
		URL          string          `json:"url"`
		EncryptedKey string          `json:"encrypted_key"`
		FileSize     json.RawMessage `json:"file_size"`
		DatabaseInfo json.RawMessage `json:"db_info"`
		ErrorCode    *int            `json:"error_code"`
		Status       *int            `json:"status"`
	}
	if json.Unmarshal(payload, &w) != nil || w.PublicKey == "" || len(w.PublicKey) > 4096 || len(w.PCName) > 128 {
		return nil, ErrMobileSyncControl
	}
	e := &model.MobileSyncEvent{Action: action, PublicKey: w.PublicKey, PCName: w.PCName, UserAction: w.UserAction, ErrorCode: w.ErrorCode, Status: w.Status}
	switch action {
	case "user_confirm":
		if w.PCName == "" || w.UserAction == nil || *w.UserAction < 0 || *w.UserAction > 3 {
			return nil, ErrMobileSyncControl
		}
	case "transfer_error":
		if w.PCName == "" || w.ErrorCode == nil && w.Status == nil {
			return nil, ErrMobileSyncControl
		}
	case "syncmsg_info":
		uid, ok := mobileDecimal(w.UID)
		if !ok || uid == "0" {
			return nil, ErrMobileSyncControl
		}
		seq, ok := mobileDecimal(w.FromSequence)
		if !ok {
			return nil, ErrMobileSyncControl
		}
		size, ok := mobileDecimal(w.FileSize)
		if !ok {
			return nil, ErrMobileSyncControl
		}
		n, _ := strconv.ParseUint(size, 10, 64)
		if n == 0 || n > 512<<20 {
			return nil, ErrMobileSyncControl
		}
		u, err := url.Parse(w.URL)
		if err != nil || len(w.URL) > 4096 || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" || w.EncryptedKey == "" || len(w.EncryptedKey) > 4096 {
			return nil, ErrMobileSyncControl
		}
		db := bytes.TrimSpace(w.DatabaseInfo)
		if len(db) == 0 || len(db) > 16<<10 {
			return nil, ErrMobileSyncControl
		}
		var info string
		if db[0] == '"' {
			if json.Unmarshal(db, &info) != nil || len(info) > 16<<10 || !json.Valid([]byte(info)) || len(bytes.TrimSpace([]byte(info))) == 0 || bytes.TrimSpace([]byte(info))[0] != '{' {
				return nil, ErrMobileSyncControl
			}
		} else if db[0] == '{' {
			info = string(db)
		} else {
			return nil, ErrMobileSyncControl
		}
		e.UID, e.FromSequence, e.FileSize, e.URL, e.EncryptedKey, e.DatabaseInfo = uid, seq, n, w.URL, w.EncryptedKey, info
	}
	return e, nil
}

func mobileDecimal(raw []byte) (string, bool) {
	raw = bytes.TrimSpace(raw)
	var s string
	if len(raw) > 0 && raw[0] == '"' {
		if json.Unmarshal(raw, &s) != nil {
			return "", false
		}
	} else {
		s = string(raw)
	}
	if s == "" || len(s) > 20 || len(s) > 1 && s[0] == '0' {
		return "", false
	}
	for _, b := range s {
		if b < '0' || b > '9' {
			return "", false
		}
	}
	_, err := strconv.ParseUint(s, 10, 64)
	return s, err == nil
}
