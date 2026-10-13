package mobilebackup

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
)

//nolint:recvcheck // Value String/GoString must redact formatted values; pointer methods mutate or clear owned state.
type Format1Archive struct {
	Archive                                        PlainArchive `json:"-"`
	CiphertextBytes, ContainerBytes, TrailingBytes uint64       `json:"-"`
}

func (Format1Archive) String() string   { return "mobile backup encrypted archive [redacted]" }
func (Format1Archive) GoString() string { return "mobile backup encrypted archive [redacted]" }
func (a *Format1Archive) Clear() {
	if a != nil {
		a.Archive.Clear()
		*a = Format1Archive{}
	}
}

func ReadFormat1Archive(
	ctx context.Context,
	r io.Reader,
	key string,
	ciphertextLimit, outputLimit uint64,
) (Format1Archive, error) {
	if ciphertextLimit == 0 || ciphertextLimit > MaxTotalBytes || outputLimit == 0 || outputLimit > MaxTotalBytes {
		return Format1Archive{}, ErrArchive
	}
	plain, physicalBytes, e := decryptFormat1(ctx, r, key, int64(ciphertextLimit), true)
	if e != nil {
		return Format1Archive{}, ErrArchive
	}
	defer clear(plain)
	end := uint64(binary.BigEndian.Uint32(plain[6:10]))
	if end < 18 || end > uint64(len(plain)) {
		slog.WarnContext(ctx, "mobile_archive_format_failed", "stage", "CONTAINER_BOUNDARY")
		return Format1Archive{}, ErrArchive
	}
	//nolint:gosec // G115: end was checked against len(plain), so it is representable as int.
	archive, e := ReadPlainArchive(ctx, bytes.NewReader(plain[:int(end)]), end, outputLimit)
	if e != nil {
		slog.WarnContext(ctx, "mobile_archive_format_failed", "stage", "PLAIN_CONTAINER")
		return Format1Archive{}, ErrArchive
	}
	if ctx.Err() != nil {
		archive.Clear()
		return Format1Archive{}, ErrArchive
	}
	return Format1Archive{
		Archive:         archive,
		CiphertextBytes: physicalBytes,
		ContainerBytes:  end,
		TrailingBytes:   physicalBytes - end,
	}, nil
}
