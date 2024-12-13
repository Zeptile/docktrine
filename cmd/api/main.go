package main

import (
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
	
	handler := handlers.NewHandler(db)
	
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
