package mobilebackup

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/skosovsky/zl-mcp/internal/domain"
)

var (
	ErrRetainedArchive  = errors.New("retained account archive unavailable")
	ErrRetainedConflict = errors.New("retained account archive conflicts with its source")
	ErrRetainedCapacity = errors.New("retained account archive capacity exceeded")
	ErrRetainedExpired  = errors.New("retained account archive expired")
	ErrRetainedAbsent   = errors.New("retained account archive not captured")
)

const retainedMagic = "ZLACCT01"
const retainedMetadataLimit = 1 << 20
const retainedLifetime = 7 * 24 * time.Hour
const retainedMaxLifetime = 30 * 24 * time.Hour
const retainedMaxSources = 2
const retainedMaxFileBytes = int64(MaxTotalBytes) + retainedMetadataLimit + 128

// RetainedArchiveStore uses the existing confined key/claim/publication primitives
// in a separate directory. It never applies staging snapshot cleanup or lifetime.
type RetainedArchiveStore struct {
	ledger    *SnapshotStore
	permanent bool
	backup    bool // read-only recovery source; never opened through the cache constructor
}

func (RetainedArchiveStore) String() string   { return "retained account archive store [redacted]" }
func (RetainedArchiveStore) GoString() string { return "retained account archive store [redacted]" }

type RetainedArchiveManifest struct {
	SourceID        string `json:"source_id"`
	Scope           string `json:"scope"`
	CapturedAt      string `json:"captured_at"`
	ExpiresAt       string `json:"expires_at"`
	Digest          string `json:"digest"`
	FileCount       int    `json:"file_count"`
	DirectFiles     int    `json:"direct_files"`
	GroupFiles      int    `json:"group_files"`
	StoredBytes     int64  `json:"stored_bytes"`
	HistoryComplete bool   `json:"history_complete"`
	ImportPerformed bool   `json:"import_performed"`
}

type retainedFile struct {
	Name      string `json:"name"`
	SessionID string `json:"session_id"`
	Group     bool   `json:"group"`
	Size      int64  `json:"size"`
	Digest    string `json:"digest"`
}
type retainedMetadata struct {
	SourceID          string         `json:"source_id"`
	Account           string         `json:"account"`
	CreatedMS         int64          `json:"created_ms"`
	ExpiresMS         int64          `json:"expires_ms"`
	PreservedMS       int64          `json:"preserved_ms,omitempty"`
	SourceStoredBytes int64          `json:"source_stored_bytes,omitempty"`
	Digest            string         `json:"digest"`
	Files             []retainedFile `json:"files"`
	CiphertextBytes   uint64         `json:"ciphertext_bytes"`
	ContainerBytes    uint64         `json:"container_bytes"`
	TrailingBytes     uint64         `json:"trailing_bytes"`
}

func (retainedFile) String() string       { return "retained archive file metadata [redacted]" }
func (retainedFile) GoString() string     { return "retained archive file metadata [redacted]" }
func (retainedMetadata) String() string   { return "retained archive metadata [redacted]" }
func (retainedMetadata) GoString() string { return "retained archive metadata [redacted]" }

func NewRetainedArchiveStore(dir string, budget int64) (*RetainedArchiveStore, error) {
	return newRetainedArchiveStore(dir, budget, false)
}

// NewArchiveLibraryStore opens durable sources; only explicit preservation can publish.
func NewArchiveLibraryStore(dir string, budget int64) (*RetainedArchiveStore, error) {
	return newRetainedArchiveStore(dir, budget, true)
}

func newRetainedArchiveStore(dir string, budget int64, permanent bool) (*RetainedArchiveStore, error) {
	if !filepath.IsAbs(dir) || budget < 1 || budget > 2<<30 {
		return nil, ErrRetainedArchive
	}
	if os.MkdirAll(dir, 0700) != nil {
		return nil, ErrRetainedArchive
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrRetainedArchive
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, ErrRetainedArchive
	}
	key, err := snapshotKey(root)
	if err != nil {
		root.Close()
		return nil, ErrRetainedArchive
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		root.Close()
		return nil, ErrRetainedArchive
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		root.Close()
		return nil, ErrRetainedArchive
	}
	claims, err := readSnapshotClaims(root)
	if err != nil {
		root.Close()
		return nil, ErrRetainedArchive
	}
	s := &RetainedArchiveStore{permanent: permanent, ledger: &SnapshotStore{root: root, aead: aead, claims: claims, budget: budget, now: time.Now, mu: make(chan struct{}, 1)}}
	s.ledger.mu <- struct{}{}
	if _, _, err = s.inventory(); err != nil {
		root.Close()
		return nil, err
	}
	return s, nil
}
func (s *RetainedArchiveStore) Close() error {
	if s == nil || s.ledger == nil {
		return nil
	}
	return s.ledger.Close()
}
func retainedName(id string) (string, bool) { _, ok := snapshotName(id); return id + ".archive", ok }
func (s *RetainedArchiveStore) lock(ctx context.Context) bool {
	if s == nil || s.ledger == nil || !s.ledger.lock(ctx) {
		return false
	}
	if s.ledger.root == nil {
		s.ledger.unlock()
		return false
	}
	return true
}

func describeRetained(a AccountArchive) ([]retainedFile, string, error) {
	if len(a.pairs) == 0 {
		return nil, "", ErrRetainedArchive
	}
	refType := "direct"
	if a.pairs[0].Group {
		refType = "group"
	}
	// Revalidate complete mapping even when an internal caller built the value.
	ref := domain.ConversationRef{Type: refType, ID: a.pairs[0].Session}
	if _, err := SelectArchiveIndex(context.Background(), a.archive, a.pairs, ref); err != nil {
		return nil, "", ErrRetainedArchive
	}
	byName := make(map[string]IdentityPair, len(a.pairs))
	for _, p := range a.pairs {
		name := p.Plain + ".db"
		if p.Group {
			name = "group_" + name
		}
		byName[name] = p
	}
	files := make([]retainedFile, 0, len(a.archive.Files))
	for _, f := range a.archive.Files {
		p := byName[f.Name]
		hash := sha256.Sum256(f.Data)
		files = append(files, retainedFile{Name: f.Name, SessionID: p.Session, Group: p.Group, Size: int64(len(f.Data)), Digest: hex.EncodeToString(hash[:])})
	}
	description, err := json.Marshal(files)
	if err != nil || len(description) > retainedMetadataLimit {
		return nil, "", ErrRetainedArchive
	}
	defer clear(description)
	digest := sha256.Sum256(description)
	return files, hex.EncodeToString(digest[:]), nil
}

func (s *RetainedArchiveStore) Save(ctx context.Context, id, account string, a AccountArchive, lifetime time.Duration) (RetainedArchiveManifest, error) {
	if !s.lock(ctx) {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	defer s.ledger.unlock()
	_, ok := retainedName(id)
	if s.permanent || s.backup || !ok || !canonicalIdentity(account) || (a.ownerAccount != "" && a.ownerAccount != account) {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	if lifetime == 0 {
		lifetime = retainedLifetime
	}
	if lifetime < time.Hour || lifetime > retainedMaxLifetime || lifetime%time.Millisecond != 0 {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	files, digest, err := describeRetained(a)
	if err != nil || ctx.Err() != nil {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	if s.ledger.claims[id] {
		meta, old, stored, err := s.read(ctx, id, account)
		defer old.Clear()
		if err != nil {
			return RetainedArchiveManifest{}, err
		}
		if meta.Digest != digest || meta.ExpiresMS-meta.CreatedMS != lifetime.Milliseconds() || meta.CiphertextBytes != a.ciphertextBytes || meta.ContainerBytes != a.containerBytes || meta.TrailingBytes != a.trailingBytes {
			return RetainedArchiveManifest{}, ErrRetainedConflict
		}
		return retainedManifest(meta, stored), nil
	}
	now := s.ledger.now()
	meta := retainedMetadata{SourceID: id, Account: account, CreatedMS: now.UnixMilli(), ExpiresMS: now.Add(lifetime).UnixMilli(), Digest: digest, Files: files, CiphertextBytes: a.ciphertextBytes, ContainerBytes: a.containerBytes, TrailingBytes: a.trailingBytes}
	return s.publish(ctx, meta, a)
}

// publish requires the caller to hold the store lock and validate the complete source.
func (s *RetainedArchiveStore) publish(ctx context.Context, meta retainedMetadata, a AccountArchive) (RetainedArchiveManifest, error) {
	id := meta.SourceID
	name, ok := retainedName(id)
	if !ok || s.ledger.claims[id] {
		return RetainedArchiveManifest{}, ErrRetainedConflict
	}
	count, used, err := s.inventory()
	if err != nil {
		return RetainedArchiveManifest{}, err
	}
	files := meta.Files
	header, err := json.Marshal(meta)
	if err != nil || len(header) > retainedMetadataLimit {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	defer clear(header)
	total := int64(4 + len(header))
	for _, f := range files {
		total += f.Size
	}
	stored := int64(len(retainedMagic)+s.ledger.aead.NonceSize()+s.ledger.aead.Overhead()) + total
	if count >= retainedMaxSources || stored > retainedMaxFileBytes || stored > s.ledger.budget-used {
		return RetainedArchiveManifest{}, ErrRetainedCapacity
	}
	plain := make([]byte, int(total))
	defer clear(plain)
	binary.BigEndian.PutUint32(plain, uint32(len(header)))
	copy(plain[4:], header)
	offset := 4 + len(header)
	for _, f := range a.archive.Files {
		copy(plain[offset:], f.Data)
		offset += len(f.Data)
	}
	nonce := make([]byte, s.ledger.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	sealed := append([]byte(retainedMagic), nonce...)
	sealed = s.ledger.aead.Seal(sealed, nonce, plain, []byte(retainedMagic+id))
	defer clear(sealed)
	if ctx.Err() != nil {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	if err = s.ledger.claim(id); err != nil {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	tmp := uuid.NewString() + ".tmp"
	f, err := s.ledger.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	defer s.ledger.root.Remove(tmp)
	_, err = f.Write(sealed)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil || ctx.Err() != nil {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	if s.ledger.root.Rename(tmp, name) != nil || snapshotSyncDirectory(s.ledger.root) != nil {
		return RetainedArchiveManifest{}, ErrRetainedArchive
	}
	return retainedManifest(meta, stored), nil
}

func retainedManifest(m retainedMetadata, stored int64) RetainedArchiveManifest {
	if m.SourceStoredBytes > 0 {
		stored = m.SourceStoredBytes
	}
	groups := 0
	for _, f := range m.Files {
		if f.Group {
			groups++
		}
	}
	return RetainedArchiveManifest{SourceID: m.SourceID, Scope: "account", CapturedAt: time.UnixMilli(m.CreatedMS).UTC().Format(time.RFC3339Nano), ExpiresAt: time.UnixMilli(m.ExpiresMS).UTC().Format(time.RFC3339Nano), Digest: m.Digest, FileCount: len(m.Files), DirectFiles: len(m.Files) - groups, GroupFiles: groups, StoredBytes: stored}
}

func (s *RetainedArchiveStore) Read(ctx context.Context, id, account string) (AccountArchive, RetainedArchiveManifest, error) {
	if !s.lock(ctx) {
		return AccountArchive{}, RetainedArchiveManifest{}, ErrRetainedArchive
	}
	defer s.ledger.unlock()
	m, a, stored, err := s.read(ctx, id, account)
	if err != nil {
		return AccountArchive{}, RetainedArchiveManifest{}, err
	}
	return a, retainedManifest(m, stored), nil
}
func (s *RetainedArchiveStore) read(ctx context.Context, id, account string) (retainedMetadata, AccountArchive, int64, error) {
	if !canonicalIdentity(account) {
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	hash := sha256.Sum256([]byte(account))
	return s.readBound(ctx, id, hex.EncodeToString(hash[:]))
}

// ReadBound permits offline owner access using the persisted account binding.
// It never needs a running collector or exposes the raw account ID in manifest.
func (s *RetainedArchiveStore) ReadBound(ctx context.Context, id, accountKey string) (AccountArchive, RetainedArchiveManifest, error) {
	if !s.lock(ctx) {
		return AccountArchive{}, RetainedArchiveManifest{}, ErrRetainedArchive
	}
	defer s.ledger.unlock()
	m, a, stored, err := s.readBound(ctx, id, accountKey)
	if err != nil {
		return AccountArchive{}, RetainedArchiveManifest{}, err
	}
	return a, retainedManifest(m, stored), nil
}

func (s *RetainedArchiveStore) readBound(ctx context.Context, id, accountKey string) (retainedMetadata, AccountArchive, int64, error) {
	key, decodeErr := hex.DecodeString(accountKey)
	if _, ok := retainedName(id); !ok || decodeErr != nil || len(key) != 32 || hex.EncodeToString(key) != accountKey || ctx.Err() != nil {
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	if !s.ledger.claims[id] {
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedAbsent
	}
	m, a, stored, err := s.decode(id)
	if err != nil {
		return retainedMetadata{}, AccountArchive{}, 0, err
	}
	hash := sha256.Sum256([]byte(m.Account))
	if hex.EncodeToString(hash[:]) != accountKey {
		a.Clear()
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedConflict
	}
	if !s.permanent && !s.backup && s.ledger.now().UnixMilli() >= m.ExpiresMS {
		a.Clear()
		name, _ := retainedName(id)
		if s.ledger.root.Remove(name) != nil || snapshotSyncDirectory(s.ledger.root) != nil {
			return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
		}
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedExpired
	}
	if ctx.Err() != nil {
		a.Clear()
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	a.ownerAccount = m.Account
	a.retainedSourceID = m.SourceID
	return m, a, stored, nil
}

func (s *RetainedArchiveStore) decode(id string) (retainedMetadata, AccountArchive, int64, error) {
	name, ok := retainedName(id)
	if !ok {
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	info, err := s.ledger.root.Lstat(name)
	minimum := int64(len(retainedMagic) + s.ledger.aead.NonceSize() + s.ledger.aead.Overhead() + 4)
	if err != nil || !privateRegular(info) || info.Size() < minimum || info.Size() > retainedMaxFileBytes {
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	f, err := s.ledger.root.Open(name)
	if err != nil {
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	if !samePrivateFile(f, info) {
		f.Close()
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	sealed, err := io.ReadAll(io.LimitReader(f, info.Size()+1))
	f.Close()
	defer clear(sealed)
	if err != nil || int64(len(sealed)) != info.Size() || string(sealed[:len(retainedMagic)]) != retainedMagic {
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	offset := len(retainedMagic) + s.ledger.aead.NonceSize()
	plain, err := s.ledger.aead.Open(nil, sealed[len(retainedMagic):offset], sealed[offset:], []byte(retainedMagic+id))
	if err != nil || len(plain) < 4 {
		clear(plain)
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	fail := func() (retainedMetadata, AccountArchive, int64, error) {
		clear(plain)
		return retainedMetadata{}, AccountArchive{}, 0, ErrRetainedArchive
	}
	headerSize := int(binary.BigEndian.Uint32(plain[:4]))
	if headerSize < 1 || headerSize > retainedMetadataLimit || headerSize > len(plain)-4 {
		return fail()
	}
	var meta retainedMetadata
	d := json.NewDecoder(bytes.NewReader(plain[4 : 4+headerSize]))
	d.DisallowUnknownFields()
	if d.Decode(&meta) != nil || d.Decode(new(any)) != io.EOF || meta.SourceID != id || !canonicalIdentity(meta.Account) || meta.CreatedMS <= 0 || meta.ExpiresMS-meta.CreatedMS < time.Hour.Milliseconds() || meta.ExpiresMS-meta.CreatedMS > retainedMaxLifetime.Milliseconds() || len(meta.Files) < 1 || len(meta.Files) > MaxFiles || (s.permanent && (meta.PreservedMS <= 0 || meta.SourceStoredBytes <= 0)) || (!s.permanent && (meta.PreservedMS != 0 || meta.SourceStoredBytes != 0)) {
		return fail()
	}
	archive := AccountArchive{ciphertextBytes: meta.CiphertextBytes, containerBytes: meta.ContainerBytes, trailingBytes: meta.TrailingBytes}
	offset = 4 + headerSize
	for _, file := range meta.Files {
		if file.Size < 1 || uint64(file.Size) > MaxFileBytes || file.Size > int64(len(plain)-offset) {
			return fail()
		}
		end := offset + int(file.Size)
		data := plain[offset:end:end]
		hash := sha256.Sum256(data)
		if hex.EncodeToString(hash[:]) != file.Digest {
			return fail()
		}
		archive.archive.Files = append(archive.archive.Files, ArchiveFile{Name: file.Name, Data: data})
		archive.pairs = append(archive.pairs, IdentityPair{Plain: strings.TrimSuffix(strings.TrimPrefix(file.Name, "group_"), ".db"), Session: file.SessionID, Group: file.Group})
		offset = end
	}
	if offset != len(plain) {
		return fail()
	}
	_, digest, err := describeRetained(archive)
	if err != nil || digest != meta.Digest {
		return fail()
	}
	clear(plain[:4+headerSize])
	return meta, archive, info.Size(), nil
}

func (s *RetainedArchiveStore) inventory() (int, int64, error) {
	f, err := s.ledger.root.Open(".")
	if err != nil {
		return 0, 0, ErrRetainedArchive
	}
	entries, err := f.ReadDir(32)
	f.Close()
	if err != nil && err != io.EOF || len(entries) > retainedMaxSources+3 {
		return 0, 0, ErrRetainedCapacity
	}
	count := 0
	var used int64
	for _, entry := range entries {
		name := entry.Name()
		info, err := s.ledger.root.Lstat(name)
		if err != nil || !privateRegular(info) {
			return 0, 0, ErrRetainedArchive
		}
		if name == snapshotKeyName || name == snapshotClaimsName {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			if _, ok := retainedName(strings.TrimSuffix(name, ".tmp")); !ok || s.ledger.root.Remove(name) != nil || snapshotSyncDirectory(s.ledger.root) != nil {
				return 0, 0, ErrRetainedArchive
			}
			continue
		}
		id := strings.TrimSuffix(name, ".archive")
		expected, ok := retainedName(id)
		if !ok || expected != name || !s.ledger.claims[id] {
			return 0, 0, ErrRetainedArchive
		}
		m, a, stored, err := s.decode(id)
		a.Clear()
		if err != nil {
			return 0, 0, err
		}
		if !s.permanent && s.ledger.now().UnixMilli() >= m.ExpiresMS {
			if s.ledger.root.Remove(name) != nil || snapshotSyncDirectory(s.ledger.root) != nil {
				return 0, 0, ErrRetainedArchive
			}
			continue
		}
		count++
		used += stored
	}
	if count > retainedMaxSources || used > s.ledger.budget {
		return 0, 0, ErrRetainedCapacity
	}
	return count, used, nil
}

func (s *RetainedArchiveStore) Remove(ctx context.Context, id, account string) error {
	if !canonicalIdentity(account) {
		return ErrRetainedArchive
	}
	hash := sha256.Sum256([]byte(account))
	return s.RemoveBound(ctx, id, hex.EncodeToString(hash[:]))
}

// RemoveBound authenticates the account binding without requiring a live session.
func (s *RetainedArchiveStore) RemoveBound(ctx context.Context, id, accountKey string) error {
	if s == nil || s.backup {
		return ErrRetainedArchive
	}
	if !s.lock(ctx) {
		return ErrRetainedArchive
	}
	defer s.ledger.unlock()
	name, ok := retainedName(id)
	key, err := hex.DecodeString(accountKey)
	if !ok || err != nil || len(key) != 32 || hex.EncodeToString(key) != accountKey {
		return ErrRetainedArchive
	}
	if _, err = s.ledger.root.Lstat(name); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if !s.ledger.claims[id] {
		return ErrRetainedArchive
	}
	m, a, _, err := s.decode(id)
	a.Clear()
	if err != nil {
		return err
	}
	hash := sha256.Sum256([]byte(m.Account))
	if hex.EncodeToString(hash[:]) != accountKey {
		return ErrRetainedConflict
	}
	if ctx.Err() != nil || s.ledger.root.Remove(name) != nil || snapshotSyncDirectory(s.ledger.root) != nil {
		return ErrRetainedArchive
	}
	return nil
}
