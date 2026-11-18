package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Zeptile/docktrine/internal/logger"
)

type DB struct {
	*sql.DB
}

// DataPath is the directory holding the database and other server state.
func DataPath() string {
	if dataPath := os.Getenv("CONFIG_PATH"); dataPath != "" {
		return dataPath
	}
	return "data"
}

func NewDatabaseConnection() (*DB, error) {
	dataPath := DataPath()

	if err := os.MkdirAll(dataPath, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(dataPath, "docktrine.db")
	db, err := sql.Open("sqlite3", dbPath+"?_foreign_keys=on&_busy_timeout=5000")
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
		keyPath := filepath.Join(dataPath, "initial-api-key")
		if err := os.WriteFile(keyPath, []byte(apiKey.Key+"\n"), 0600); err != nil {
			return nil, err
		}
		logger.Info("Created default API key, saved to " + keyPath + " (delete the file once you have it)")
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
		`CREATE TABLE IF NOT EXISTS apps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS environments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			app_id INTEGER NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
			name TEXT NOT NULL,
			server TEXT NOT NULL,
			is_preview BOOLEAN DEFAULT 0,
			branch TEXT,
			expires_at DATETIME,
			current_deployment_id INTEGER,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(app_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS deployments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			environment_id INTEGER NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
			image TEXT,
			spec TEXT NOT NULL,
			status TEXT NOT NULL,
			message TEXT,
			log TEXT DEFAULT '',
			created_by TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			finished_at DATETIME
		)`,
		`CREATE TABLE IF NOT EXISTS secrets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			app TEXT NOT NULL,
			environment TEXT NOT NULL,
			key TEXT NOT NULL,
			value BLOB NOT NULL,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(app, environment, key)
		)`,
		`CREATE TABLE IF NOT EXISTS proxy_hosts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			environment_id INTEGER NOT NULL REFERENCES environments(id) ON DELETE CASCADE,
			domain TEXT UNIQUE NOT NULL,
			npm_id INTEGER NOT NULL
		)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return err
		}
	}

	return db.migrate()
}

// migrate adds columns introduced after a table was first created.
func (db *DB) migrate() error {
	columns := []struct{ table, column, definition string }{
		{"servers", "ingress_host", "TEXT"},
		{"servers", "ingress_port", "INTEGER"},
	}

	for _, col := range columns {
		var count int
		err := db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`,
			col.table, col.column).Scan(&count)
		if err != nil {
			return err
		}
		if count > 0 {
			continue
		}
		if _, err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s",
			col.table, col.column, col.definition)); err != nil {
			return err
		}
	}

	return nil
} 