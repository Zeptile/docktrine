package database

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// Roles, from least to most access. Viewers can read everything except
// secret values (which nobody can read back); deployers can also deploy,
// manage secrets and start or stop containers; admins can also manage
// servers, users and every API key.
const (
	RoleViewer   = "viewer"
	RoleDeployer = "deployer"
	RoleAdmin    = "admin"
)

var roleRank = map[string]int{RoleViewer: 1, RoleDeployer: 2, RoleAdmin: 3}

// RoleAtLeast reports whether role grants at least the access of min.
func RoleAtLeast(role, min string) bool {
	return roleRank[role] >= roleRank[min]
}

func ValidRole(role string) bool {
	return roleRank[role] > 0
}

const (
	InviteLifetime   = 7 * 24 * time.Hour
	SessionLifetime  = 30 * 24 * time.Hour
	MinPasswordChars = 10
)

var (
	ErrUsernameTaken   = errors.New("a user with this username already exists")
	ErrInvalidUsername = errors.New("usernames use lowercase letters, digits, dots, dashes and underscores, up to 32 characters")
	ErrInvalidRole     = errors.New("role must be viewer, deployer or admin")
	ErrLastAdmin       = errors.New("there must always be at least one admin")
	ErrWeakPassword    = fmt.Errorf("passwords need at least %d characters", MinPasswordChars)
	ErrWrongPassword   = errors.New("the current password is wrong")
)

var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,31}$`)

type User struct {
	ID          int64      `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	LastSeenAt  *time.Time `json:"last_seen_at"`
	// Invited users haven't chosen a password yet
	Invited         bool       `json:"invited"`
	InviteExpiresAt *time.Time `json:"invite_expires_at,omitempty"`

	passwordHash string
}

// Name is how the user appears in deploy history and the UI.
func (u *User) Name() string {
	if u.DisplayName != "" {
		return u.DisplayName
	}
	return u.Username
}

const userColumns = `id, username, display_name, role, created_at, last_seen_at,
	COALESCE(password_hash, ''), invite_expires_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	var lastSeen, inviteExpires sql.NullTime
	if err := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.Role, &u.CreatedAt, &lastSeen,
		&u.passwordHash, &inviteExpires); err != nil {
		return nil, err
	}
	if lastSeen.Valid {
		u.LastSeenAt = &lastSeen.Time
	}
	u.Invited = u.passwordHash == ""
	if u.Invited && inviteExpires.Valid {
		u.InviteExpiresAt = &inviteExpires.Time
	}
	return &u, nil
}

func (db *DB) queryUser(where string, args ...any) (*User, error) {
	u, err := scanUser(db.QueryRow(`SELECT `+userColumns+` FROM users WHERE `+where, args...))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return u, err
}

func (db *DB) GetUser(id int64) (*User, error) {
	return db.queryUser(`id = ?`, id)
}

func (db *DB) GetUserByUsername(username string) (*User, error) {
	return db.queryUser(`username = ?`, strings.ToLower(strings.TrimSpace(username)))
}

func (db *DB) ListUsers() ([]User, error) {
	rows, err := db.Query(`SELECT ` + userColumns + ` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

func (db *DB) CountUsers() (int, error) {
	var n int
	err := db.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

// CreateUser adds a user who signs in once they open the returned invite
// token and choose a password.
func (db *DB) CreateUser(username, displayName, role string) (*User, string, error) {
	username = strings.ToLower(strings.TrimSpace(username))
	if !usernamePattern.MatchString(username) {
		return nil, "", ErrInvalidUsername
	}
	if !ValidRole(role) {
		return nil, "", ErrInvalidRole
	}
	token, err := randomToken("")
	if err != nil {
		return nil, "", err
	}

	result, err := db.Exec(`
		INSERT INTO users (username, display_name, role, invite_hash, invite_expires_at)
		VALUES (?, ?, ?, ?, ?)`,
		username, strings.TrimSpace(displayName), role, hashToken(token), time.Now().Add(InviteLifetime))
	if isUniqueViolation(err, "users.username") {
		return nil, "", ErrUsernameTaken
	}
	if err != nil {
		return nil, "", err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, "", err
	}
	u, err := db.GetUser(id)
	return u, token, err
}

// ResetUserPassword clears a user's password, signs them out everywhere and
// returns a new invite token for choosing another one.
func (db *DB) ResetUserPassword(id int64) (string, error) {
	token, err := randomToken("")
	if err != nil {
		return "", err
	}
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	result, err := tx.Exec(`
		UPDATE users SET password_hash = NULL, invite_hash = ?, invite_expires_at = ? WHERE id = ?`,
		hashToken(token), time.Now().Add(InviteLifetime), id)
	if err != nil {
		return "", err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return "", sql.ErrNoRows
	}
	if _, err := tx.Exec(`DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return "", err
	}
	return token, tx.Commit()
}

// GetUserByInvite returns the user an unexpired invite token belongs to.
func (db *DB) GetUserByInvite(token string) (*User, error) {
	return db.queryUser(`invite_hash = ? AND invite_expires_at > ?`, hashToken(token), time.Now())
}

// AcceptInvite sets the password of the invite's user and consumes the invite.
func (db *DB) AcceptInvite(token, password string) (*User, error) {
	u, err := db.GetUserByInvite(token)
	if err != nil || u == nil {
		return nil, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
		UPDATE users SET password_hash = ?, invite_hash = NULL, invite_expires_at = NULL WHERE id = ?`,
		hash, u.ID); err != nil {
		return nil, err
	}
	return db.GetUser(u.ID)
}

func hashPassword(password string) (string, error) {
	if len([]rune(password)) < MinPasswordChars {
		return "", ErrWeakPassword
	}
	// bcrypt ignores everything past 72 bytes
	if len(password) > 72 {
		return "", errors.New("passwords can be at most 72 bytes")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// A hash of a random password, compared against when the username doesn't
// exist so the response takes as long as for a real user.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("docktrine-no-such-user"), bcrypt.DefaultCost)

// CheckPassword returns the user if username and password match.
func (db *DB) CheckPassword(username, password string) (*User, error) {
	u, err := db.GetUserByUsername(username)
	if err != nil {
		return nil, err
	}
	if u == nil || u.passwordHash == "" {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(password))
		return nil, nil
	}
	if bcrypt.CompareHashAndPassword([]byte(u.passwordHash), []byte(password)) != nil {
		return nil, nil
	}
	return u, nil
}

// ChangePassword sets a new password after checking the current one, and
// signs out every other session.
func (db *DB) ChangePassword(id int64, current, next string, keepSession int64) error {
	u, err := db.GetUser(id)
	if err != nil {
		return err
	}
	if u == nil || bcrypt.CompareHashAndPassword([]byte(u.passwordHash), []byte(current)) != nil {
		return ErrWrongPassword
	}
	hash, err := hashPassword(next)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE users SET password_hash = ? WHERE id = ?`, hash, id); err != nil {
		return err
	}
	_, err = db.Exec(`DELETE FROM sessions WHERE user_id = ? AND id != ?`, id, keepSession)
	return err
}

// UpdateUser changes a user's display name and role. Returns false if no
// user has this ID.
func (db *DB) UpdateUser(id int64, displayName, role string) (bool, error) {
	if !ValidRole(role) {
		return false, ErrInvalidRole
	}
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	admins, err := countAdmins(tx)
	if err != nil {
		return false, err
	}
	result, err := tx.Exec(`UPDATE users SET display_name = ?, role = ? WHERE id = ?`,
		strings.TrimSpace(displayName), role, id)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return false, nil
	}
	if err := checkAdminRemains(tx, admins); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// DeleteUser removes a user along with their sessions and API keys.
func (db *DB) DeleteUser(id int64) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	admins, err := countAdmins(tx)
	if err != nil {
		return false, err
	}
	result, err := tx.Exec(`DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return false, nil
	}
	if err := checkAdminRemains(tx, admins); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func countAdmins(tx *sql.Tx) (int, error) {
	var n int
	err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE role = ?`, RoleAdmin).Scan(&n)
	return n, err
}

// checkAdminRemains refuses a change that took away the last admin.
func checkAdminRemains(tx *sql.Tx, before int) error {
	after, err := countAdmins(tx)
	if err != nil {
		return err
	}
	if before > 0 && after == 0 {
		return ErrLastAdmin
	}
	return nil
}

// TouchUser records activity, at most once a minute.
func (db *DB) TouchUser(id int64) error {
	_, err := db.Exec(`
		UPDATE users SET last_seen_at = CURRENT_TIMESTAMP
		WHERE id = ? AND (last_seen_at IS NULL OR last_seen_at < datetime('now', '-1 minute'))`, id)
	return err
}

type Session struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"-"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
	UserAgent string    `json:"user_agent"`
}

// CreateSession signs a user in and returns the session token for the cookie.
func (db *DB) CreateSession(userID int64, userAgent string) (string, error) {
	token, err := randomToken("")
	if err != nil {
		return "", err
	}
	if len(userAgent) > 200 {
		userAgent = userAgent[:200]
	}
	_, err = db.Exec(`
		INSERT INTO sessions (user_id, token_hash, expires_at, user_agent) VALUES (?, ?, ?, ?)`,
		userID, hashToken(token), time.Now().Add(SessionLifetime), userAgent)
	if err != nil {
		return "", err
	}
	return token, db.TouchUser(userID)
}

// AuthenticateSession returns the session and user for a cookie token, or
// nils if it is unknown or expired.
func (db *DB) AuthenticateSession(token string) (*Session, *User, error) {
	var s Session
	err := db.QueryRow(`
		SELECT id, user_id, created_at, expires_at, COALESCE(user_agent, '') FROM sessions
		WHERE token_hash = ? AND expires_at > ?`, hashToken(token), time.Now()).
		Scan(&s.ID, &s.UserID, &s.CreatedAt, &s.ExpiresAt, &s.UserAgent)
	if err == sql.ErrNoRows {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	u, err := db.GetUser(s.UserID)
	if err != nil || u == nil {
		return nil, nil, err
	}
	return &s, u, nil
}

func (db *DB) DeleteSession(token string) error {
	_, err := db.Exec(`DELETE FROM sessions WHERE token_hash = ?`, hashToken(token))
	return err
}

func (db *DB) DeleteExpiredSessions() error {
	_, err := db.Exec(`DELETE FROM sessions WHERE expires_at <= ?`, time.Now())
	return err
}

const usersTables = `CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT UNIQUE NOT NULL,
	display_name TEXT NOT NULL DEFAULT '',
	role TEXT NOT NULL,
	password_hash TEXT,
	invite_hash TEXT UNIQUE,
	invite_expires_at DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	last_seen_at DATETIME
);
CREATE TABLE IF NOT EXISTS sessions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
	token_hash TEXT UNIQUE NOT NULL,
	user_agent TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	expires_at DATETIME NOT NULL
)`
