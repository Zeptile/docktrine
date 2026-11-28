package handlers

import (
	"net/http/httptest"
	"testing"

	"github.com/Zeptile/docktrine/cmd/api/middleware"
	"github.com/Zeptile/docktrine/internal/database"
	"github.com/gofiber/fiber/v2"
)

func principal(role string) *middleware.Principal {
	return &middleware.Principal{User: &database.User{Username: role, Role: role}}
}

func TestCanOpenShell(t *testing.T) {
	cases := []struct {
		p    *middleware.Principal
		kind string
		ok   bool
	}{
		{principal(database.RoleViewer), shellContainer, false},
		{principal(database.RoleDeployer), shellContainer, true},
		{principal(database.RoleAdmin), shellContainer, true},
		{principal(database.RoleViewer), shellNode, false},
		{principal(database.RoleDeployer), shellNode, false},
		{principal(database.RoleAdmin), shellNode, true},
		// An ownerless API key acts as admin
		{&middleware.Principal{Key: &database.APIKey{Name: "ci"}}, shellNode, true},
		{nil, shellContainer, false},
		{principal(database.RoleAdmin), "something else", false},
	}
	for _, c := range cases {
		name := "nobody"
		if c.p != nil {
			name = c.p.Name() + "/" + c.p.Role()
		}
		if got := canOpenShell(c.p, c.kind); got != c.ok {
			t.Errorf("%s opening a %s shell: %v, want %v", name, c.kind, got, c.ok)
		}
	}
}

// The gate in front of the shell sockets, wired as in main.go: the role
// first, then the handshake and origin checks.
func TestShellGate(t *testing.T) {
	h := &Handler{}
	app := fiber.New()
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("principal", principal(c.Get("X-Test-Role")))
		return c.Next()
	})
	ok := func(c *fiber.Ctx) error { return c.SendString("upgraded") }
	app.Get("/containers/:id/shell", middleware.Require(database.RoleDeployer), h.ShellUpgrade, ok)
	app.Get("/servers/:name/shell", middleware.Require(database.RoleAdmin), h.ShellUpgrade, ok)

	cases := []struct {
		path, role, origin string
		upgrade            bool
		want               int
	}{
		{"/containers/abc/shell", database.RoleViewer, "http://docktrine.test", true, 403},
		{"/containers/abc/shell", database.RoleDeployer, "http://docktrine.test", true, 200},
		{"/servers/local/shell", database.RoleDeployer, "http://docktrine.test", true, 403},
		{"/servers/local/shell", database.RoleAdmin, "http://docktrine.test", true, 200},
		{"/servers/local/shell", database.RoleAdmin, "http://evil.test", true, 403},
		{"/containers/abc/shell", database.RoleDeployer, "http://evil.test", true, 403},
		{"/containers/abc/shell", database.RoleDeployer, "http://docktrine.test", false, 426},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "http://docktrine.test"+c.path, nil)
		req.Header.Set("X-Test-Role", c.role)
		req.Header.Set("Origin", c.origin)
		if c.upgrade {
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-WebSocket-Version", "13")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != c.want {
			t.Errorf("%s as %s from %s (upgrade %v): status %d, want %d",
				c.path, c.role, c.origin, c.upgrade, resp.StatusCode, c.want)
		}
	}
}
