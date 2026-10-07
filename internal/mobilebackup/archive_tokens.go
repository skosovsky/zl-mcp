package mobilebackup

import (
	"context"
	"crypto/rand"
	"encoding/base64"
)

// ArchiveToken seals opaque read capabilities under a distinct purpose/AAD.
// Tokens grant no source ownership or policy permission; readers recheck both.
func (s *RetainedArchiveStore) SealArchiveToken(ctx context.Context, purpose string, body []byte) (string, error) {
	if !s.lock(ctx) {
		return "", ErrRetainedArchive
	}
	defer s.ledger.unlock()
	if !s.permanent || !archiveTokenPurpose(purpose) || len(body) == 0 || len(body) > 4096 {
		return "", ErrRetainedArchive
	}
	nonce := make([]byte, s.ledger.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", ErrRetainedArchive
	}
	sealed := s.ledger.aead.Seal(nonce, nonce, body, []byte("zl-mcp/archive-read/v1/"+purpose))
	defer clear(sealed)
	if ctx.Err() != nil {
		return "", ErrRetainedArchive
	}
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (s *RetainedArchiveStore) OpenArchiveToken(ctx context.Context, purpose, token string) ([]byte, error) {
	if !s.lock(ctx) {
		return nil, ErrRetainedArchive
	}
	defer s.ledger.unlock()
	if !s.permanent || !archiveTokenPurpose(purpose) || len(token) > 5600 {
		return nil, ErrRetainedArchive
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	defer clear(sealed)
	n := s.ledger.aead.NonceSize()
	if err != nil || len(sealed) < n+s.ledger.aead.Overhead()+1 || base64.RawURLEncoding.EncodeToString(sealed) != token {
		return nil, ErrRetainedArchive
	}
	body, err := s.ledger.aead.Open(nil, sealed[:n], sealed[n:], []byte("zl-mcp/archive-read/v1/"+purpose))
	if err != nil || len(body) > 4096 || ctx.Err() != nil {
		clear(body)
		return nil, ErrRetainedArchive
	}
	return body, nil
}

func archiveTokenPurpose(value string) bool { return value == "cursor" || value == "resource" }
