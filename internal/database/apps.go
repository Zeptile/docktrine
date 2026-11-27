package database

import (
	"database/sql"
	"encoding/json"
	"time"

	"github.com/Zeptile/docktrine/internal/manifest"
)

const (
	DeploymentPending   = "pending"
	DeploymentRunning   = "running"
	DeploymentSucceeded = "succeeded"
	DeploymentFailed    = "failed"
)

type Environment struct {
	ID                  int64      `json:"id"`
	App                 string     `json:"app"`
	Name                string     `json:"name"`
	Server              string     `json:"server"`
	IsPreview           bool       `json:"is_preview"`
	Branch              string     `json:"branch,omitempty"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	CurrentDeploymentID *int64     `json:"current_deployment_id,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type Deployment struct {
	ID            int64          `json:"id"`
	EnvironmentID int64          `json:"environment_id"`
	App           string         `json:"app"`
	Environment   string         `json:"environment"`
	Image         string         `json:"image"`
	Spec          *manifest.Spec `json:"spec"`
	Status        string         `json:"status"`
	Message       string         `json:"message,omitempty"`
	Log           string         `json:"log,omitempty"`
	CreatedBy     string         `json:"created_by,omitempty"`
	CreatedAt     time.Time      `json:"created_at"`
	FinishedAt    *time.Time     `json:"finished_at,omitempty"`
}

const environmentColumns = `e.id, a.name, e.name, e.server, e.is_preview, COALESCE(e.branch, ''),
	e.expires_at, e.current_deployment_id, e.created_at, e.updated_at`

func scanEnvironment(row interface{ Scan(...any) error }) (*Environment, error) {
	var e Environment
	var expires sql.NullTime
	var current sql.NullInt64
	err := row.Scan(&e.ID, &e.App, &e.Name, &e.Server, &e.IsPreview, &e.Branch,
		&expires, &current, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if expires.Valid {
		e.ExpiresAt = &expires.Time
	}
	if current.Valid {
		e.CurrentDeploymentID = &current.Int64
	}
	return &e, nil
}

// UpsertEnvironment creates the app and environment rows if needed and
// refreshes the server and preview settings from the spec.
func (db *DB) UpsertEnvironment(spec *manifest.Spec) (*Environment, error) {
	tx, err := db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT OR IGNORE INTO apps (name) VALUES (?)`, spec.App); err != nil {
		return nil, err
	}

	var expires any
	if spec.Preview && spec.TTL != "" {
		ttl, _ := time.ParseDuration(spec.TTL)
		expires = time.Now().UTC().Add(ttl)
	}

	_, err = tx.Exec(`
		INSERT INTO environments (app_id, name, server, is_preview, branch, expires_at)
		VALUES ((SELECT id FROM apps WHERE name = ?), ?, ?, ?, ?, ?)
		ON CONFLICT(app_id, name) DO UPDATE SET
			server = excluded.server,
			is_preview = excluded.is_preview,
			branch = excluded.branch,
			expires_at = excluded.expires_at,
			updated_at = CURRENT_TIMESTAMP`,
		spec.App, spec.Environment, spec.Node, spec.Preview, spec.Branch, expires)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return db.GetEnvironment(spec.App, spec.Environment)
}

func (db *DB) GetEnvironment(app, name string) (*Environment, error) {
	e, err := scanEnvironment(db.QueryRow(`
		SELECT `+environmentColumns+`
		FROM environments e JOIN apps a ON a.id = e.app_id
		WHERE a.name = ? AND e.name = ?`, app, name))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return e, err
}

func (db *DB) ListEnvironments() ([]Environment, error) {
	rows, err := db.Query(`
		SELECT ` + environmentColumns + `
		FROM environments e JOIN apps a ON a.id = e.app_id
		ORDER BY a.name, e.is_preview, e.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	envs := []Environment{}
	for rows.Next() {
		e, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		envs = append(envs, *e)
	}
	return envs, rows.Err()
}

func (db *DB) ListExpiredPreviews(now time.Time) ([]Environment, error) {
	rows, err := db.Query(`
		SELECT `+environmentColumns+`
		FROM environments e JOIN apps a ON a.id = e.app_id
		WHERE e.is_preview = 1 AND e.expires_at IS NOT NULL AND e.expires_at < ?`, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	envs := []Environment{}
	for rows.Next() {
		e, err := scanEnvironment(rows)
		if err != nil {
			return nil, err
		}
		envs = append(envs, *e)
	}
	return envs, rows.Err()
}

// DeleteEnvironment removes the environment, its deployments and proxy host
// records, and the app itself once it has no environments left.
func (db *DB) DeleteEnvironment(id int64) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var appID int64
	if err := tx.QueryRow(`SELECT app_id FROM environments WHERE id = ?`, id).Scan(&appID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM environments WHERE id = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`
		DELETE FROM apps WHERE id = ? AND NOT EXISTS (SELECT 1 FROM environments WHERE app_id = ?)`,
		appID, appID); err != nil {
		return err
	}
	return tx.Commit()
}

const deploymentColumns = `d.id, d.environment_id, a.name, e.name, COALESCE(d.image, ''), d.spec,
	d.status, COALESCE(d.message, ''), COALESCE(d.log, ''), COALESCE(d.created_by, ''),
	d.created_at, d.finished_at`

const deploymentJoins = `FROM deployments d
	JOIN environments e ON e.id = d.environment_id
	JOIN apps a ON a.id = e.app_id`

func scanDeployment(row interface{ Scan(...any) error }) (*Deployment, error) {
	var d Deployment
	var spec string
	var finished sql.NullTime
	err := row.Scan(&d.ID, &d.EnvironmentID, &d.App, &d.Environment, &d.Image, &spec,
		&d.Status, &d.Message, &d.Log, &d.CreatedBy, &d.CreatedAt, &finished)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(spec), &d.Spec); err != nil {
		return nil, err
	}
	if finished.Valid {
		d.FinishedAt = &finished.Time
	}
	return &d, nil
}

func (db *DB) CreateDeployment(env *Environment, spec *manifest.Spec, createdBy string) (*Deployment, error) {
	data, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}

	image := spec.Image
	if spec.Source != nil {
		image = spec.Source.Label()
	} else if image == "" {
		image = "compose"
	}

	result, err := db.Exec(`
		INSERT INTO deployments (environment_id, image, spec, status, created_by)
		VALUES (?, ?, ?, ?, ?)`,
		env.ID, image, string(data), DeploymentPending, createdBy)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	return db.GetDeployment(id)
}

func (db *DB) GetDeployment(id int64) (*Deployment, error) {
	d, err := scanDeployment(db.QueryRow(`SELECT `+deploymentColumns+` `+deploymentJoins+` WHERE d.id = ?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// ListDeployments returns the most recent deployments of an environment
// (or of every environment when envID is 0), newest first, without logs.
func (db *DB) ListDeployments(envID int64, limit int) ([]Deployment, error) {
	query := `SELECT ` + deploymentColumns + ` ` + deploymentJoins
	args := []any{}
	if envID != 0 {
		query += ` WHERE d.environment_id = ?`
		args = append(args, envID)
	}
	query += ` ORDER BY d.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	deployments := []Deployment{}
	for rows.Next() {
		d, err := scanDeployment(rows)
		if err != nil {
			return nil, err
		}
		d.Log = ""
		deployments = append(deployments, *d)
	}
	return deployments, rows.Err()
}

// UpdateDeploymentSpec records what a deployment resolved while running,
// such as the commit of a stack from Git.
func (db *DB) UpdateDeploymentSpec(id int64, image string, spec *manifest.Spec) error {
	data, err := json.Marshal(spec)
	if err != nil {
		return err
	}
	_, err = db.Exec(`UPDATE deployments SET image = ?, spec = ? WHERE id = ?`, image, string(data), id)
	return err
}

func (db *DB) AppendDeploymentLog(id int64, line string) error {
	_, err := db.Exec(`UPDATE deployments SET log = log || ? WHERE id = ?`, line+"\n", id)
	return err
}

func (db *DB) SetDeploymentStatus(id int64, status, message string) error {
	var finished any
	if status == DeploymentSucceeded || status == DeploymentFailed {
		finished = time.Now().UTC()
	}
	_, err := db.Exec(`UPDATE deployments SET status = ?, message = ?, finished_at = ? WHERE id = ?`,
		status, message, finished, id)
	return err
}

func (db *DB) SetCurrentDeployment(envID, deploymentID int64) error {
	_, err := db.Exec(`
		UPDATE environments SET current_deployment_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?`, deploymentID, envID)
	return err
}

// FailStaleDeployments marks deployments left running by a previous process
// as failed, so they don't block new deploys after a restart.
func (db *DB) FailStaleDeployments() error {
	_, err := db.Exec(`
		UPDATE deployments SET status = ?, message = 'interrupted by server restart', finished_at = ?
		WHERE status IN (?, ?)`,
		DeploymentFailed, time.Now().UTC(), DeploymentPending, DeploymentRunning)
	return err
}

func (db *DB) HasActiveDeployment(envID int64) (bool, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM deployments WHERE environment_id = ? AND status IN (?, ?)`,
		envID, DeploymentPending, DeploymentRunning).Scan(&count)
	return count > 0, err
}

type ProxyHost struct {
	Domain string
	NpmID  int64
}

func (db *DB) ListProxyHosts(envID int64) ([]ProxyHost, error) {
	rows, err := db.Query(`SELECT domain, npm_id FROM proxy_hosts WHERE environment_id = ?`, envID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	hosts := []ProxyHost{}
	for rows.Next() {
		var h ProxyHost
		if err := rows.Scan(&h.Domain, &h.NpmID); err != nil {
			return nil, err
		}
		hosts = append(hosts, h)
	}
	return hosts, rows.Err()
}

func (db *DB) SaveProxyHost(envID int64, domain string, npmID int64) error {
	_, err := db.Exec(`
		INSERT INTO proxy_hosts (environment_id, domain, npm_id) VALUES (?, ?, ?)
		ON CONFLICT(domain) DO UPDATE SET environment_id = excluded.environment_id, npm_id = excluded.npm_id`,
		envID, domain, npmID)
	return err
}

func (db *DB) DeleteProxyHost(domain string) error {
	_, err := db.Exec(`DELETE FROM proxy_hosts WHERE domain = ?`, domain)
	return err
}
