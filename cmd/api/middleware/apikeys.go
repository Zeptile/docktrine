package middleware

import (
	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/logger"
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

        key, err := db.AuthenticateAPIKey(apiKey)
        if err != nil {
            logger.Error(err, "Failed to check API key")
            return c.Status(500).JSON(fiber.Map{
                "error": "Failed to check API key",
            })
        }
        if key == nil {
            return c.Status(401).JSON(fiber.Map{
                "error": "Invalid API key",
            })
        }

        if err := db.TouchAPIKey(key.ID); err != nil {
            logger.Warn("Failed to record API key use: " + err.Error())
        }
        c.Locals("api_key_id", key.ID)
        c.Locals("api_key_name", key.Name)

        return c.Next()
    }
}
