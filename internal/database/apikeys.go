package database

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Keys are stored as a SHA-256 hash; the key itself is only shown once, when
// it is created. The prefix identifies a key in listings.
const (
	apiKeyPrefix     = "dkt_"
	apiKeyShownChars = 12
)

var (
	ErrAPIKeyNameTaken = errors.New("an API key with this name already exists")
	ErrLastAPIKey      = errors.New("can't revoke the last API key")
)

type APIKey struct {
	ID         int64      `json:"id"`
	Name       string     `json:"name"`
	Prefix     string     `json:"prefix"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at"`
}

// NewAPIKey is returned on creation only, with the key in clear.
type NewAPIKey struct {
	APIKey
	Key string `json:"key"`
}

func generateAPIKey() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return apiKeyPrefix + hex.EncodeToString(bytes), nil
}

func hashAPIKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

func displayPrefix(key string) string {
	if len(key) > apiKeyShownChars {
		return key[:apiKeyShownChars]
	}
	return key
}

const apiKeyColumns = `id, name, prefix, created_at, last_used_at`

func scanAPIKey(row interface{ Scan(...any) error }) (*APIKey, error) {
	var k APIKey
	var lastUsed sql.NullTime
	if err := row.Scan(&k.ID, &k.Name, &k.Prefix, &k.CreatedAt, &lastUsed); err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		k.LastUsedAt = &lastUsed.Time
	}
	return &k, nil
}

// AuthenticateAPIKey returns the key matching a presented secret, or nil.
func (db *DB) AuthenticateAPIKey(key string) (*APIKey, error) {
	k, err := scanAPIKey(db.QueryRow(
		`SELECT `+apiKeyColumns+` FROM api_keys WHERE key_hash = ?`, hashAPIKey(key)))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return k, err
}

func (db *DB) ListAPIKeys() ([]APIKey, error) {
	rows, err := db.Query(`SELECT ` + apiKeyColumns + ` FROM api_keys ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	keys := []APIKey{}
	for rows.Next() {
		k, err := scanAPIKey(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, *k)
	}
	return keys, rows.Err()
}

func (db *DB) CreateAPIKey(name string) (*NewAPIKey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}
	key, err := generateAPIKey()
	if err != nil {
		return nil, err
	}

	result, err := db.Exec(`
		INSERT INTO api_keys (name, key_hash, prefix) VALUES (?, ?, ?)`,
		name, hashAPIKey(key), displayPrefix(key))
	if isUniqueViolation(err, "api_keys.name") {
		return nil, ErrAPIKeyNameTaken
	}
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	created, err := scanAPIKey(db.QueryRow(`SELECT `+apiKeyColumns+` FROM api_keys WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	return &NewAPIKey{APIKey: *created, Key: key}, nil
}

// RenameAPIKey returns false if no key has this ID.
func (db *DB) RenameAPIKey(id int64, name string) (bool, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return false, fmt.Errorf("name is required")
	}
	result, err := db.Exec(`UPDATE api_keys SET name = ? WHERE id = ?`, name, id)
	if isUniqueViolation(err, "api_keys.name") {
		return false, ErrAPIKeyNameTaken
	}
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	return n > 0, err
}

// RevokeAPIKey deletes a key, refusing to delete the last one so the API
// can't lock everyone out. Returns false if no key has this ID.
func (db *DB) RevokeAPIKey(id int64) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	var exists, total int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM api_keys WHERE id = ?`, id).Scan(&exists); err != nil {
		return false, err
	}
	if exists == 0 {
		return false, nil
	}
	if err := tx.QueryRow(`SELECT COUNT(*) FROM api_keys`).Scan(&total); err != nil {
		return false, err
	}
	if total <= 1 {
		return false, ErrLastAPIKey
	}
	if _, err := tx.Exec(`DELETE FROM api_keys WHERE id = ?`, id); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// TouchAPIKey records use of a key, at most once a minute to spare the
// database a write on every request.
func (db *DB) TouchAPIKey(id int64) error {
	_, err := db.Exec(`
		UPDATE api_keys SET last_used_at = CURRENT_TIMESTAMP
		WHERE id = ? AND (last_used_at IS NULL OR last_used_at < datetime('now', '-1 minute'))`, id)
	return err
}

func isUniqueViolation(err error, column string) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed: "+column)
}

// migrateAPIKeys moves keys stored in clear (before hashing was introduced)
// to the hashed table layout. Existing keys keep working.
func (db *DB) migrateAPIKeys() error {
	var plaintext int
	err := db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('api_keys') WHERE name = 'key'`).Scan(&plaintext)
	if err != nil || plaintext == 0 {
		return err
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, key, COALESCE(description, ''), created_at, last_used_at FROM api_keys ORDER BY id`)
	if err != nil {
		return err
	}
	type oldKey struct {
		id        int64
		key, name string
		created   time.Time
		lastUsed  sql.NullTime
	}
	var old []oldKey
	for rows.Next() {
		var k oldKey
		if err := rows.Scan(&k.id, &k.key, &k.name, &k.created, &k.lastUsed); err != nil {
			rows.Close()
			return err
		}
		old = append(old, k)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	if _, err := tx.Exec(`ALTER TABLE api_keys RENAME TO api_keys_plaintext`); err != nil {
		return err
	}
	if _, err := tx.Exec(apiKeysTable); err != nil {
		return err
	}

	used := map[string]bool{}
	for _, k := range old {
		// Names became unique; old descriptions may repeat or be empty
		base := k.name
		if base == "" {
			base = "key"
		}
		name := base
		for i := 2; used[name]; i++ {
			name = fmt.Sprintf("%s (%d)", base, i)
		}
		used[name] = true

		var lastUsed any
		if k.lastUsed.Valid {
			lastUsed = k.lastUsed.Time
		}
		if _, err := tx.Exec(`
			INSERT INTO api_keys (id, name, key_hash, prefix, created_at, last_used_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			k.id, name, hashAPIKey(k.key), displayPrefix(k.key), k.created, lastUsed); err != nil {
			return err
		}
	}

	if _, err := tx.Exec(`DROP TABLE api_keys_plaintext`); err != nil {
		return err
	}
	return tx.Commit()
}

const apiKeysTable = `CREATE TABLE IF NOT EXISTS api_keys (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT UNIQUE NOT NULL,
	key_hash TEXT UNIQUE NOT NULL,
	prefix TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_used_at DATETIME
)`
