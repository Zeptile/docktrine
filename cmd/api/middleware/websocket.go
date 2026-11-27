package middleware

import (
	"net/url"
	"strings"

	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
)

// WebSocketProtocol is the subprotocol the web UI asks for. A browser can't
// set headers on a WebSocket, so the UI signed in with an API key offers it
// as a second subprotocol, KeyProtocolPrefix + key, which the server reads
// but never echoes back.
const (
	WebSocketProtocol = "docktrine"
	KeyProtocolPrefix = "docktrine-key."
)

// websocketKey returns the API key offered in a WebSocket handshake.
func websocketKey(protocols string) string {
	for _, p := range strings.Split(protocols, ",") {
		if key, ok := strings.CutPrefix(strings.TrimSpace(p), KeyProtocolPrefix); ok && key != "" {
			return key
		}
	}
	return ""
}

// SameOrigin refuses WebSocket handshakes started by another site. Browsers
// send cookies with a cross-site handshake and CORS doesn't apply to it, so
// without this check any page a signed-in user visits could open a shell
// with their session (cross-site WebSocket hijacking).
func SameOrigin() fiber.Handler {
	return func(c *fiber.Ctx) error {
		origin := c.Get(fiber.HeaderOrigin)
		if !OriginAllowed(origin, string(c.Request().Host()), c.Get(fiber.HeaderXForwardedHost)) {
			logger.Warn("Refused a WebSocket from origin " + origin + " for " + c.Path())
			return c.Status(403).JSON(fiber.Map{"error": "Cross-origin WebSocket refused"})
		}
		return c.Next()
	}
}

// OriginAllowed reports whether a handshake's Origin is the host it was
// sent to. A reverse proxy may rewrite Host and pass the original along in
// X-Forwarded-Host; a page can't set that header on a WebSocket, so it's
// safe to accept either. Clients that aren't browsers send no Origin and
// are let through: they can't be tricked into sending someone's cookie.
func OriginAllowed(origin, host, forwardedHost string) bool {
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		// Includes "null", sent by sandboxed frames and file:// pages
		return false
	}
	if host != "" && strings.EqualFold(u.Host, host) {
		return true
	}
	if forwardedHost != "" {
		first, _, _ := strings.Cut(forwardedHost, ",")
		if strings.EqualFold(u.Host, strings.TrimSpace(first)) {
			return true
		}
	}
	return false
}
