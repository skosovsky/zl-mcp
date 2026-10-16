package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
)

// This opt-in check uses only an explicitly marked disposable SQLite backup.
// It never opens a Zalo session, collector, worker, MCP endpoint or real config.
func TestRichUpgradeOnDisposableSnapshot(t *testing.T) {
	path := os.Getenv("ZL_RICH_UPGRADE_SNAPSHOT")
	if path == "" {
		t.Skip("set ZL_RICH_UPGRADE_SNAPSHOT to a marked disposable v16 SQLite backup")
	}
	if !validRichSnapshotPath(path) {
		t.Fatal("refusing a non-disposable snapshot path")
	}
	// Arrange: hash every preexisting table, with no row values in test output.
	u := url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}
	before, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal("snapshot cannot be opened read-only")
	}
	var version int
	if err = before.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil || version != 16 {
		before.Close()
		t.Fatal("snapshot is not the expected v16 source")
	}
	baseline := snapshotTableDigests(t, before)
	oldVersions := snapshotMigrationVersions(t, before)
	if err = before.Close(); err != nil {
		t.Fatal(err)
	}
	// Act: execute the actual candidate migration, without starting any service.
	u.RawQuery = "_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		t.Fatal("candidate snapshot connection failed")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	// Migration uses the database only; no session, policy or runtime is initialized.
	s := &Store{DB: db}
	if err = s.migrate(t.Context()); err != nil {
		t.Fatal("candidate snapshot migration failed")
	}
	after := snapshotTableDigests(t, s.DB)
	if len(after) != len(baseline)+1 {
		t.Fatal("migration created an unexpected table set")
	}
	// Assert: all original table contents survive; only schema/new alias state differs.
	for table, digest := range baseline {
		if table != "schema_migrations" && after[table] != digest {
			t.Fatal("migration changed an original table", table)
		}
	}
	var integrity string
	if err = s.DB.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal("migrated snapshot integrity failure")
	}
	if err = s.DB.QueryRow(`SELECT MAX(version) FROM schema_migrations`).
		Scan(&version); err != nil || version != 17 ||
		countClientAliases(t, s) != 0 {
		t.Fatal("candidate alias migration incomplete")
	}
	assertSnapshotMigrationMarkers(t, s.DB, oldVersions)
	report := map[string]any{
		"source_schema":                   16,
		"candidate_schema":                version,
		"original_table_count":            len(baseline),
		"original_data_tables_identical":  true,
		"original_data_table_count":       len(baseline) - 1,
		"old_migration_markers_preserved": true,
		"aliases_empty":                   true,
		"integrity_ok":                    true,
		"network_or_session_started":      false,
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(filepath.Dir(path), "upgrade-proof.json"), body, 0600); err != nil {
		t.Fatal("snapshot proof could not be saved")
	}
}

func assertSnapshotMigrationMarkers(t *testing.T, db *sql.DB, oldVersions []int) {
	t.Helper()
	newVersions := snapshotMigrationVersions(t, db)
	if len(newVersions) != len(oldVersions)+1 {
		t.Fatal("migration changed earlier version markers")
	}
	for i, old := range oldVersions {
		if newVersions[i] != old {
			t.Fatal("migration changed an earlier version marker")
		}
	}
}

func validRichSnapshotPath(path string) bool {
	if !filepath.IsAbs(path) || filepath.Base(path) != "messages.sqlite" {
		return false
	}
	parent := filepath.Dir(path)
	if !strings.HasPrefix(filepath.Base(parent), "zl-rich-schema17-shadow-") {
		return false
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return false
	}
	if !strings.HasPrefix(filepath.Base(filepath.Dir(resolvedPath)), "zl-rich-schema17-shadow-") {
		return false
	}
	parentInfo, err := os.Stat(filepath.Dir(resolvedPath))
	if err != nil || !parentInfo.IsDir() || parentInfo.Mode().Perm() != 0700 {
		return false
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0600 || !richSnapshotInTemp(resolvedPath) {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Nlink != 1 {
		return false
	}
	marker := filepath.Join(parent, "snapshot-scope.json")
	markerInfo, err := os.Lstat(marker)
	if err != nil || !markerInfo.Mode().IsRegular() || markerInfo.Mode().Perm() != 0600 || markerInfo.Size() > 1024 {
		return false
	}
	body, err := os.ReadFile(marker)
	if err != nil {
		return false
	}
	var scope struct {
		Purpose string `json:"purpose"`
	}
	return json.Unmarshal(body, &scope) == nil && scope.Purpose == "rich-schema17-migration-proof"
}

// A multiset digest avoids assuming rowid or column names on FTS shadow tables.
func snapshotTableDigests(t *testing.T, db *sql.DB) map[string]string {
	t.Helper()
	tables := snapshotTableNames(t, db)
	result := map[string]string{}
	for _, table := range tables {
		result[table] = snapshotTableDigest(t, db, table)
	}
	return result
}

//nolint:unqueryvet // Preservation proof intentionally fingerprints every column of the guarded disposable snapshot.
func snapshotTableDigest(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	quoted := `"` + strings.ReplaceAll(table, `"`, `""`) + `"`
	rows, err := db.Query(`SELECT * FROM ` + quoted)
	if err != nil {
		t.Fatal("snapshot table read failed", table)
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	digests := []string{}
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err = rows.Scan(dest...); err != nil {
			t.Fatal("snapshot row read failed", table)
		}
		typed := make([]any, len(values))
		for i, value := range values {
			typed[i] = []any{fmt.Sprintf("%T", value), value}
		}
		body, encodeErr := json.Marshal(typed)
		if encodeErr != nil {
			t.Fatal("snapshot row digest failed", table)
		}
		digest := sha256.Sum256(body)
		digests = append(digests, hex.EncodeToString(digest[:]))
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	sort.Strings(digests)
	body, err := json.Marshal([]any{columns, digests})
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func TestRichSnapshotGuardRejectsNonDisposableFiles(t *testing.T) {
	for _, name := range []string{"valid", "unmarked", "wrong purpose", "public file", "public directory", "hard link", "file symlink", "marker symlink", "large marker"} {
		t.Run(name, func(t *testing.T) {
			// Arrange: synthetic files only; no account database is used here.
			parent := filepath.Join(t.TempDir(), "zl-rich-schema17-shadow-fixture")
			if err := os.Mkdir(parent, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(parent, "messages.sqlite")
			marker := filepath.Join(parent, "snapshot-scope.json")
			if err := os.WriteFile(path, []byte("synthetic"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(marker, []byte(`{"purpose":"rich-schema17-migration-proof"}`), 0600); err != nil {
				t.Fatal(err)
			}
			if err := mutateSnapshotGuardFixture(t, path, name); err != nil {
				t.Fatal(err)
			}
			// Act.
			accepted := validRichSnapshotPath(path)
			// Assert: migration is gated by scope, private permissions and exact files.
			if accepted != (name == "valid") {
				t.Fatal("unsafe disposable snapshot gate", name)
			}
		})
	}
}

func TestSnapshotDigestDistinguishesSQLiteStorageClasses(t *testing.T) {
	for _, pair := range [][2]any{{int64(1), float64(1)}, {[]byte("abc"), "YWJj"}} {
		// Arrange: these pairs collapse if untyped JSON values alone are hashed.
		s := reactionTargetStore(t)
		if _, err := s.DB.Exec(`CREATE TABLE digest_probe(value)`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.DB.Exec(`INSERT INTO digest_probe VALUES(?)`, pair[0]); err != nil {
			t.Fatal(err)
		}
		before := snapshotTableDigest(t, s.DB, "digest_probe")
		// Act: replace only the SQLite value/type with a JSON-colliding alternative.
		if _, err := s.DB.Exec(`UPDATE digest_probe SET value=?`, pair[1]); err != nil {
			t.Fatal(err)
		}
		after := snapshotTableDigest(t, s.DB, "digest_probe")
		// Assert: preservation proof must distinguish integer/real and blob/text.
		if before == after {
			t.Fatal("snapshot digest lost SQLite type identity")
		}
	}
}

func richSnapshotInTemp(path string) bool {
	for _, root := range []string{os.TempDir(), "/tmp"} {
		resolved, err := filepath.EvalSymlinks(root)
		if err != nil {
			continue
		}
		relative, err := filepath.Rel(resolved, path)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

func snapshotTableNames(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT name FROM sqlite_schema WHERE type='table'`)
	if err != nil {
		t.Fatal("snapshot table discovery failed")
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return tables
}

func mutateSnapshotGuardFixture(t *testing.T, path, name string) error {
	t.Helper()
	parent := filepath.Dir(path)
	marker := filepath.Join(parent, "snapshot-scope.json")
	switch name {
	case "unmarked":
		return os.Remove(marker)
	case "wrong purpose":
		return os.WriteFile(marker, []byte(`{"purpose":"other"}`), 0600)
	case "public file":
		return os.Chmod(path, 0644)
	case "public directory":
		return os.Chmod(parent, 0755)
	case "hard link":
		return os.Link(path, filepath.Join(parent, "other.sqlite"))
	case "file symlink":
		target := filepath.Join(t.TempDir(), "messages.sqlite")
		if err := os.WriteFile(target, []byte("synthetic"), 0600); err != nil {
			return err
		}
		if err := os.Remove(path); err != nil {
			return err
		}
		return os.Symlink(target, path)
	case "marker symlink":
		if err := os.Remove(marker); err != nil {
			return err
		}
		return os.Symlink("not-a-marker", marker)
	case "large marker":
		return os.WriteFile(marker, []byte(strings.Repeat("x", 1025)), 0600)
	default:
		return nil
	}
}
func snapshotMigrationVersions(t *testing.T, db *sql.DB) []int {
	t.Helper()
	rows, err := db.Query(`SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	versions := []int{}
	for rows.Next() {
		var version int
		if err = rows.Scan(&version); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, version)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	return versions
}
