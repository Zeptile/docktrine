package middleware

import (
	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
)

const SessionCookie = "docktrine_session"

// Browsers send this header on every request; a cross-site form can't, so
// requiring it on cookie-authenticated writes blocks CSRF.
const ClientHeader = "X-Docktrine-Client"

// Principal is who made a request: a signed-in user, or an API key (owned
// by a user, or ownerless and acting as admin).
type Principal struct {
	User      *database.User
	SessionID int64
	Key       *database.APIKey
}

func (p *Principal) Role() string {
	if p.User != nil {
		return p.User.Role
	}
	return database.RoleAdmin
}

func (p *Principal) UserID() *int64 {
	if p.User != nil {
		return &p.User.ID
	}
	return nil
}

// Name is recorded as the author of deployments.
func (p *Principal) Name() string {
	switch {
	case p.Key != nil && p.User != nil:
		return p.User.Username + " (" + p.Key.Name + ")"
	case p.Key != nil:
		return p.Key.Name
	case p.User != nil:
		return p.User.Username
	}
	return "api"
}

func GetPrincipal(c *fiber.Ctx) *Principal {
	p, _ := c.Locals("principal").(*Principal)
	return p
}

// Auth accepts an X-API-Key header or a session cookie.
func Auth(db *database.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if apiKey := c.Get("X-API-Key"); apiKey != "" {
			return keyAuth(db, c, apiKey)
		}
		if apiKey := websocketKey(c.Get(fiber.HeaderSecWebSocketProtocol)); apiKey != "" {
			return keyAuth(db, c, apiKey)
		}
		if token := c.Cookies(SessionCookie); token != "" {
			return sessionAuth(db, c, token)
		}
		return c.Status(401).JSON(fiber.Map{"error": "Sign in or send an API key"})
	}
}

func keyAuth(db *database.DB, c *fiber.Ctx, apiKey string) error {
	key, err := db.AuthenticateAPIKey(apiKey)
	if err != nil {
		logger.Error(err, "Failed to check API key")
		return c.Status(500).JSON(fiber.Map{"error": "Failed to check API key"})
	}
	if key == nil {
		return c.Status(401).JSON(fiber.Map{"error": "Invalid API key"})
	}
	if err := db.TouchAPIKey(key.ID); err != nil {
		logger.Warn("Failed to record API key use: " + err.Error())
	}

	p := &Principal{Key: key}
	if key.UserID != nil {
		if p.User, err = db.GetUser(*key.UserID); err != nil || p.User == nil {
			return c.Status(401).JSON(fiber.Map{"error": "Invalid API key"})
		}
		db.TouchUser(p.User.ID)
	}
	c.Locals("principal", p)
	return c.Next()
}

func sessionAuth(db *database.DB, c *fiber.Ctx, token string) error {
	session, user, err := db.AuthenticateSession(token)
	if err != nil {
		logger.Error(err, "Failed to check session")
		return c.Status(500).JSON(fiber.Map{"error": "Failed to check session"})
	}
	if session == nil {
		c.ClearCookie(SessionCookie)
		return c.Status(401).JSON(fiber.Map{"error": "Your session expired; sign in again"})
	}
	if c.Method() != fiber.MethodGet && c.Method() != fiber.MethodHead && c.Get(ClientHeader) == "" {
		return c.Status(403).JSON(fiber.Map{"error": "Missing " + ClientHeader + " header"})
	}
	if err := db.TouchUser(user.ID); err != nil {
		logger.Warn("Failed to record user activity: " + err.Error())
	}
	c.Locals("principal", &Principal{User: user, SessionID: session.ID})
	return c.Next()
}

// Require rejects requests whose principal has less than the given role.
func Require(role string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if p := GetPrincipal(c); p == nil || !database.RoleAtLeast(p.Role(), role) {
			return c.Status(403).JSON(fiber.Map{"error": "This needs the " + role + " role"})
		}
		return c.Next()
	}
}
