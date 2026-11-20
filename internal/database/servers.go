package database

import (
	"database/sql"
	"fmt"
	"time"
)

type Server struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	Host        string `json:"host"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	// IngressHost/IngressPort are where the gateway (NPM) forwards traffic
	// for this node, e.g. the gateway end of an SSH tunnel to the node's ingress.
	IngressHost string    `json:"ingress_host"`
	IngressPort int       `json:"ingress_port"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

const serverColumns = `id, name, host, COALESCE(description, ''), is_default,
	COALESCE(ingress_host, ''), COALESCE(ingress_port, 0), created_at, updated_at`

func scanServer(row interface{ Scan(...any) error }) (*Server, error) {
	var s Server
	err := row.Scan(&s.ID, &s.Name, &s.Host, &s.Description, &s.IsDefault,
		&s.IngressHost, &s.IngressPort, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (db *DB) GetServers() ([]Server, error) {
	rows, err := db.Query(`SELECT ` + serverColumns + ` FROM servers`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	servers := []Server{}
	for rows.Next() {
		s, err := scanServer(rows)
		if err != nil {
			return nil, err
		}
		servers = append(servers, *s)
	}
	return servers, nil
}

func (db *DB) GetServerByName(name string) (*Server, error) {
	s, err := scanServer(db.QueryRow(`SELECT `+serverColumns+` FROM servers WHERE name = ?`, name))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

func (db *DB) GetDefaultServer() (*Server, error) {
	s, err := scanServer(db.QueryRow(`SELECT ` + serverColumns + ` FROM servers WHERE is_default = 1`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return s, err
}

// ResolveServer returns the named server, or the default one when name is empty.
func (db *DB) ResolveServer(name string) (*Server, error) {
	var server *Server
	var err error
	if name != "" {
		server, err = db.GetServerByName(name)
	} else {
		server, err = db.GetDefaultServer()
	}
	if err != nil {
		return nil, err
	}
	if server == nil {
		if name == "" {
			return nil, fmt.Errorf("no default server configured")
		}
		return nil, fmt.Errorf("server %q not found", name)
	}
	return server, nil
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
		INSERT INTO servers (name, host, description, is_default, ingress_host, ingress_port)
		VALUES (?, ?, ?, ?, ?, ?)`,
		server.Name, server.Host, server.Description, server.IsDefault,
		server.IngressHost, server.IngressPort)
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

func (db *DB) UpdateServer(server *Server) error {
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

	_, err = tx.Exec(`
		UPDATE servers SET host = ?, description = ?, is_default = ?,
			ingress_host = ?, ingress_port = ?, updated_at = CURRENT_TIMESTAMP
		WHERE name = ?`,
		server.Host, server.Description, server.IsDefault,
		server.IngressHost, server.IngressPort, server.Name)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (db *DB) DeleteServer(name string) error {
	result, err := db.Exec(`DELETE FROM servers WHERE name = ?`, name)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if affected == 0 {
		return fmt.Errorf("server not found")
	}

	return nil
}
