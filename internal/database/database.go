package database

import (
	"database/sql"
	"os"
	"path/filepath"

	"github.com/Zeptile/docktrine/internal/logger"
	_ "github.com/mattn/go-sqlite3"
)

type DB struct {
	*sql.DB
}

func NewDatabaseConnection() (*DB, error) {
	dataPath := os.Getenv("CONFIG_PATH")
	if dataPath == "" {
		dataPath = "data"
	}

	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataPath, "docktrine.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	database := &DB{db}
	if err := database.createTables(); err != nil {
		return nil, err
	}

	var count int
	err = database.QueryRow("SELECT COUNT(*) FROM servers").Scan(&count)
	if err != nil {
		return nil, err
	}

	if count == 0 {
		defaultServer := &Server{
			Name:        "local",
			Host:        "unix:///var/run/docker.sock",
			Description: "Local Docker daemon",
			IsDefault:   true,
		}
		if err := database.CreateServer(defaultServer); err != nil {
			return nil, err
		}
	}

	var keyCount int
	err = database.QueryRow("SELECT COUNT(*) FROM api_keys").Scan(&keyCount)
	if err != nil {
		return nil, err
	}

	if keyCount == 0 {
		apiKey, err := database.CreateAPIKey("Default API key")
		if err != nil {
			return nil, err
		}
		logger.Info("Created default API key: " + apiKey.Key)
	}

	return database, nil
}

func (db *DB) createTables() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS servers (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			host TEXT NOT NULL,
			description TEXT,
			is_default BOOLEAN DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT UNIQUE NOT NULL,
			description TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME
		)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}

	return nil
} 