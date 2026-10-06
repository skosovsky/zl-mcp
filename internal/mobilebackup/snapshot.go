package mobilebackup

import (
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
	ErrSnapshot         = errors.New("mobile archive snapshot unavailable")
	ErrSnapshotConflict = errors.New("mobile archive snapshot conflicts with saved source")
	ErrSnapshotCapacity = errors.New("mobile archive snapshot capacity exhausted")
)

const snapshotLifetime = 15 * time.Minute
const snapshotMetadataLimit = 4096
const snapshotMagic = "ZLSNAP01"
const snapshotKeyName = ".snapshot-key"
const snapshotClaimsName = ".snapshot-claims"
const maxSnapshots = 20
const maxSnapshotClaims = 100000

// SnapshotStore belongs to the single service owning its state lock. No path,
// key or archive data from this internal port is an MCP argument or result.
type SnapshotStore struct {
	mu     chan struct{}
	root   *os.Root
	aead   cipher.AEAD
	budget int64
	now    func() time.Time
	claims map[string]bool
}

func (s *SnapshotStore) lock(ctx context.Context) bool {
	if s == nil || s.mu == nil || ctx == nil || ctx.Err() != nil {
		return false
	}
	select {
	case <-ctx.Done():
		return false
	case <-s.mu:
		if ctx.Err() != nil {
			s.unlock()
			return false
		}
		return true
	}
}
func (s *SnapshotStore) unlock() { s.mu <- struct{}{} }

func (SnapshotStore) String() string   { return "mobile archive snapshot store [redacted]" }
func (SnapshotStore) GoString() string { return "mobile archive snapshot store [redacted]" }

func NewSnapshotStore(dir string, budget int64) (*SnapshotStore, error) {
	if !filepath.IsAbs(dir) || budget < 1 || uint64(budget) > MaxTotalBytes {
		return nil, ErrSnapshot
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, ErrSnapshot
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return nil, ErrSnapshot
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, ErrSnapshot
	}
	key, err := snapshotKey(root)
	if err != nil {
		root.Close()
		return nil, err
	}
	defer clear(key)
	block, err := aes.NewCipher(key)
	if err != nil {
		root.Close()
		return nil, ErrSnapshot
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		root.Close()
		return nil, ErrSnapshot
	}
	claims, err := readSnapshotClaims(root)
	if err != nil {
		root.Close()
		return nil, err
	}
	store := &SnapshotStore{mu: make(chan struct{}, 1), root: root, aead: aead, budget: budget, now: time.Now, claims: claims}
	store.mu <- struct{}{}
	if err = store.Cleanup(context.Background()); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

func readSnapshotClaims(root *os.Root) (map[string]bool, error) {
	result := map[string]bool{}
	info, err := root.Lstat(snapshotClaimsName)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil || !privateRegular(info) || info.Size() > 37*maxSnapshotClaims {
		return nil, ErrSnapshot
	}
	f, err := root.Open(snapshotClaimsName)
	if err != nil {
		return nil, ErrSnapshot
	}
	if !samePrivateFile(f, info) {
		f.Close()
		return nil, ErrSnapshot
	}
	data, err := io.ReadAll(io.LimitReader(f, 37*maxSnapshotClaims+1))
	f.Close()
	if err != nil || int64(len(data)) != info.Size() {
		return nil, ErrSnapshot
	}
	if len(data) == 0 {
		return result, nil
	}
	if data[len(data)-1] != '\n' {
		return nil, ErrSnapshot
	}
	for _, id := range strings.Split(string(data[:len(data)-1]), "\n") {
		_, ok := snapshotName(id)
		if !ok || result[id] {
			return nil, ErrSnapshot
		}
		result[id] = true
	}
	return result, nil
}

func (s *SnapshotStore) claim(id string) error {
	if s.claims[id] {
		return ErrSnapshot
	}
	if len(s.claims) >= maxSnapshotClaims {
		return ErrSnapshotCapacity
	}
	info, err := s.root.Lstat(snapshotClaimsName)
	if err != nil && !errors.Is(err, os.ErrNotExist) || err == nil && !privateRegular(info) {
		return ErrSnapshot
	}
	f, err := s.root.OpenFile(snapshotClaimsName, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0600)
	if err != nil {
		return ErrSnapshot
	}
	opened, statErr := f.Stat()
	if statErr != nil || !privateRegular(opened) || info != nil && !os.SameFile(info, opened) {
		f.Close()
		return ErrSnapshot
	}
	_, err = f.WriteString(id + "\n")
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return ErrSnapshot
	}
	s.claims[id] = true
	return snapshotSyncDirectory(s.root)
}

func snapshotKey(root *os.Root) ([]byte, error) {
	info, err := root.Lstat(snapshotKeyName)
	if errors.Is(err, os.ErrNotExist) {
		// An existing snapshot without its key must not be silently abandoned.
		dir, e := root.Open(".")
		if e != nil {
			return nil, ErrSnapshot
		}
		entries, e := dir.ReadDir(64)
		dir.Close()
		if e != nil && e != io.EOF || len(entries) != 0 {
			return nil, ErrSnapshot
		}
		key := make([]byte, 32)
		if _, e = rand.Read(key); e != nil {
			clear(key)
			return nil, ErrSnapshot
		}
		f, e := root.OpenFile(snapshotKeyName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			clear(key)
			return nil, ErrSnapshot
		}
		_, e = f.Write(key)
		if e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e != nil || closeErr != nil {
			clear(key)
			return nil, ErrSnapshot
		}
		if snapshotSyncDirectory(root) != nil {
			clear(key)
			return nil, ErrSnapshot
		}
		return key, nil
	}
	if err != nil || !privateRegular(info) || info.Size() != 32 {
		return nil, ErrSnapshot
	}
	f, err := root.Open(snapshotKeyName)
	if err != nil {
		return nil, ErrSnapshot
	}
	if !samePrivateFile(f, info) {
		f.Close()
		return nil, ErrSnapshot
	}
	key, err := io.ReadAll(io.LimitReader(f, 33))
	f.Close()
	if err != nil || len(key) != 32 {
		clear(key)
		return nil, ErrSnapshot
	}
	return key, nil
}

func privateRegular(info os.FileInfo) bool {
	return info != nil && info.Mode().IsRegular() && info.Mode().Perm()&0077 == 0
}

func samePrivateFile(f *os.File, expected os.FileInfo) bool {
	actual, err := f.Stat()
	return err == nil && privateRegular(actual) && os.SameFile(expected, actual)
}

type snapshotMetadata struct {
	Account         string `json:"account"`
	Fingerprint     string `json:"fingerprint"`
	Filename        string `json:"filename"`
	Digest          string `json:"digest"`
	CreatedMS       int64  `json:"created_ms"`
	ExpiresMS       int64  `json:"expires_ms"`
	CiphertextBytes uint64 `json:"ciphertext_bytes"`
	ContainerBytes  uint64 `json:"container_bytes"`
	TrailingBytes   uint64 `json:"trailing_bytes"`
}

func snapshotName(id string) (string, bool) {
	u, err := uuid.Parse(id)
	return id + ".snapshot", err == nil && u.String() == id && len(id) == 36
}

// Save binds one exact immutable source. Retrying never extends its lifetime.
func (s *SnapshotStore) Save(ctx context.Context, selected SelectedArchive, request domain.MobileBackupRequest, account string) error {
	if !s.lock(ctx) {
		return ErrSnapshot
	}
	defer s.unlock()
	r, err := request.Normalize()
	if err != nil || ctx == nil || ctx.Err() != nil || s.root == nil || !canonicalIdentity(account) || !canonicalIdentity(r.ConversationID) || selected.requestID != r.RequestID || selected.requestFingerprint != r.Fingerprint() || selected.ref != r.Ref() || !validName(selected.File.Name) || uint64(len(selected.File.Data)) > MaxFileBytes || !standaloneSQLite(selected.File.Data) {
		return ErrSnapshot
	}
	name, ok := snapshotName(r.RequestID)
	if !ok {
		return ErrSnapshot
	}
	if _, err = s.root.Lstat(name); err == nil {
		prior, e := s.read(ctx, r, account)
		defer prior.Clear()
		if e != nil {
			return e
		}
		if prior.File.Name != selected.File.Name || sha256.Sum256(prior.File.Data) != sha256.Sum256(selected.File.Data) {
			return ErrSnapshotConflict
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrSnapshot
	}
	if s.claims[r.RequestID] {
		return ErrSnapshot
	}
	count, used, err := s.sweep(ctx)
	if err != nil {
		return err
	}
	digest := sha256.Sum256(selected.File.Data)
	now := s.now()
	meta := snapshotMetadata{Account: account, Fingerprint: r.Fingerprint(), Filename: selected.File.Name, Digest: hex.EncodeToString(digest[:]), CreatedMS: now.UnixMilli(), ExpiresMS: now.Add(snapshotLifetime).UnixMilli(), CiphertextBytes: selected.CiphertextBytes, ContainerBytes: selected.ContainerBytes, TrailingBytes: selected.TrailingBytes}
	metadata, err := json.Marshal(meta)
	if err != nil || len(metadata) > snapshotMetadataLimit {
		return ErrSnapshot
	}
	plain := make([]byte, 4+len(metadata)+len(selected.File.Data))
	defer clear(plain)
	binary.BigEndian.PutUint32(plain, uint32(len(metadata)))
	copy(plain[4:], metadata)
	copy(plain[4+len(metadata):], selected.File.Data)
	nonce := make([]byte, s.aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return ErrSnapshot
	}
	sealed := append([]byte(snapshotMagic), nonce...)
	sealed = s.aead.Seal(sealed, nonce, plain, []byte(snapshotMagic+r.RequestID))
	defer clear(sealed)
	if count >= maxSnapshots || int64(len(sealed)) > s.budget-used {
		return ErrSnapshotCapacity
	}
	if ctx.Err() != nil {
		return ErrSnapshot
	}
	if err = s.claim(r.RequestID); err != nil {
		return err
	}
	// A random confined temporary filename never contains a source identity.
	tmp := uuid.NewString() + ".tmp"
	f, err := s.root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return ErrSnapshot
	}
	defer s.root.Remove(tmp)
	_, err = f.Write(sealed)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil || ctx.Err() != nil {
		return ErrSnapshot
	}
	if err = s.root.Rename(tmp, name); err != nil {
		return ErrSnapshot
	}
	return snapshotSyncDirectory(s.root)
}

func snapshotSyncDirectory(root *os.Root) error {
	f, err := root.Open(".")
	if err != nil {
		return ErrSnapshot
	}
	defer f.Close()
	if f.Sync() != nil {
		return ErrSnapshot
	}
	return nil
}

func (s *SnapshotStore) Read(ctx context.Context, request domain.MobileBackupRequest, account string) (SelectedArchive, error) {
	if !s.lock(ctx) {
		return SelectedArchive{}, ErrSnapshot
	}
	defer s.unlock()
	if s.root == nil || ctx == nil || ctx.Err() != nil {
		return SelectedArchive{}, ErrSnapshot
	}
	r, err := request.Normalize()
	if err != nil || !canonicalIdentity(account) {
		return SelectedArchive{}, ErrSnapshot
	}
	return s.read(ctx, r, account)
}

func (s *SnapshotStore) read(ctx context.Context, r domain.MobileBackupRequest, account string) (SelectedArchive, error) {
	name, ok := snapshotName(r.RequestID)
	if !ok {
		return SelectedArchive{}, ErrSnapshot
	}
	if !s.claims[r.RequestID] {
		return SelectedArchive{}, ErrSnapshot
	}
	meta, data, err := s.decode(name, r.RequestID)
	if err != nil {
		return SelectedArchive{}, err
	}
	if s.now().UnixMilli() >= meta.ExpiresMS {
		clear(data)
		if s.root.Remove(name) == nil {
			_ = snapshotSyncDirectory(s.root)
		}
		return SelectedArchive{}, ErrSnapshot
	}
	if ctx.Err() != nil || meta.Account != account || meta.Fingerprint != r.Fingerprint() {
		clear(data)
		return SelectedArchive{}, ErrSnapshotConflict
	}
	return SelectedArchive{requestID: r.RequestID, requestFingerprint: r.Fingerprint(), ref: r.Ref(), snapshotCreatedMS: meta.CreatedMS, snapshotExpiresMS: meta.ExpiresMS, File: ArchiveFile{Name: meta.Filename, Data: data}, CiphertextBytes: meta.CiphertextBytes, ContainerBytes: meta.ContainerBytes, TrailingBytes: meta.TrailingBytes}, nil
}

func (s *SnapshotStore) decode(name, id string) (snapshotMetadata, []byte, error) {
	var meta snapshotMetadata
	info, err := s.root.Lstat(name)
	maxBytes := int64(MaxFileBytes) + snapshotMetadataLimit + 64
	if err != nil || !privateRegular(info) || info.Size() < int64(len(snapshotMagic)+s.aead.NonceSize()+s.aead.Overhead()+4) || info.Size() > maxBytes {
		return meta, nil, ErrSnapshot
	}
	f, err := s.root.Open(name)
	if err != nil {
		return meta, nil, ErrSnapshot
	}
	if !samePrivateFile(f, info) {
		f.Close()
		return meta, nil, ErrSnapshot
	}
	sealed, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	f.Close()
	defer clear(sealed)
	if err != nil || int64(len(sealed)) != info.Size() || string(sealed[:len(snapshotMagic)]) != snapshotMagic {
		return meta, nil, ErrSnapshot
	}
	offset := len(snapshotMagic) + s.aead.NonceSize()
	plain, err := s.aead.Open(nil, sealed[len(snapshotMagic):offset], sealed[offset:], []byte(snapshotMagic+id))
	if err != nil || len(plain) < 4 {
		clear(plain)
		return meta, nil, ErrSnapshot
	}
	size := int(binary.BigEndian.Uint32(plain[:4]))
	if size < 1 || size > snapshotMetadataLimit || size > len(plain)-4 || json.Unmarshal(plain[4:4+size], &meta) != nil {
		clear(plain)
		return snapshotMetadata{}, nil, ErrSnapshot
	}
	data := plain[4+size:]
	digest := sha256.Sum256(data)
	if !canonicalIdentity(meta.Account) || !validName(meta.Filename) || len(meta.Fingerprint) != 64 || meta.CreatedMS <= 0 || meta.ExpiresMS-meta.CreatedMS != snapshotLifetime.Milliseconds() || uint64(len(data)) > MaxFileBytes || meta.Digest != hex.EncodeToString(digest[:]) || !standaloneSQLite(data) {
		clear(plain)
		return snapshotMetadata{}, nil, ErrSnapshot
	}
	clear(plain[:4+size])
	return meta, data[:len(data):len(data)], nil
}

// sweep counts bounded owned artifacts and removes only authenticated expired
// snapshots. Unknown files, corruption and symlinks fail closed.
func (s *SnapshotStore) sweep(ctx context.Context) (int, int64, error) {
	f, err := s.root.Open(".")
	if err != nil {
		return 0, 0, ErrSnapshot
	}
	entries, err := f.ReadDir(maxSnapshots + 4)
	f.Close()
	if err != nil && err != io.EOF || len(entries) > maxSnapshots+3 {
		return 0, 0, ErrSnapshotCapacity
	}
	count := 0
	var used int64
	for _, entry := range entries {
		if ctx.Err() != nil {
			return 0, 0, ErrSnapshot
		}
		name := entry.Name()
		if name == snapshotKeyName || name == snapshotClaimsName {
			continue
		}
		if strings.HasSuffix(name, ".tmp") {
			id := strings.TrimSuffix(name, ".tmp")
			_, ok := snapshotName(id)
			info, e := s.root.Lstat(name)
			if !ok || e != nil || !privateRegular(info) || s.root.Remove(name) != nil {
				return 0, 0, ErrSnapshot
			}
			continue
		}
		id := strings.TrimSuffix(name, ".snapshot")
		expected, ok := snapshotName(id)
		if !ok || expected != name || !s.claims[id] {
			return 0, 0, ErrSnapshot
		}
		meta, data, e := s.decode(name, id)
		clear(data)
		if e != nil {
			return 0, 0, e
		}
		if s.now().UnixMilli() >= meta.ExpiresMS {
			if s.root.Remove(name) != nil {
				return 0, 0, ErrSnapshot
			}
			continue
		}
		info, e := s.root.Lstat(name)
		if e != nil {
			return 0, 0, ErrSnapshot
		}
		count++
		used += info.Size()
	}
	if snapshotSyncDirectory(s.root) != nil {
		return 0, 0, ErrSnapshot
	}
	return count, used, nil
}

func (s *SnapshotStore) Cleanup(ctx context.Context) error {
	if !s.lock(ctx) {
		return ErrSnapshot
	}
	defer s.unlock()
	if s.root == nil || ctx == nil || ctx.Err() != nil {
		return ErrSnapshot
	}
	_, _, err := s.sweep(ctx)
	return err
}

func (s *SnapshotStore) Remove(ctx context.Context, request domain.MobileBackupRequest, account string) error {
	if !s.lock(ctx) {
		return ErrSnapshot
	}
	defer s.unlock()
	if s.root == nil || ctx == nil || ctx.Err() != nil {
		return ErrSnapshot
	}
	r, e := request.Normalize()
	if e != nil || !canonicalIdentity(account) {
		return ErrSnapshot
	}
	prior, e := s.read(ctx, r, account)
	defer prior.Clear()
	if e != nil {
		return e
	}
	name, _ := snapshotName(r.RequestID)
	if s.root.Remove(name) != nil {
		return ErrSnapshot
	}
	return snapshotSyncDirectory(s.root)
}

func (s *SnapshotStore) Close() error {
	if s == nil || s.mu == nil {
		return nil
	}
	if !s.lock(context.Background()) {
		return ErrSnapshot
	}
	defer s.unlock()
	if s.root == nil {
		return nil
	}
	err := s.root.Close()
	s.root = nil
	s.aead = nil
	return err
}
