package mobilebackup

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
)

var ErrArchive = errors.New("invalid mobile backup plaintext archive")

// PlainArchive owns opaque, unvalidated file bytes. It is never a public result.
type PlainArchive struct {
	Files []ArchiveFile `json:"-"`
}
type ArchiveFile struct {
	Name string `json:"-"`
	Data []byte `json:"-"`
}

func (PlainArchive) String() string   { return "mobile backup plaintext archive [redacted]" }
func (PlainArchive) GoString() string { return "mobile backup plaintext archive [redacted]" }
func (ArchiveFile) String() string    { return "mobile backup file [redacted]" }
func (ArchiveFile) GoString() string  { return "mobile backup file [redacted]" }
func (a *PlainArchive) Clear() {
	if a == nil {
		return
	}
	for i := range a.Files {
		clear(a.Files[i].Data)
		a.Files[i] = ArchiveFile{}
	}
	a.Files = nil
}

// ReadPlainArchive accepts the exact declared plaintext region, without guessing
// encrypted padding or opening SQLite. Ownership must be verified before import.
func ReadPlainArchive(ctx context.Context, r io.Reader, containerLimit, outputLimit uint64) (PlainArchive, error) {
	if ctx == nil || r == nil || outputLimit == 0 || outputLimit > MaxTotalBytes {
		return PlainArchive{}, ErrArchive
	}
	container, err := ReadContainer(ctx, r, containerLimit)
	if err != nil {
		slog.Warn("mobile_archive_format_failed", "stage", "HEADER_CHECKSUM_TABLE")
		return PlainArchive{}, ErrArchive
	}
	defer clear(container.Compressed)
	var trailing [1]byte
	n, err := io.ReadFull(cancelReader{ctx, r}, trailing[:])
	if n != 0 || err != io.EOF || container.Header.TotalBytes > outputLimit {
		return PlainArchive{}, ErrArchive
	}
	out, err := DecompressXZ(ctx, bytes.NewReader(container.Compressed), int64(len(container.Compressed)), int64(outputLimit))
	if err != nil || uint64(len(out)) != container.Header.TotalBytes || ctx.Err() != nil {
		slog.Warn("mobile_archive_format_failed", "stage", "XZ_OUTPUT", "decoder_failed", err != nil, "output_size_matches", uint64(len(out)) == container.Header.TotalBytes)
		clear(out)
		return PlainArchive{}, ErrArchive
	}
	result := PlainArchive{Files: make([]ArchiveFile, 0, len(container.Header.Files))}
	offset := 0
	for _, file := range container.Header.Files {
		end := offset + int(file.Size)
		result.Files = append(result.Files, ArchiveFile{Name: file.Name, Data: out[offset:end:end]})
		offset = end
	}
	return result, nil
}
