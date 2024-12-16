package database

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

type APIKey struct {
	ID          int64     `json:"id"`
	Key         string    `json:"key"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	LastUsedAt  *time.Time `json:"last_used_at"`
}

func generateAPIKey() string {
	bytes := make([]byte, 32)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func (db *DB) GetAPIKey(key string) (*APIKey, error) {
	var apiKey APIKey
	err := db.QueryRow(`
		SELECT id, key, description, created_at, last_used_at 
		FROM api_keys WHERE key = ?`, key).Scan(
		&apiKey.ID, &apiKey.Key, &apiKey.Description, &apiKey.CreatedAt, &apiKey.LastUsedAt)
	
	if err != nil {
		return nil, err
	}
	return &apiKey, nil
}

func (db *DB) CreateAPIKey(description string) (*APIKey, error) {
	key := generateAPIKey()
	result, err := db.Exec(`
		INSERT INTO api_keys (key, description)
		VALUES (?, ?)`,
		key, description)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &APIKey{
		ID:          id,
		Key:         key,
		Description: description,
		CreatedAt:   time.Now(),
	}, nil
}

func (db *DB) UpdateAPIKeyLastUsed(key string) error {
	_, err := db.Exec(`
		UPDATE api_keys 
		SET last_used_at = CURRENT_TIMESTAMP 
		WHERE key = ?`, key)
	return err
}