package middleware

import (
	"github.com/Zeptile/docktrine/internal/database"
	"github.com/gofiber/fiber/v2"
)

func APIKeyAuth(db *database.DB) fiber.Handler {
    return func(c *fiber.Ctx) error {
        apiKey := c.Get("X-API-Key")
        if apiKey == "" {
            return c.Status(401).JSON(fiber.Map{
                "error": "API key is required",
            })
        }

        key, err := db.GetAPIKey(apiKey)
        if err != nil || key == nil {
            return c.Status(401).JSON(fiber.Map{
                "error": "Invalid API key",
            })
        }

        db.UpdateAPIKeyLastUsed(apiKey)

        return c.Next()
    }
}