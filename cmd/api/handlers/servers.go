package handlers

import (
	"context"
	"fmt"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/deploy"
	"github.com/Zeptile/docktrine/internal/docker"
	"github.com/docker/docker/api/types/filters"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
)

// ListServers godoc
// @Summary List all servers
// @Description Get a list of all Docker servers
// @Tags servers
// @Accept json
// @Produce json
// @Success 200 {array} database.Server
// @Failure 500 {object} interface{}
// @Router /servers [get]
func (h *Handler) ListServers(c *fiber.Ctx) error {
	servers, err := h.db.GetServers()
	if err != nil {
		logger.Error(err, "Failed to list servers")
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(servers)
}

// GetServer godoc
// @Summary Get server details
// @Description Get details of a specific server by name
// @Tags servers
// @Accept json
// @Produce json
// @Param name path string true "Server name"
// @Success 200 {object} database.Server
// @Failure 404 {object} interface{}
// @Failure 500 {object} interface{}
// @Router /servers/{name} [get]
func (h *Handler) GetServer(c *fiber.Ctx) error {
	name := c.Params("name")
	server, err := h.db.GetServerByName(name)
	if err != nil {
		logger.Error(err, "Failed to get server")
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if server == nil {
		return c.Status(404).JSON(fiber.Map{"error": "server not found"})
	}
	return c.JSON(server)
}

// CreateServer godoc
// @Summary Create a new server
// @Description Create a new Docker server configuration
// @Tags servers
// @Accept json
// @Produce json
// @Param server body database.Server true "Server configuration"
// @Success 201 {object} database.Server
// @Failure 400 {object} interface{}
// @Failure 500 {object} interface{}
// @Router /servers [post]
func (h *Handler) CreateServer(c *fiber.Ctx) error {
	var server database.Server
	if err := c.BodyParser(&server); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if server.Name == "" || server.Host == "" {
		return c.Status(400).JSON(fiber.Map{"error": "name and host are required"})
	}

	// Check if server already exists
	existing, err := h.db.GetServerByName(server.Name)
	if err != nil {
		logger.Error(err, "Failed to check server existence")
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if existing != nil {
		return c.Status(400).JSON(fiber.Map{"error": "server with this name already exists"})
	}

	if err := h.db.CreateServer(&server); err != nil {
		logger.Error(err, "Failed to create server")
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.Status(201).JSON(server)
}

// DeleteServer godoc
// @Summary Delete a server
// @Description Delete a server configuration by name
// @Tags servers
// @Accept json
// @Produce json
// @Param name path string true "Server name"
// @Success 200 {object} interface{}
// @Failure 404 {object} interface{}
// @Failure 500 {object} interface{}
// @Router /servers/{name} [delete]
func (h *Handler) DeleteServer(c *fiber.Ctx) error {
	name := c.Params("name")
	
	exists, err := h.db.GetServerByName(name)
	if err != nil {
		logger.Error(err, "Failed to check server existence")
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if exists == nil {
		return c.Status(404).JSON(fiber.Map{"error": "server not found"})
	}

	if err := h.db.DeleteServer(name); err != nil {
		logger.Error(err, "Failed to delete server")
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}

	return c.JSON(fiber.Map{"message": "server deleted successfully"})
} 
// UpdateServer godoc
// @Summary Update a server
// @Description Update host, description, default flag and ingress settings
// @Tags servers
// @Accept json
// @Produce json
// @Param name path string true "Server name"
// @Param server body database.Server true "Server configuration"
// @Success 200 {object} database.Server
// @Router /servers/{name} [put]
func (h *Handler) UpdateServer(c *fiber.Ctx) error {
	name := c.Params("name")
	server, err := h.db.GetServerByName(name)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	if server == nil {
		return c.Status(404).JSON(fiber.Map{"error": "server not found"})
	}

	if err := c.BodyParser(server); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}
	server.Name = name
	if server.Host == "" {
		return c.Status(400).JSON(fiber.Map{"error": "host is required"})
	}

	if err := h.db.UpdateServer(server); err != nil {
		logger.Error(err, "Failed to update server")
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	updated, err := h.db.GetServerByName(name)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(updated)
}

// InstallIngress godoc
// @Summary Install the node ingress
// @Description (Re)create the Traefik container that routes gateway traffic to apps by hostname
// @Tags servers
// @Accept json
// @Produce json
// @Param name path string true "Server name"
// @Success 200 {object} interface{}
// @Router /servers/{name}/ingress [post]
func (h *Handler) InstallIngress(c *fiber.Ctx) error {
	server, err := h.db.ResolveServer(c.Params("name"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	var opts deploy.IngressOptions
	if err := c.BodyParser(&opts); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "invalid request body"})
	}

	if err := h.deployer.InstallIngress(server, opts); err != nil {
		logger.Error(err, "Failed to install ingress")
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("ingress running on %s, port %d", server.Name, opts.Port),
	})
}

// ServerHealth godoc
// @Summary Node health
// @Description Docker daemon info and ingress status for a server
// @Tags servers
// @Produce json
// @Param name path string true "Server name"
// @Success 200 {object} interface{}
// @Router /servers/{name}/health [get]
func (h *Handler) ServerHealth(c *fiber.Ctx) error {
	server, err := h.db.ResolveServer(c.Params("name"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cli, err := docker.Connect(server)
	if err != nil {
		return c.JSON(fiber.Map{"reachable": false, "error": err.Error()})
	}
	defer cli.Close()

	info, err := cli.Info(ctx)
	if err != nil {
		return c.JSON(fiber.Map{"reachable": false, "error": err.Error()})
	}

	ingress := "not installed"
	if inspect, err := cli.ContainerInspect(ctx, deploy.IngressContainer); err == nil {
		ingress = inspect.State.Status
	}

	return c.JSON(fiber.Map{
		"reachable":          true,
		"server_version":     info.ServerVersion,
		"os":                 info.OperatingSystem,
		"cpus":               info.NCPU,
		"memory_bytes":       info.MemTotal,
		"containers":         info.Containers,
		"containers_running": info.ContainersRunning,
		"images":             info.Images,
		"ingress":            ingress,
	})
}

// PruneServer godoc
// @Summary Prune unused images
// @Description Remove dangling images, and with all=true every image not used by a container
// @Tags servers
// @Produce json
// @Param name path string true "Server name"
// @Param all query bool false "Remove all unused images, not just dangling ones"
// @Success 200 {object} interface{}
// @Router /servers/{name}/prune [post]
func (h *Handler) PruneServer(c *fiber.Ctx) error {
	server, err := h.db.ResolveServer(c.Params("name"))
	if err != nil {
		return c.Status(404).JSON(fiber.Map{"error": err.Error()})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cli, err := docker.Connect(server)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	defer cli.Close()

	args := filters.NewArgs()
	if c.QueryBool("all", false) {
		args.Add("dangling", "false")
	}
	report, err := cli.ImagesPrune(ctx, args)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": err.Error()})
	}
	return c.JSON(fiber.Map{
		"images_deleted":  len(report.ImagesDeleted),
		"space_reclaimed": report.SpaceReclaimed,
	})
}
