package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestOriginAllowed(t *testing.T) {
	cases := []struct {
		origin, host, forwarded string
		ok                      bool
		comment                 string
	}{
		{"https://docktrine.example.com", "docktrine.example.com", "", true, "same site"},
		{"http://localhost:5182", "localhost:5182", "", true, "dev server"},
		{"https://Docktrine.Example.com", "docktrine.example.com", "", true, "hosts ignore case"},
		{"https://evil.example", "docktrine.example.com", "", false, "another site"},
		{"https://docktrine.example.com.evil.example", "docktrine.example.com", "", false, "a lookalike"},
		{"http://localhost:5182", "localhost:4002", "", false, "another port"},
		{"https://docktrine.example.com", "10.0.0.5:3000", "docktrine.example.com", true, "behind a proxy"},
		{"https://docktrine.example.com", "10.0.0.5:3000", "docktrine.example.com, proxy.internal", true, "proxy chain"},
		{"https://evil.example", "10.0.0.5:3000", "docktrine.example.com", false, "another site behind a proxy"},
		{"null", "docktrine.example.com", "", false, "sandboxed frame"},
		{"file://", "docktrine.example.com", "", false, "file page"},
		{"chrome-extension://abc", "abc", "", false, "not a web origin"},
		{"", "docktrine.example.com", "", true, "not a browser"},
	}
	for _, c := range cases {
		if got := OriginAllowed(c.origin, c.host, c.forwarded); got != c.ok {
			t.Errorf("%s: OriginAllowed(%q, %q, %q) = %v, want %v", c.comment, c.origin, c.host, c.forwarded, got, c.ok)
		}
	}
}

func TestSameOrigin(t *testing.T) {
	app := fiber.New()
	app.Get("/shell", SameOrigin(), func(c *fiber.Ctx) error { return c.SendString("ok") })

	for origin, want := range map[string]int{
		"http://docktrine.test": 200,
		"http://evil.test":      403,
		"":                      200,
	} {
		req := httptest.NewRequest("GET", "http://docktrine.test/shell", nil)
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != want {
			t.Errorf("origin %q: status %d, want %d", origin, resp.StatusCode, want)
		}
	}
}

func TestWebsocketKey(t *testing.T) {
	cases := map[string]string{
		"docktrine, docktrine-key.dkt_abc123": "dkt_abc123",
		"docktrine-key.dkt_abc123,docktrine":  "dkt_abc123",
		"docktrine":                           "",
		"docktrine, docktrine-key.":           "",
		"":                                    "",
		"other.docktrine-key.dkt_x":           "",
	}
	for in, want := range cases {
		if got := websocketKey(in); got != want {
			t.Errorf("websocketKey(%q) = %q, want %q", in, got, want)
		}
	}
}
