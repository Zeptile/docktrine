package database

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Zeptile/docktrine/internal/manifest"
)

const (
	BackupRunning   = "running"
	BackupSucceeded = "succeeded"
	BackupFailed    = "failed"
)

// Kinds of what a backup holds.
const (
	BackupItemVolume = "volume"
	// The stack's directory on the node, for stacks from Git
	BackupItemStack = "stack"
)

// Backups are keyed by app and environment name rather than by the
// environment's row, so they outlive destroying the environment: deploying
// it again finds them.
const backupsTable = `CREATE TABLE IF NOT EXISTS backups (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	app TEXT NOT NULL,
	environment TEXT NOT NULL,
	server TEXT NOT NULL,
	path TEXT NOT NULL,
	reason TEXT NOT NULL,
	status TEXT NOT NULL,
	message TEXT,
	deployment_id INTEGER,
	triggered_by INTEGER,
	spec TEXT,
	items TEXT NOT NULL DEFAULT '[]',
	skipped TEXT NOT NULL DEFAULT '[]',
	size INTEGER NOT NULL DEFAULT 0,
	created_by TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	finished_at DATETIME,
	restored_at DATETIME
)`

const backupsIndex = `CREATE INDEX IF NOT EXISTS backups_environment ON backups (app, environment)`

// BackupItem is one archive in a backup.
type BackupItem struct {
	Kind string `json:"kind"`
	// The volume's name; the directory's path on the node for the stack
	Name string `json:"name"`
	// The archive, in the backup's directory
	File string `json:"file"`
	// The volume's labels, to create it as it was if it's gone on restore
	Labels map[string]string `json:"labels,omitempty"`
	// Bytes in the volume, and in its archive
	Size        int64 `json:"size"`
	ArchiveSize int64 `json:"archive_size"`
}

// SkippedMount is something the environment's containers mount that a
// backup leaves out, such as a directory on the node.
type SkippedMount struct {
	Type   string `json:"type"`
	Source string `json:"source"`
	Target string `json:"target"`
	// The service or container that mounts it
	Container string `json:"container,omitempty"`
	Reason    string `json:"reason"`
}

type Backup struct {
	ID          int64  `json:"id"`
	App         string `json:"app"`
	Environment string `json:"environment"`
	Server      string `json:"server"`
	// The backup's directory on the node
	Path    string `json:"path"`
	Reason  string `json:"reason"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
	// The deployment that was running when it was taken, and its spec, so
	// restoring can bring back the code that goes with the data
	DeploymentID *int64         `json:"deployment_id,omitempty"`
	Spec         *manifest.Spec `json:"spec,omitempty"`
	// The deploy that took it, if one did
	TriggeredBy *int64         `json:"triggered_by,omitempty"`
	Items       []BackupItem   `json:"items"`
	Skipped     []SkippedMount `json:"skipped"`
	// Bytes of all its archives
	Size       int64      `json:"size"`
	CreatedBy  string     `json:"created_by,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	RestoredAt *time.Time `json:"restored_at,omitempty"`
}

const backupColumns = `id, app, environment, server, path, reason, status, COALESCE(message, ''),
	deployment_id, triggered_by, COALESCE(spec, ''), items, skipped, size, COALESCE(created_by, ''),
	created_at, finished_at, restored_at`

func scanBackup(row interface{ Scan(...any) error }) (*Backup, error) {
	var b Backup
	var deployment, triggered sql.NullInt64
	var spec, items, skipped string
	var finished, restored sql.NullTime
	err := row.Scan(&b.ID, &b.App, &b.Environment, &b.Server, &b.Path, &b.Reason, &b.Status, &b.Message,
		&deployment, &triggered, &spec, &items, &skipped, &b.Size, &b.CreatedBy,
		&b.CreatedAt, &finished, &restored)
	if err != nil {
		return nil, err
	}
	if deployment.Valid {
		b.DeploymentID = &deployment.Int64
	}
	if triggered.Valid {
		b.TriggeredBy = &triggered.Int64
	}
	if spec != "" {
		if err := json.Unmarshal([]byte(spec), &b.Spec); err != nil {
			return nil, err
		}
	}
	if err := json.Unmarshal([]byte(items), &b.Items); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(skipped), &b.Skipped); err != nil {
		return nil, err
	}
	if finished.Valid {
		b.FinishedAt = &finished.Time
	}
	if restored.Valid {
		b.RestoredAt = &restored.Time
	}
	return &b, nil
}

// CreateBackup records a backup that is starting.
func (db *DB) CreateBackup(b *Backup) (*Backup, error) {
	var spec any
	if b.Spec != nil {
		data, err := json.Marshal(b.Spec)
		if err != nil {
			return nil, err
		}
		spec = string(data)
	}
	result, err := db.Exec(`
		INSERT INTO backups (app, environment, server, path, reason, status, deployment_id, triggered_by, spec, created_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		b.App, b.Environment, b.Server, b.Path, b.Reason, BackupRunning, b.DeploymentID, b.TriggeredBy, spec, b.CreatedBy)
	if err != nil {
		return nil, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetBackup(id)
}

// FinishBackup records how a backup ended and what it holds.
func (db *DB) FinishBackup(id int64, status, message string, items []BackupItem, skipped []SkippedMount) error {
	if items == nil {
		items = []BackupItem{}
	}
	if skipped == nil {
		skipped = []SkippedMount{}
	}
	itemsJSON, err := json.Marshal(items)
	if err != nil {
		return err
	}
	skippedJSON, err := json.Marshal(skipped)
	if err != nil {
		return err
	}
	var size int64
	for _, item := range items {
		size += item.ArchiveSize
	}
	_, err = db.Exec(`
		UPDATE backups SET status = ?, message = ?, items = ?, skipped = ?, size = ?, finished_at = ?
		WHERE id = ?`,
		status, message, string(itemsJSON), string(skippedJSON), size, time.Now().UTC(), id)
	return err
}

func (db *DB) GetBackup(id int64) (*Backup, error) {
	b, err := scanBackup(db.QueryRow(`SELECT `+backupColumns+` FROM backups WHERE id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return b, err
}

// ListBackups returns an environment's backups, newest first.
func (db *DB) ListBackups(app, env string) ([]Backup, error) {
	rows, err := db.Query(`SELECT `+backupColumns+` FROM backups
		WHERE app = ? AND environment = ? ORDER BY id DESC`, app, env)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	backups := []Backup{}
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		backups = append(backups, *b)
	}
	return backups, rows.Err()
}

func (db *DB) DeleteBackup(id int64) error {
	_, err := db.Exec(`DELETE FROM backups WHERE id = ?`, id)
	return err
}

func (db *DB) SetBackupRestored(id int64) error {
	_, err := db.Exec(`UPDATE backups SET restored_at = ? WHERE id = ?`, time.Now().UTC(), id)
	return err
}

// FailStaleBackups marks backups left running by a previous process as
// failed; their archives may be incomplete.
func (db *DB) FailStaleBackups() error {
	_, err := db.Exec(`
		UPDATE backups SET status = ?, message = 'interrupted by server restart', finished_at = ?
		WHERE status = ?`,
		BackupFailed, time.Now().UTC(), BackupRunning)
	return err
}
