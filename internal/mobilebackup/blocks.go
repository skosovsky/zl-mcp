package mobilebackup

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"errors"
	"io"
)

var ErrBlocks = errors.New("invalid mobile backup block stream")

const blockChunk = 65536

// DecryptFormat1 implements only the observed offset-zero block transform.
// The caller must validate header, payload and ownership before persistence.
func DecryptFormat1(ctx context.Context, r io.Reader, keyText string, limit int64) ([]byte, error) {
	if ctx == nil || r == nil || limit <= 0 || uint64(limit) > MaxTotalBytes || len(keyText) < 32 || len(keyText) > 256 || len(keyText)%2 != 0 {
		return nil, ErrBlocks
	}
	var key [32]byte
	for i := range len(keyText) {
		b := keyText[i]
		if b >= 'a' && b <= 'f' {
			b -= 'a' - 'A'
		}
		if !(b >= '0' && b <= '9' || b >= 'A' && b <= 'F') {
			return nil, ErrBlocks
		}
		if i < len(key) {
			key[i] = b
		}
	}
	defer clear(key[:])
	data, err := io.ReadAll(io.LimitReader(cancelReader{ctx, r}, limit+1))
	if err != nil {
		clear(data)
		return nil, ErrBlocks
	}
	if len(data) < aes.BlockSize || int64(len(data)) > limit || len(data)%aes.BlockSize != 0 {
		clear(data)
		return nil, ErrBlocks
	}
	block, err := aes.NewCipher(key[:])
	if err != nil {
		clear(data)
		return nil, ErrBlocks
	}
	for offset := 0; offset < len(data); offset += blockChunk {
		if ctx.Err() != nil {
			clear(data)
			return nil, ErrBlocks
		}
		part := data[offset:min(offset+blockChunk, len(data))]
		var iv [aes.BlockSize]byte
		cipher.NewCBCDecrypter(block, iv[:]).CryptBlocks(part, part)
	}
	if string(data[:6]) != "ZDB4.0" {
		clear(data)
		return nil, ErrBlocks
	}
	return data, nil
}

type cancelReader struct {
	ctx context.Context
	r   io.Reader
}

func (r cancelReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
