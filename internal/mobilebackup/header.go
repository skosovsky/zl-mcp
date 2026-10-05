// Package mobilebackup contains offline format candidates, not a live history source.
package mobilebackup

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"strings"
)

const (
	MaxHeaderBytes        = 64 << 10
	MaxFiles              = 1000
	MaxFileBytes   uint64 = 256 << 20
	MaxTotalBytes  uint64 = 512 << 20
)

// Header describes declarations only. It proves neither payload nor ownership.
type Header struct {
	Files      []File
	TotalBytes uint64
}
type File struct {
	Name string
	Size uint32
}

var ErrHeader = errors.New("invalid mobile backup header")

// Container contains the complete declared plaintext region, not just a header.
// Compressed is sensitive and must be cleared by its owner after use.
type Container struct {
	Header     Header `json:"-"`
	Compressed []byte `json:"-"`
}

func (Container) String() string   { return "mobile backup container [redacted]" }
func (Container) GoString() string { return "mobile backup container [redacted]" }

// ReadContainer checks the checksum through the declared container end and then
// separates the file table from compressed bytes. It leaves outer framing unread.
func ReadContainer(ctx context.Context, r io.Reader, limit uint64) (Container, error) {
	var prefix [14]byte
	if ctx == nil || r == nil || limit == 0 || limit > MaxTotalBytes {
		return Container{}, ErrHeader
	}
	reader := cancelReader{ctx, r}
	if _, err := io.ReadFull(reader, prefix[:]); err != nil || string(prefix[:6]) != "ZDB4.0" {
		return Container{}, ErrHeader
	}
	length := uint64(binary.BigEndian.Uint32(prefix[6:10]))
	if length < 18 || length > limit {
		return Container{}, ErrHeader
	}
	body := make([]byte, int(length)-len(prefix))
	if _, err := io.ReadFull(reader, body); err != nil || ctx.Err() != nil || checksum32(body) != binary.BigEndian.Uint32(prefix[10:14]) {
		clear(body)
		return Container{}, ErrHeader
	}
	h, offset, err := readFileTable(body)
	if err != nil || offset == len(body) {
		clear(body)
		return Container{}, ErrHeader
	}
	compressed := body[offset:]
	clear(body[:offset])
	return Container{Header: h, Compressed: compressed}, nil
}

func readFileTable(body []byte) (Header, int, error) {
	if len(body) < 4 {
		return Header{}, 0, ErrHeader
	}
	original := len(body)
	count := binary.BigEndian.Uint32(body[:4])
	if count == 0 || count > MaxFiles {
		return Header{}, 0, ErrHeader
	}
	body = body[4:]
	result := Header{Files: make([]File, 0, int(count))}
	seen := make(map[string]bool, int(count))
	for i := uint32(0); i < count; i++ {
		if len(body) < 4 {
			return Header{}, 0, ErrHeader
		}
		n := binary.BigEndian.Uint32(body[:4])
		body = body[4:]
		if n == 0 || n > 128 || uint64(n)+4 > uint64(len(body)) {
			return Header{}, 0, ErrHeader
		}
		name := string(body[:n])
		body = body[n:]
		size := binary.BigEndian.Uint32(body[:4])
		body = body[4:]
		if original-len(body)+14 > MaxHeaderBytes || !validName(name) || seen[name] || size == 0 || uint64(size) > MaxFileBytes || result.TotalBytes+uint64(size) > MaxTotalBytes {
			return Header{}, 0, ErrHeader
		}
		seen[name] = true
		result.TotalBytes += uint64(size)
		result.Files = append(result.Files, File{Name: name, Size: size})
	}
	return result, original - len(body), nil
}

func validName(name string) bool {
	if !strings.HasSuffix(name, ".db") {
		return false
	}
	id := strings.TrimSuffix(strings.TrimPrefix(name, "group_"), ".db")
	if id == "" || len(id) > 1 && id[0] == '0' {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}
