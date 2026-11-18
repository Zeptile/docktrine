package main

import (
	"os"
	"strings"

	"github.com/Zeptile/docktrine/cmd/api/handlers"
	"github.com/Zeptile/docktrine/cmd/api/middleware"
	_ "github.com/Zeptile/docktrine/docs"
	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

// @title Docktrine API
// @version 1.0
// @description Docker management API
// @host localhost:3000
// @BasePath /
func main() {
	logger.Init()

	db, err := database.NewDatabaseConnection()
	if err != nil {
		logger.Fatal(err, "Failed to initialize database")
	}
	defer db.Close()

	app := fiber.New()
	
	app.Use(middleware.RequestLogger())
	
	// Serve static files from the web/dist directory for the UI
	app.Static("/", "./web/dist")
	
	api := app.Group("/api", func(c *fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), "/api/swagger") {
			return c.Next()
		}
		return middleware.APIKeyAuth(db)(c)
	})
	
	handler := handlers.NewHandler(db)
	
	logger.Info("Setting up routes...")
	api.Get("/swagger/*", swagger.HandlerDefault)
	
	containers := api.Group("/containers")
	containers.Get("/", handler.ListContainers)
	containers.Post("/start/:id", handler.StartContainer)
	containers.Post("/stop/:id", handler.StopContainer)
	containers.Get("/:id", handler.GetContainer)
	containers.Post("/restart/:id", handler.RestartContainer)
	containers.Post("/create", handler.CreateContainer)
	
	servers := api.Group("/servers")
	servers.Get("/", handler.ListServers)
	servers.Get("/:name", handler.GetServer)
	servers.Post("/", handler.CreateServer)
	servers.Delete("/:name", handler.DeleteServer)
	
	api.All("/*", func(c *fiber.Ctx) error {
		return c.Status(404).JSON(fiber.Map{
			"error": "Not found",
		})
	})
	
	// Catch-all route for SPA (must be last)
	app.Get("/*", func(c *fiber.Ctx) error {
		return c.SendFile("./web/dist/index.html")
	})
	
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	
	logger.Info("Starting server on :" + port)
	logger.Info("UI available at: http://localhost:" + port)
	logger.Info("API documentation at: http://localhost:" + port + "/api/swagger/")
	if err := app.Listen(":" + port); err != nil {
		logger.Fatal(err, "Server failed to start")
	}
} 
