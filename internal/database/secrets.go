package database

import "time"

type SecretInfo struct {
	Key       string    `json:"key"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SetSecret stores an already encrypted value.
func (db *DB) SetSecret(app, env, key string, value []byte) error {
	_, err := db.Exec(`
		INSERT INTO secrets (app, environment, key, value) VALUES (?, ?, ?, ?)
		ON CONFLICT(app, environment, key) DO UPDATE SET
			value = excluded.value, updated_at = CURRENT_TIMESTAMP`,
		app, env, key, value)
	return err
}

func (db *DB) DeleteSecret(app, env, key string) (bool, error) {
	result, err := db.Exec(`DELETE FROM secrets WHERE app = ? AND environment = ? AND key = ?`, app, env, key)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	return affected > 0, err
}

// ListSecrets returns secret names only, never values.
func (db *DB) ListSecrets(app, env string) ([]SecretInfo, error) {
	rows, err := db.Query(`
		SELECT key, updated_at FROM secrets WHERE app = ? AND environment = ? ORDER BY key`, app, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	secrets := []SecretInfo{}
	for rows.Next() {
		var s SecretInfo
		if err := rows.Scan(&s.Key, &s.UpdatedAt); err != nil {
			return nil, err
		}
		secrets = append(secrets, s)
	}
	return secrets, rows.Err()
}

// GetSecretValues returns the encrypted values for the given keys.
func (db *DB) GetSecretValues(app, env string) (map[string][]byte, error) {
	rows, err := db.Query(`SELECT key, value FROM secrets WHERE app = ? AND environment = ?`, app, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	values := map[string][]byte{}
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, err
		}
		values[key] = value
	}
	return values, rows.Err()
}
