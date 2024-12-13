package database

import (
	"database/sql"
	"time"
)

type Server struct {
	ID          int64
	Name        string
	Host        string
	Description string
	IsDefault   bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (db *DB) GetServers() ([]Server, error) {
	rows, err := db.Query(`SELECT id, name, host, description, is_default, created_at, updated_at FROM servers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []Server
	for rows.Next() {
		var s Server
		err := rows.Scan(&s.ID, &s.Name, &s.Host, &s.Description, &s.IsDefault, &s.CreatedAt, &s.UpdatedAt)
		if err != nil {
			return nil, err
		}
		servers = append(servers, s)
	}
	return servers, nil
}

func (db *DB) GetServerByName(name string) (*Server, error) {
	var s Server
	err := db.QueryRow(`
		SELECT id, name, host, description, is_default, created_at, updated_at 
		FROM servers WHERE name = ?`, name).Scan(
		&s.ID, &s.Name, &s.Host, &s.Description, &s.IsDefault, &s.CreatedAt, &s.UpdatedAt)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (db *DB) GetDefaultServer() (*Server, error) {
	var s Server
	err := db.QueryRow(`
		SELECT id, name, host, description, is_default, created_at, updated_at 
		FROM servers WHERE is_default = 1`).Scan(
		&s.ID, &s.Name, &s.Host, &s.Description, &s.IsDefault, &s.CreatedAt, &s.UpdatedAt)
	
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (db *DB) CreateServer(server *Server) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if server.IsDefault {
		_, err = tx.Exec(`UPDATE servers SET is_default = 0`)
		if err != nil {
			return err
		}
	}

	result, err := tx.Exec(`
		INSERT INTO servers (name, host, description, is_default)
		VALUES (?, ?, ?, ?)`,
		server.Name, server.Host, server.Description, server.IsDefault)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	server.ID = id

	return tx.Commit()
} 