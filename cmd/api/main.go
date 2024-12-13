package main

import (
	_ "github.com/Zeptile/docktrine/docs"
	"github.com/Zeptile/docktrine/internal/api"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/Zeptile/docktrine/internal/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
)

// @title Docktrine API
// @version 1.0
// @description Docker management API
// @host localhost:3000
// @BasePath /
func main() {
	// Initialize logger
	logger.Init()

	app := fiber.New()
	
	// Add request logger middleware
	app.Use(middleware.RequestLogger())
	
	
	handler := api.NewHandler()
	
	logger.Info("Setting up routes...")
	app.Get("/swagger/*", swagger.HandlerDefault)
	
	containers := app.Group("/containers")
	containers.Get("/", handler.ListContainers)
	containers.Post("/start/:id", handler.StartContainer)
	containers.Post("/stop/:id", handler.StopContainer)
	containers.Get("/:id", handler.GetContainer)
	containers.Post("/restart/:id", handler.RestartContainer)
	
	logger.Info("Starting server on :3000")
	if err := app.Listen(":3000"); err != nil {
		logger.Fatal(err, "Server failed to start")
	}
} 
