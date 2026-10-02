package service

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"github.com/skosovsky/zl-mcp/internal/local"
)

func tokenFile(path string) (string, error) {
	b, err := local.ReadPrivate(path)
	if errors.Is(err, os.ErrNotExist) {
		random := make([]byte, 32)
		if _, err = rand.Read(random); err != nil {
			return "", err
		}
		b = []byte(hex.EncodeToString(random))
		f, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if errors.Is(createErr, os.ErrExist) {
			return tokenFile(path)
		}
		if createErr != nil {
			return "", createErr
		}
		_, err = f.Write(b)
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			return "", err
		}
	} else if err != nil {
		return "", fmt.Errorf("cannot read private MCP token: %w", err)
	}
	return validateToken(b)
}
