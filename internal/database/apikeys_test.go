package database

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func openTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CONFIG_PATH", dir)
	db, err := NewDatabaseConnection()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db, dir
}

func TestInitialKeyIsHashed(t *testing.T) {
	db, dir := openTestDB(t)

	data, err := os.ReadFile(filepath.Join(dir, "initial-api-key"))
	if err != nil {
		t.Fatal(err)
	}
	key := strings.TrimSpace(string(data))
	if !strings.HasPrefix(key, apiKeyPrefix) {
		t.Errorf("key %q lacks the %s prefix", key, apiKeyPrefix)
	}

	var stored int
	db.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE key_hash = ? OR prefix = ?`, key, key).Scan(&stored)
	if stored != 0 {
		t.Error("key is stored in clear")
	}

	k, err := db.AuthenticateAPIKey(key)
	if err != nil || k == nil || k.Name != "Default API key" {
		t.Fatalf("authenticate initial key: %+v, %v", k, err)
	}
	if k, _ := db.AuthenticateAPIKey(key + "x"); k != nil {
		t.Error("wrong key authenticated")
	}
}

func TestCreateRenameRevoke(t *testing.T) {
	db, _ := openTestDB(t)

	ci, err := db.CreateAPIKey("ci", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ci.Key, ci.Prefix) || len(ci.Prefix) != apiKeyShownChars {
		t.Errorf("prefix %q doesn't match key %q", ci.Prefix, ci.Key)
	}
	if _, err := db.CreateAPIKey("ci", nil); !errors.Is(err, ErrAPIKeyNameTaken) {
		t.Errorf("duplicate name: got %v", err)
	}
	if _, err := db.CreateAPIKey("  ", nil); err == nil {
		t.Error("blank name accepted")
	}

	if _, err := db.RenameAPIKey(ci.ID, "Default API key"); !errors.Is(err, ErrAPIKeyNameTaken) {
		t.Errorf("rename onto taken name: got %v", err)
	}
	if ok, err := db.RenameAPIKey(ci.ID, "github-actions"); !ok || err != nil {
		t.Fatalf("rename: %v %v", ok, err)
	}
	if k, _ := db.AuthenticateAPIKey(ci.Key); k == nil || k.Name != "github-actions" {
		t.Errorf("renamed key: %+v", k)
	}

	keys, _ := db.ListAPIKeys(nil)
	if len(keys) != 2 {
		t.Fatalf("want 2 keys, got %d", len(keys))
	}
	if ok, err := db.RevokeAPIKey(keys[0].ID); !ok || err != nil {
		t.Fatalf("revoke: %v %v", ok, err)
	}
	if _, err := db.RevokeAPIKey(ci.ID); !errors.Is(err, ErrLastAPIKey) {
		t.Errorf("revoking the last key: got %v", err)
	}
	if ok, _ := db.RevokeAPIKey(9999); ok {
		t.Error("revoked a missing key")
	}
}

func TestTouchIsThrottled(t *testing.T) {
	db, _ := openTestDB(t)
	k, _ := db.CreateAPIKey("laptop", nil)

	if err := db.TouchAPIKey(k.ID); err != nil {
		t.Fatal(err)
	}
	db.Exec(`UPDATE api_keys SET last_used_at = '2000-01-01 00:00:00' WHERE id = ?`, k.ID)
	db.TouchAPIKey(k.ID)
	got, _ := db.AuthenticateAPIKey(k.Key)
	if got.LastUsedAt == nil || got.LastUsedAt.Year() == 2000 {
		t.Errorf("stale last_used_at not refreshed: %v", got.LastUsedAt)
	}

	db.Exec(`UPDATE api_keys SET last_used_at = datetime('now', '-10 seconds') WHERE id = ?`, k.ID)
	before, _ := db.AuthenticateAPIKey(k.Key)
	db.TouchAPIKey(k.ID)
	after, _ := db.AuthenticateAPIKey(k.Key)
	if !after.LastUsedAt.Equal(*before.LastUsedAt) {
		t.Error("recent last_used_at was rewritten")
	}
}

// Databases from before hashing stored keys in clear, in a "key" column.
func TestMigratePlaintextKeys(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONFIG_PATH", dir)

	raw, err := sql.Open("sqlite3", filepath.Join(dir, "docktrine.db"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = raw.Exec(`
		CREATE TABLE api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT UNIQUE NOT NULL,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME
		);
		INSERT INTO api_keys (key, description, last_used_at) VALUES
			('aaaa1111bbbb2222', 'Default API key', '2026-11-01 10:00:00'),
			('cccc3333dddd4444', 'Default API key', NULL),
			('eeee5555ffff6666', NULL, NULL);`)
	raw.Close()
	if err != nil {
		t.Fatal(err)
	}

	db, err := NewDatabaseConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	for key, name := range map[string]string{
		"aaaa1111bbbb2222": "Default API key",
		"cccc3333dddd4444": "Default API key (2)",
		"eeee5555ffff6666": "key",
	} {
		k, err := db.AuthenticateAPIKey(key)
		if err != nil || k == nil {
			t.Fatalf("old key %s no longer works: %v", key, err)
		}
		if k.Name != name || k.Prefix != displayPrefix(key) {
			t.Errorf("old key %s: name %q prefix %q", key, k.Name, k.Prefix)
		}
	}
	if k, _ := db.AuthenticateAPIKey("aaaa1111bbbb2222"); k.LastUsedAt == nil {
		t.Error("last_used_at lost in migration")
	}

	var leftovers int
	db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = 'api_keys_plaintext'`).Scan(&leftovers)
	if leftovers != 0 {
		t.Error("plaintext table left behind")
	}
	if _, err := os.Stat(filepath.Join(dir, "initial-api-key")); err == nil {
		t.Error("migration created a new initial key although keys existed")
	}

	// Running again is a no-op
	db.Close()
	db2, err := NewDatabaseConnection()
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	keys, _ := db2.ListAPIKeys(nil)
	if len(keys) != 3 {
		t.Errorf("want 3 keys after reopening, got %d", len(keys))
	}
}
