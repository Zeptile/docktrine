package middleware

import (
	"fmt"
	"time"

	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
)

func RequestLogger() fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now()
		path := c.Path()
		method := c.Method()

		ip := c.IP()

		// X-REAL-IP > X-FORWARDED-FOR > IP
		// - GBF 2024-12-03
		realIP := c.Get("X-Real-IP", c.Get("X-Forwarded-For", ip))

		logger.Info(fmt.Sprintf("--> %s %s [IP: %s]", method, path, realIP))

		body := string(c.Body())
		if len(body) > 0 {
			logger.Debug(fmt.Sprintf("Request Body: %s", body))
		}

		err := c.Next()

		duration := time.Since(start)

		status := c.Response().StatusCode()

		if err != nil {
			logger.Error(err, fmt.Sprintf("<-- %s %s %d %v [IP: %s]", method, path, status, duration, realIP))
		} else {
			logger.Info(fmt.Sprintf("<-- %s %s %d %v [IP: %s]", method, path, status, duration, realIP))
		}

		return err
	}
}
