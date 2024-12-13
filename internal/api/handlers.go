package api

import (
	"fmt"

	"github.com/Zeptile/docktrine/internal/docker"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	docker *docker.DockerClient
}

func NewHandler() *Handler {
	return &Handler{}
}

// ListContainers godoc
// @Summary List all containers
// @Description Get a list of all Docker containers
// @Tags containers
// @Accept json
// @Produce json
// @Param server query string false "Server name"
// @Success 200 {array} interface{}
// @Failure 500 {object} interface{}
// @Router /containers [get]
func (h *Handler) ListContainers(c *fiber.Ctx) error {
	serverName := c.Query("server", "")
	logger.Debug("Listing containers")
	containers, err := h.docker.ListContainers(serverName)
	if err != nil {
		logger.Error(err, "Failed to list containers")
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	logger.Info("Successfully listed containers")
	return c.JSON(containers)
}

// StartContainer godoc
// @Summary Start a container
// @Description Start a Docker container by ID
// @Tags containers
// @Accept json
// @Produce json
// @Param id path string true "Container ID"
// @Param server query string false "Server name"
// @Success 200 {object} interface{}
// @Failure 400 {object} interface{}
// @Failure 500 {object} interface{}
// @Router /containers/start/{id} [post]
func (h *Handler) StartContainer(c *fiber.Ctx) error {
	containerID := c.Params("id")
	serverName := c.Query("server", "")
	logger.Debug(fmt.Sprintf("Starting container: %s", containerID))
	
	if containerID == "" {
		logger.Warn("Container ID is required")
		return c.Status(400).JSON(fiber.Map{
			"error": "container ID is required",
		})
	}

	err := h.docker.StartContainer(containerID, serverName)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to start container: %s", containerID))
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	logger.Info(fmt.Sprintf("Container started successfully: %s", containerID))
	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Container %s started successfully", containerID),
	})
}

// StopContainer godoc
// @Summary Stop a container
// @Description Stop a Docker container by ID
// @Tags containers
// @Accept json
// @Produce json
// @Param id path string true "Container ID"
// @Param server query string false "Server name"
// @Success 200 {object} interface{}
// @Failure 400 {object} interface{}
// @Failure 500 {object} interface{}
// @Router /containers/stop/{id} [post]
func (h *Handler) StopContainer(c *fiber.Ctx) error {
	containerID := c.Params("id")
	serverName := c.Query("server", "")
	
	if containerID == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "container ID is required",
		})
	}

	err := h.docker.StopContainer(containerID, serverName)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	logger.Info(fmt.Sprintf("Container stopped successfully: %s", containerID))
	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Container %s stopped successfully", containerID),
	})
}

// GetContainer godoc
// @Summary Get container details
// @Description Get detailed information about a specific Docker container
// @Tags containers
// @Accept json
// @Produce json
// @Param id path string true "Container ID"
// @Param server query string false "Server name"
// @Success 200 {object} interface{}
// @Failure 400 {object} interface{}
// @Failure 500 {object} interface{}
// @Router /containers/{id} [get]
func (h *Handler) GetContainer(c *fiber.Ctx) error {
	containerID := c.Params("id")
	serverName := c.Query("server", "")
	logger.Debug(fmt.Sprintf("Getting container: %s", containerID))
	
	if containerID == "" {
		logger.Warn("Container ID is required")
		return c.Status(400).JSON(fiber.Map{
			"error": "container ID is required",
		})
	}

	container, err := h.docker.GetContainer(containerID, serverName)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to get container: %s", containerID))
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	logger.Info(fmt.Sprintf("Container retrieved successfully: %s", containerID))
	return c.JSON(container)
}

// RestartContainer godoc
// @Summary Restart a container
// @Description Restart a Docker container by ID
// @Tags containers
// @Accept json
// @Produce json
// @Param id path string true "Container ID"
// @Param server query string false "Server name"
// @Param pull_latest query boolean false "Pull latest image before restart" default(false)
// @Success 200 {object} interface{}
// @Failure 400 {object} interface{}
// @Failure 500 {object} interface{}
// @Router /containers/restart/{id} [post]
func (h *Handler) RestartContainer(c *fiber.Ctx) error {
	containerID := c.Params("id")
	serverName := c.Query("server", "")
	logger.Debug(fmt.Sprintf("Restarting container: %s", containerID))
	
	if containerID == "" {
		logger.Warn("Container ID is required")
		return c.Status(400).JSON(fiber.Map{
			"error": "container ID is required",
		})
	}

	pullLatest := c.Query("pull_latest", "false") == "true"

	err := h.docker.RestartContainer(containerID, serverName, pullLatest)
	if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to restart container: %s", containerID))
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	logger.Info(fmt.Sprintf("Container restarted successfully: %s", containerID))
	return c.JSON(fiber.Map{
		"message": fmt.Sprintf("Container %s restarted successfully", containerID),
	})
}
