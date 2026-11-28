package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Zeptile/docktrine/cmd/api/handlers"
	"github.com/Zeptile/docktrine/cmd/api/middleware"
	_ "github.com/Zeptile/docktrine/docs"
	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/deploy"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/Zeptile/docktrine/internal/secrets"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/swagger"
	_ "github.com/mattn/go-sqlite3"
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

	// Recovery for anyone with a shell on the server, e.g. when every key
	// is lost: docker compose exec docktrine /app/docktrine-api create-key <name>
	// Runs before anything that assumes it is the only server process.
	if len(os.Args) > 1 && os.Args[1] == "invite" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: docktrine-api invite <username>")
			os.Exit(2)
		}
		inviteAdmin(db, os.Args[2])
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "create-key" {
		if len(os.Args) != 3 {
			fmt.Fprintln(os.Stderr, "usage: docktrine-api create-key <name>")
			os.Exit(2)
		}
		key, err := db.CreateAPIKey(os.Args[2], nil)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Created API key %q. Store it now, it won't be shown again:\n", key.Name)
		fmt.Println(key.Key)
		return
	}

	if err := db.DeleteExpiredSessions(); err != nil {
		logger.Warn("Failed to delete expired sessions: " + err.Error())
	}
	if err := db.FailStaleDeployments(); err != nil {
		logger.Fatal(err, "Failed to clean up interrupted deployments")
	}

	box, err := secrets.LoadBox(database.DataPath())
	if err != nil {
		logger.Fatal(err, "Failed to load secret key")
	}

	deployer := deploy.New(db, box, deploy.ConfigFromEnv())
	deployer.StartJanitor(context.Background(), 10*time.Minute)
	deployer.StartUpdateChecks(context.Background(), deploy.UpdateIntervalFromEnv())

	app := fiber.New()
	
	// Unauthenticated liveness check for container health checks, ahead of
	// the request logger so it doesn't log every probe
	app.Get("/healthz", func(c *fiber.Ctx) error {
		if err := db.Ping(); err != nil {
			return c.Status(503).SendString("database unavailable")
		}
		return c.SendString("ok")
	})

	app.Use(middleware.RequestLogger())

	// Serve static files from the web/dist directory for the UI
	app.Static("/", "./web/dist")
	
	handler := handlers.NewHandler(db, deployer, box)
	handler.StartShellJanitor(context.Background())

	// Signing in, and accepting an invite, happen before there's a session
	auth := app.Group("/api/auth")
	auth.Get("/status", handler.AuthStatus)
	auth.Post("/login", handler.Login)
	auth.Post("/logout", handler.Logout)
	auth.Get("/invite/:token", handler.GetInvite)
	auth.Post("/invite/:token", handler.AcceptInvite)

	api := app.Group("/api", func(c *fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), "/api/swagger") {
			return c.Next()
		}
		return middleware.Auth(db)(c)
	})

	// Reading needs any role; changing apps and containers needs deployer;
	// servers and users need admin
	canDeploy := middleware.Require(database.RoleDeployer)
	isAdmin := middleware.Require(database.RoleAdmin)

	logger.Info("Setting up routes...")
	api.Get("/swagger/*", swagger.HandlerDefault)

	api.Get("/me", handler.GetMe)
	api.Patch("/me", handler.UpdateMe)
	api.Put("/me/password", handler.ChangePassword)

	users := api.Group("/users", isAdmin)
	users.Get("/", handler.ListUsers)
	users.Post("/", handler.CreateUser)
	users.Patch("/:id", handler.UpdateUser)
	users.Delete("/:id", handler.DeleteUser)
	users.Post("/:id/reset", handler.ResetUser)

	containers := api.Group("/containers")
	containers.Get("/", handler.ListContainers)
	containers.Post("/start/:id", canDeploy, handler.StartContainer)
	containers.Post("/stop/:id", canDeploy, handler.StopContainer)
	containers.Get("/:id", handler.GetContainer)
	containers.Post("/restart/:id", canDeploy, handler.RestartContainer)
	containers.Post("/create", canDeploy, handler.CreateContainer)
	// A WebSocket; a shell in a container needs deployer, one on the node
	// is root on the host and needs admin
	containers.Get("/:id/shell", canDeploy, handler.ShellUpgrade, handler.ContainerShell())

	servers := api.Group("/servers")
	servers.Get("/", handler.ListServers)
	servers.Get("/:name", handler.GetServer)
	servers.Post("/", isAdmin, handler.CreateServer)
	servers.Put("/:name", isAdmin, handler.UpdateServer)
	servers.Delete("/:name", isAdmin, handler.DeleteServer)
	servers.Get("/:name/health", handler.ServerHealth)
	servers.Post("/:name/ingress", isAdmin, handler.InstallIngress)
	servers.Post("/:name/prune", isAdmin, handler.PruneServer)
	servers.Get("/:name/shell", isAdmin, handler.ShellUpgrade, handler.NodeShell())

	api.Get("/config", handler.GetConfig)

	// Everyone manages their own keys; admins manage all of them
	keys := api.Group("/keys")
	keys.Get("/", handler.ListAPIKeys)
	keys.Post("/", handler.CreateAPIKey)
	keys.Patch("/:id", handler.RenameAPIKey)
	keys.Delete("/:id", handler.RevokeAPIKey)

	deployments := api.Group("/deployments")
	deployments.Get("/", handler.ListDeployments)
	deployments.Post("/", canDeploy, handler.CreateDeployment)
	deployments.Get("/:id", handler.GetDeployment)

	// Reading a stack or image for the launch form fetches from URLs the
	// user gives, so it takes the same role as deploying
	api.Post("/stacks/inspect", canDeploy, handler.InspectStack)
	api.Get("/images/inspect", canDeploy, handler.InspectImage)
	api.Post("/launch/preflight", canDeploy, handler.Preflight)
	api.Get("/catalog", canDeploy, handler.ListCatalog)

	apps := api.Group("/apps")
	apps.Get("/", handler.ListApps)
	apps.Get("/:app/:env", handler.GetAppEnvironment)
	apps.Delete("/:app/:env", canDeploy, handler.DestroyEnvironment)
	apps.Get("/:app/:env/logs", handler.GetAppLogs)
	apps.Post("/:app/:env/rollback", canDeploy, handler.RollbackEnvironment)
	// Checking reaches registries and forges, so it takes the same role as
	// the update it leads to
	apps.Post("/:app/:env/update", canDeploy, handler.UpdateEnvironment)
	apps.Post("/:app/:env/update/check", canDeploy, handler.CheckUpdates)
	apps.Get("/:app/:env/secrets", handler.ListSecrets)
	apps.Put("/:app/:env/secrets", canDeploy, handler.SetSecrets)
	apps.Delete("/:app/:env/secrets/:key", canDeploy, handler.DeleteSecret)
	
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

// inviteAdmin prints an invite link for an admin, creating the user if
// needed or resetting their password if they exist.
func inviteAdmin(db *database.DB, username string) {
	var token string
	u, err := db.GetUserByUsername(username)
	if err == nil && u == nil {
		u, token, err = db.CreateUser(username, "", database.RoleAdmin)
	} else if err == nil {
		// Recovery: whoever has a shell on the server can make anyone admin
		if _, err = db.UpdateUser(u.ID, u.DisplayName, database.RoleAdmin); err == nil {
			token, err = db.ResetUserPassword(u.ID)
		}
		u.Role = database.RoleAdmin
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "Open this in the next %d days to choose a password for %s (%s):\n",
		int(database.InviteLifetime.Hours()/24), u.Username, u.Role)
	fmt.Println("https://<docktrine host>/#/invite/" + token)
}
