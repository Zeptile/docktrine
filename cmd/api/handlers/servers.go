package handlers

import (
	"github.com/Zeptile/docktrine/internal/database"
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