package database

import (
	"database/sql"
	"encoding/json"
	"time"
)

// Outcomes of checking an environment for a newer version of what it runs.
const (
	UpdateCurrent   = "current"
	UpdateAvailable = "available"
	// Nothing it runs can move: a commit, or images by digest
	UpdatePinned = "pinned"
	UpdateError  = "error"
)

// UpdateCheck is the last look at whether an environment runs the newest
// commit of its Git ref and the newest images behind its tags. It holds for
// the deployment it compared against; a later deploy makes it stale.
type UpdateCheck struct {
	EnvironmentID int64         `json:"-"`
	DeploymentID  int64         `json:"deployment_id"`
	Status        string        `json:"status"`
	Git           *GitUpdate    `json:"git,omitempty"`
	Images        []ImageUpdate `json:"images,omitempty"`
	Error         string        `json:"error,omitempty"`
	// Checks that failed in a row, which spaces out the next ones
	Failures    int       `json:"-"`
	CheckedAt   time.Time `json:"checked_at"`
	NextCheckAt time.Time `json:"next_check_at"`
}

// GitUpdate compares the commit a stack from Git runs with its ref's head.
type GitUpdate struct {
	Repo string `json:"repo"`
	Ref  string `json:"ref,omitempty"`
	// "branch", "tag", "default" (the remote's default branch) or "commit"
	RefKind string `json:"ref_kind"`
	Current string `json:"current"`
	Latest  string `json:"latest,omitempty"`
	// How many commits the ref is ahead of what runs; 0 when not known
	Behind     int    `json:"behind,omitempty"`
	CompareURL string `json:"compare_url,omitempty"`
	Update     bool   `json:"update"`
}

// ImageUpdate compares the image a container runs with what its tag points
// to in the registry now.
type ImageUpdate struct {
	// The compose service, for stacks
	Service string `json:"service,omitempty"`
	Image   string `json:"image"`
	Current string `json:"current,omitempty"`
	Latest  string `json:"latest,omitempty"`
	Update  bool   `json:"update"`
	Error   string `json:"error,omitempty"`
}

// updateChecksTable caches update checks, one row per environment.
const updateChecksTable = `CREATE TABLE IF NOT EXISTS update_checks (
	environment_id INTEGER PRIMARY KEY REFERENCES environments(id) ON DELETE CASCADE,
	deployment_id INTEGER NOT NULL,
	status TEXT NOT NULL,
	result TEXT NOT NULL DEFAULT '{}',
	error TEXT,
	failures INTEGER NOT NULL DEFAULT 0,
	checked_at DATETIME NOT NULL,
	next_check_at DATETIME NOT NULL
)`

// The parts of a check kept as JSON, since they vary with what it checked
type updateResult struct {
	Git    *GitUpdate    `json:"git,omitempty"`
	Images []ImageUpdate `json:"images,omitempty"`
}

const updateCheckColumns = `environment_id, deployment_id, status, result, COALESCE(error, ''),
	failures, checked_at, next_check_at`

func scanUpdateCheck(row interface{ Scan(...any) error }) (*UpdateCheck, error) {
	var c UpdateCheck
	var result string
	err := row.Scan(&c.EnvironmentID, &c.DeploymentID, &c.Status, &result, &c.Error,
		&c.Failures, &c.CheckedAt, &c.NextCheckAt)
	if err != nil {
		return nil, err
	}
	var r updateResult
	if err := json.Unmarshal([]byte(result), &r); err != nil {
		return nil, err
	}
	c.Git, c.Images = r.Git, r.Images
	return &c, nil
}

// GetUpdateCheck returns the last check of an environment, or nil.
func (db *DB) GetUpdateCheck(envID int64) (*UpdateCheck, error) {
	c, err := scanUpdateCheck(db.QueryRow(
		`SELECT `+updateCheckColumns+` FROM update_checks WHERE environment_id = ?`, envID))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return c, err
}

// ListUpdateChecks returns every environment's last check, by environment.
func (db *DB) ListUpdateChecks() (map[int64]*UpdateCheck, error) {
	rows, err := db.Query(`SELECT ` + updateCheckColumns + ` FROM update_checks`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	checks := map[int64]*UpdateCheck{}
	for rows.Next() {
		c, err := scanUpdateCheck(rows)
		if err != nil {
			return nil, err
		}
		checks[c.EnvironmentID] = c
	}
	return checks, rows.Err()
}

// SaveUpdateCheck replaces an environment's last check.
func (db *DB) SaveUpdateCheck(c *UpdateCheck) error {
	data, err := json.Marshal(updateResult{Git: c.Git, Images: c.Images})
	if err != nil {
		return err
	}
	_, err = db.Exec(`
		INSERT INTO update_checks (environment_id, deployment_id, status, result, error, failures, checked_at, next_check_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(environment_id) DO UPDATE SET
			deployment_id = excluded.deployment_id,
			status = excluded.status,
			result = excluded.result,
			error = excluded.error,
			failures = excluded.failures,
			checked_at = excluded.checked_at,
			next_check_at = excluded.next_check_at`,
		c.EnvironmentID, c.DeploymentID, c.Status, string(data), c.Error, c.Failures,
		c.CheckedAt.UTC(), c.NextCheckAt.UTC())
	return err
}
