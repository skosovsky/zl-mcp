package mobilebackup

import (
	"context"
	"time"
)

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
