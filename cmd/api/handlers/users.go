package handlers

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/Zeptile/docktrine/cmd/api/middleware"
	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
)

// Sign-in attempts allowed per IP address in loginWindow before further
// attempts are refused until the window passes.
const (
	loginAttempts = 10
	loginWindow   = 15 * time.Minute
)

type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
}

var logins = &loginLimiter{failures: map[string][]time.Time{}}

func (l *loginLimiter) recent(ip string) []time.Time {
	cutoff := time.Now().Add(-loginWindow)
	kept := l.failures[ip][:0]
	for _, t := range l.failures[ip] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, ip)
	} else {
		l.failures[ip] = kept
	}
	return kept
}

func (l *loginLimiter) blocked(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.recent(ip)) >= loginAttempts
}

func (l *loginLimiter) fail(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[ip] = append(l.recent(ip), time.Now())
}

func setSessionCookie(c *fiber.Ctx, token string) {
	c.Cookie(&fiber.Cookie{
		Name:     middleware.SessionCookie,
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(database.SessionLifetime),
		HTTPOnly: true,
		Secure:   c.Protocol() == "https",
		SameSite: fiber.CookieSameSiteStrictMode,
	})
}

func (h *Handler) signIn(c *fiber.Ctx, u *database.User) error {
	token, err := h.db.CreateSession(u.ID, c.Get(fiber.HeaderUserAgent))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	setSessionCookie(c, token)
	return c.JSON(u)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// AuthStatus godoc
// @Summary Sign-in options
// @Description Whether anyone can sign in with a password yet. No authentication needed.
// @Tags auth
// @Produce json
// @Success 200 {object} interface{}
// @Router /auth/status [get]
func (h *Handler) AuthStatus(c *fiber.Ctx) error {
	n, err := h.db.CountUsers()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{"has_users": n > 0})
}

// Login godoc
// @Summary Sign in
// @Description Starts a browser session (cookie). Scripts should use API keys instead.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body loginRequest true "Credentials"
// @Success 200 {object} database.User
// @Failure 401 {object} interface{}
// @Failure 429 {object} interface{}
// @Router /auth/login [post]
func (h *Handler) Login(c *fiber.Ctx) error {
	if logins.blocked(c.IP()) {
		return c.Status(429).JSON(fiber.Map{"error": "Too many failed sign-ins; try again in 15 minutes"})
	}
	var body loginRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	u, err := h.db.CheckPassword(body.Username, body.Password)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if u == nil {
		logins.fail(c.IP())
		return c.Status(401).JSON(fiber.Map{"error": "Wrong username or password"})
	}
	logger.Info(fmt.Sprintf("%s signed in from %s", u.Username, c.IP()))
	return h.signIn(c, u)
}

// Logout godoc
// @Summary Sign out
// @Description Ends the browser session
// @Tags auth
// @Success 204
// @Router /auth/logout [post]
func (h *Handler) Logout(c *fiber.Ctx) error {
	if token := c.Cookies(middleware.SessionCookie); token != "" {
		if err := h.db.DeleteSession(token); err != nil {
			return c.Status(500).JSON(fiber.Map{"error": err.Error()})
		}
	}
	c.ClearCookie(middleware.SessionCookie)
	return c.SendStatus(204)
}

// GetInvite godoc
// @Summary Look up an invite
// @Description Who an invite link is for. No authentication needed.
// @Tags auth
// @Produce json
// @Param token path string true "Invite token"
// @Success 200 {object} database.User
// @Failure 404 {object} interface{}
// @Router /auth/invite/{token} [get]
func (h *Handler) GetInvite(c *fiber.Ctx) error {
	u, err := h.db.GetUserByInvite(c.Params("token"))
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if u == nil {
		return c.Status(404).JSON(fiber.Map{"error": "This invite link has expired or was already used. Ask an admin for a new one."})
	}
	return c.JSON(u)
}

// AcceptInvite godoc
// @Summary Accept an invite
// @Description Sets the password and signs in
// @Tags auth
// @Accept json
// @Produce json
// @Param token path string true "Invite token"
// @Success 200 {object} database.User
// @Router /auth/invite/{token} [post]
func (h *Handler) AcceptInvite(c *fiber.Ctx) error {
	var body struct {
		Password string `json:"password"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	u, err := h.db.AcceptInvite(c.Params("token"), body.Password)
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if u == nil {
		return c.Status(404).JSON(fiber.Map{"error": "This invite link has expired or was already used. Ask an admin for a new one."})
	}
	logger.Info(fmt.Sprintf("%s accepted their invite", u.Username))
	return h.signIn(c, u)
}

// GetMe godoc
// @Summary Who am I
// @Description The user (if any) and role behind this request
// @Tags auth
// @Produce json
// @Success 200 {object} interface{}
// @Router /me [get]
func (h *Handler) GetMe(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	result := fiber.Map{"user": p.User, "role": p.Role(), "via": "session"}
	if p.Key != nil {
		result["via"] = "key"
		result["key_name"] = p.Key.Name
	}
	return c.JSON(result)
}

// UpdateMe godoc
// @Summary Change your display name
// @Tags auth
// @Accept json
// @Produce json
// @Success 200 {object} database.User
// @Router /me [patch]
func (h *Handler) UpdateMe(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	if p.User == nil {
		return c.Status(400).JSON(fiber.Map{"error": "This API key doesn't belong to a user"})
	}
	var body struct {
		DisplayName string `json:"display_name"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if _, err := h.db.UpdateUser(p.User.ID, body.DisplayName, p.User.Role); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	u, err := h.db.GetUser(p.User.ID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(u)
}

// ChangePassword godoc
// @Summary Change your password
// @Description Signs out your other sessions
// @Tags auth
// @Accept json
// @Success 204
// @Router /me/password [put]
func (h *Handler) ChangePassword(c *fiber.Ctx) error {
	p := middleware.GetPrincipal(c)
	if p.User == nil {
		return c.Status(400).JSON(fiber.Map{"error": "This API key doesn't belong to a user"})
	}
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	if err := h.db.ChangePassword(p.User.ID, body.Current, body.New, p.SessionID); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	return c.SendStatus(204)
}

type userRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
}

type invitedUser struct {
	*database.User
	// InviteToken goes in the link the new user opens: /#/invite/<token>
	InviteToken string `json:"invite_token"`
}

func userIDParam(c *fiber.Ctx) (int64, error) {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, c.Status(400).JSON(fiber.Map{"error": "invalid user id"})
	}
	return id, nil
}

// ListUsers godoc
// @Summary List users
// @Tags users
// @Produce json
// @Success 200 {array} database.User
// @Router /users [get]
func (h *Handler) ListUsers(c *fiber.Ctx) error {
	users, err := h.db.ListUsers()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(users)
}

// CreateUser godoc
// @Summary Invite a user
// @Description Returns an invite token, valid for 7 days, for the user to choose a password
// @Tags users
// @Accept json
// @Produce json
// @Param body body userRequest true "New user"
// @Success 201 {object} invitedUser
// @Router /users [post]
func (h *Handler) CreateUser(c *fiber.Ctx) error {
	var body userRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	u, token, err := h.db.CreateUser(body.Username, body.DisplayName, body.Role)
	if errors.Is(err, database.ErrUsernameTaken) {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	logger.Info(fmt.Sprintf("User %s (%s) invited by %s", u.Username, u.Role, createdBy(c)))
	return c.Status(201).JSON(invitedUser{User: u, InviteToken: token})
}

// UpdateUser godoc
// @Summary Change a user's name or role
// @Tags users
// @Accept json
// @Produce json
// @Param id path int true "User id"
// @Success 200 {object} database.User
// @Router /users/{id} [patch]
func (h *Handler) UpdateUser(c *fiber.Ctx) error {
	id, err := userIDParam(c)
	if id == 0 {
		return err
	}
	var body userRequest
	if err := c.BodyParser(&body); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	found, err := h.db.UpdateUser(id, body.DisplayName, body.Role)
	if errors.Is(err, database.ErrLastAdmin) {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(400).JSON(fiber.Map{"error": err.Error()})
	}
	if !found {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	u, err := h.db.GetUser(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(u)
}

// ResetUser godoc
// @Summary Reset a user's password
// @Description Signs them out and returns a new invite token for choosing a password
// @Tags users
// @Produce json
// @Param id path int true "User id"
// @Success 200 {object} invitedUser
// @Router /users/{id}/reset [post]
func (h *Handler) ResetUser(c *fiber.Ctx) error {
	id, err := userIDParam(c)
	if id == 0 {
		return err
	}
	token, err := h.db.ResetUserPassword(id)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	u, err := h.db.GetUser(id)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	logger.Info(fmt.Sprintf("Password of %s reset by %s", u.Username, createdBy(c)))
	return c.JSON(invitedUser{User: u, InviteToken: token})
}

// DeleteUser godoc
// @Summary Remove a user
// @Description Their sessions and API keys stop working
// @Tags users
// @Param id path int true "User id"
// @Success 204
// @Router /users/{id} [delete]
func (h *Handler) DeleteUser(c *fiber.Ctx) error {
	id, err := userIDParam(c)
	if id == 0 {
		return err
	}
	found, err := h.db.DeleteUser(id)
	if errors.Is(err, database.ErrLastAdmin) {
		return c.Status(409).JSON(fiber.Map{"error": err.Error()})
	}
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if !found {
		return c.Status(404).JSON(fiber.Map{"error": "user not found"})
	}
	logger.Info(fmt.Sprintf("User %d removed by %s", id, createdBy(c)))
	return c.SendStatus(204)
}
