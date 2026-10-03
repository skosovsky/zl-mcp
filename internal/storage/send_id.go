package storage

import (
	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

func canonicalSendID(id string) (string, error) {
	u, err := uuid.Parse(id)
	if err != nil || len(id) != 36 {
		return "", domain.Invalid("request_id must be a UUID.")
	}
	return u.String(), nil
}
