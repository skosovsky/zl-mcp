package mobilebackup

import (
	"context"
	"encoding/hex"
	"io"
	"sort"
	"strings"
	"time"
)

// Sources authenticates every currently published library source before returning metadata.
func (s *RetainedArchiveStore) Sources(ctx context.Context, accountKey string) ([]PreservationReceipt, error) {
	if !s.lock(ctx) {
		return nil, ErrRetainedArchive
	}
	defer s.ledger.unlock()
	key, decodeErr := hex.DecodeString(accountKey)
	if !s.permanent || decodeErr != nil || len(key) != 32 || hex.EncodeToString(key) != accountKey {
		return nil, ErrRetainedArchive
	}
	dir, err := s.ledger.root.Open(".")
	if err != nil {
		return nil, ErrRetainedArchive
	}
	entries, err := dir.ReadDir(32)
	dir.Close()
	if err != nil && err != io.EOF || len(entries) > retainedMaxSources+3 {
		return nil, ErrRetainedArchive
	}
	result := []PreservationReceipt{}
	for _, entry := range entries {
		name := entry.Name()
		if name == snapshotKeyName || name == snapshotClaimsName {
			continue
		}
		id := strings.TrimSuffix(name, ".archive")
		expected, valid := retainedName(id)
		if !valid || expected != name {
			return nil, ErrRetainedArchive
		}
		meta, a, stored, e := s.readBound(ctx, id, accountKey)
		a.Clear()
		if e != nil {
			return nil, e
		}
		result = append(result, preservationReceipt(meta, stored))
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Source.SourceID < result[j].Source.SourceID })
	if ctx.Err() != nil {
		return nil, ErrRetainedArchive
	}
	return result, nil
}

type PreservationReceipt struct {
	Source              RetainedArchiveManifest `json:"source"`
	Retention           string                  `json:"retention"`
	PreservedAt         string                  `json:"preserved_at"`
	DownloadPerformed   bool                    `json:"download_performed"`
	ImportPerformed     bool                    `json:"import_performed"`
	PublicAccessEnabled bool                    `json:"public_access_enabled"`
}

func preservationReceipt(meta retainedMetadata, stored int64) PreservationReceipt {
	return PreservationReceipt{Source: retainedManifest(meta, stored), Retention: "until_owner_deletion", PreservedAt: time.UnixMilli(meta.PreservedMS).UTC().Format(time.RFC3339Nano)}
}

// PreservationStatus authenticates durable provenance without opening a Zalo session.
func (s *RetainedArchiveStore) PreservationStatus(ctx context.Context, id, accountKey string) (PreservationReceipt, error) {
	if !s.lock(ctx) {
		return PreservationReceipt{}, ErrRetainedArchive
	}
	defer s.ledger.unlock()
	if !s.permanent {
		return PreservationReceipt{}, ErrRetainedArchive
	}
	meta, a, stored, err := s.readBound(ctx, id, accountKey)
	defer a.Clear()
	if err != nil {
		return PreservationReceipt{}, err
	}
	return preservationReceipt(meta, stored), nil
}

// PreserveFrom promotes one authenticated cache source into this library. It
// keeps original bytes and provenance and uses the library's independent key.
func (s *RetainedArchiveStore) PreserveFrom(ctx context.Context, source *RetainedArchiveStore, id, accountKey string) (PreservationReceipt, error) {
	if source == nil || source == s || !s.permanent || source.permanent || !source.lock(ctx) {
		return PreservationReceipt{}, ErrRetainedArchive
	}
	meta, a, originalStored, err := source.readBound(ctx, id, accountKey)
	source.ledger.unlock()
	defer a.Clear()
	if err != nil {
		return PreservationReceipt{}, err
	}
	return s.preserveAuthenticated(ctx, meta, a, originalStored, accountKey)
}

func (s *RetainedArchiveStore) preserveAuthenticated(ctx context.Context, meta retainedMetadata, a AccountArchive, originalStored int64, accountKey string) (PreservationReceipt, error) {
	id := meta.SourceID
	if !s.lock(ctx) {
		return PreservationReceipt{}, ErrRetainedArchive
	}
	defer s.ledger.unlock()
	if s.ledger.claims[id] {
		old, b, stored, e := s.readBound(ctx, id, accountKey)
		defer b.Clear()
		if e != nil {
			return PreservationReceipt{}, e
		}
		if old.Digest != meta.Digest || old.CreatedMS != meta.CreatedMS || old.ExpiresMS != meta.ExpiresMS || old.CiphertextBytes != meta.CiphertextBytes || old.ContainerBytes != meta.ContainerBytes || old.TrailingBytes != meta.TrailingBytes {
			return PreservationReceipt{}, ErrRetainedConflict
		}
		return preservationReceipt(old, stored), nil
	}
	meta.PreservedMS = s.ledger.now().UnixMilli()
	meta.SourceStoredBytes = originalStored
	m, err := s.publish(ctx, meta, a)
	if err != nil {
		return PreservationReceipt{}, err
	}
	return PreservationReceipt{Source: m, Retention: "until_owner_deletion", PreservedAt: time.UnixMilli(meta.PreservedMS).UTC().Format(time.RFC3339Nano)}, nil
}
