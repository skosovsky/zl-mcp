package mobilebackup

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/skosovsky/zl-mcp/internal/domain"
	"io"
	"strconv"
	"strings"
)

var ErrIdentities = errors.New("invalid mobile backup identity mapping")

type IdentityRequest = domain.MobileIdentityRequest
type IdentityPair = domain.MobileIdentityPair

func canonicalIdentity(id string) bool {
	if len(id) == 0 || len(id) > 20 || id[0] == '0' {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	_, err := strconv.ParseUint(id, 10, 64)
	return err == nil
}
func validIdentityRequest(r IdentityRequest) bool {
	total := len(r.Direct) + len(r.Groups)
	if total < 1 || total > 1000 {
		return false
	}
	for _, ids := range [][]string{r.Direct, r.Groups} {
		seen := make(map[string]bool, len(ids))
		for _, id := range ids {
			if !canonicalIdentity(id) || seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	return true
}

func IdentityPayload(r IdentityRequest) ([]byte, error) {
	if !validIdentityRequest(r) {
		return nil, ErrIdentities
	}
	payload := make(map[string][]json.Number)
	for i, ids := range [][]string{r.Direct, r.Groups} {
		if len(ids) == 0 {
			continue
		}
		name := "fids"
		if i == 1 {
			name = "gids"
		}
		values := make([]json.Number, len(ids))
		for j, id := range ids {
			values[j] = json.Number(id)
		}
		payload[name] = values
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, ErrIdentities
	}
	return data, nil
}

// DecodeIdentities accepts already unwrapped plaintext only. It does not perform
// authenticated requests, infer account identity or resolve missing mappings.
func DecodeIdentities(r IdentityRequest, data []byte) ([]IdentityPair, error) {
	if !validIdentityRequest(r) || len(data) == 0 || len(data) > 256<<10 {
		return nil, ErrIdentities
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, ErrIdentities
	}
	arrays := make(map[string][]json.RawMessage, 2)
	for decoder.More() {
		token, err = decoder.Token()
		name, ok := token.(string)
		if err != nil || !ok || (name != "fids" && name != "gids") {
			return nil, ErrIdentities
		}
		if _, exists := arrays[name]; exists {
			return nil, ErrIdentities
		}
		var raw []json.RawMessage
		if decoder.Decode(&raw) != nil || raw == nil {
			return nil, ErrIdentities
		}
		arrays[name] = raw
	}
	token, err = decoder.Token()
	if err != nil || token != json.Delim('}') {
		return nil, ErrIdentities
	}
	if _, err = decoder.Token(); err != io.EOF {
		return nil, ErrIdentities
	}
	if len(arrays["fids"]) != len(r.Direct) || len(arrays["gids"]) != len(r.Groups) {
		return nil, ErrIdentities
	}
	result := make([]IdentityPair, 0, len(r.Direct)+len(r.Groups))
	for category, ids := range [][]string{r.Direct, r.Groups} {
		name := "fids"
		if category == 1 {
			name = "gids"
		}
		seen := make(map[string]bool, len(ids))
		for i, id := range ids {
			raw := arrays[name][i]
			mapped := string(raw)
			if len(raw) > 0 && raw[0] == '"' {
				if json.Unmarshal(raw, &mapped) != nil {
					return nil, ErrIdentities
				}
				if category == 1 {
					mapped = strings.TrimPrefix(mapped, "g")
				}
			}
			if !canonicalIdentity(mapped) || seen[mapped] {
				return nil, ErrIdentities
			}
			seen[mapped] = true
			result = append(result, IdentityPair{Plain: id, Session: mapped, Group: category == 1})
		}
	}
	return result, nil
}
