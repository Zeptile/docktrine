package handlers

import (
	"fmt"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/Zeptile/docktrine/internal/deploy"
	"github.com/Zeptile/docktrine/internal/secrets"
	"github.com/Zeptile/docktrine/internal/docker"
	"github.com/Zeptile/docktrine/internal/logger"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	docker   *docker.DockerClient
	db       *database.DB
	deployer *deploy.Deployer
	box      *secrets.Box
}

func NewHandler(db *database.DB, deployer *deploy.Deployer, box *secrets.Box) *Handler {
	return &Handler{
		docker:   docker.NewDockerClient(db),
		db:       db,
		deployer: deployer,
		box:      box,
	}
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
	pullLatest := c.Query("pull_latest", "false") == "true"

	logger.Debug(fmt.Sprintf("Restarting container: %s", containerID))
	
	if containerID == "" {
		logger.Warn("Container ID is required")
		return c.Status(400).JSON(fiber.Map{
			"error": "container ID is required",
		})
	}


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

// CreateContainer godoc
// @Summary Create a container
// @Description Create a new Docker container
// @Tags containers
// @Accept json
// @Produce json
// @Param server query string true "Server name"
// @Param name body string true "Container name"
// @Param image body string true "Container image"
// @Param ports body []string false "Container ports"
// @Param env body []string false "Container environment variables"
// @Param volumes body []string false "Container volumes"
// @Param auto_start body boolean false "Auto start container"
// @Success 201 {object} interface{}
// @Failure 400 {object} interface{}
// @Failure 500 {object} interface{}
// @Router /containers/create [post]
func (h *Handler) CreateContainer(c *fiber.Ctx) error {
	serverName := c.Query("server", "")
	var request struct {
		Name       string                 `json:"name"`
		Image      string                 `json:"image"`
		Ports      []string               `json:"ports"`
		Env        []string               `json:"env"`
		Volumes    []string               `json:"volumes"`
		AutoStart  bool                   `json:"auto_start"`
	}

	if err := c.BodyParser(&request); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "invalid request body",
		})
	}

	config := &container.Config{
		Image: request.Image,
		Env:   request.Env,
	}

	hostConfig := &container.HostConfig{}
	
	if len(request.Ports) > 0 {
		portBindings := make(map[nat.Port][]nat.PortBinding)
		exposedPorts := make(nat.PortSet)
		
		for _, p := range request.Ports {
			binding, err := nat.ParsePortSpec(p)
			if err != nil {
				return c.Status(400).JSON(fiber.Map{
					"error": fmt.Sprintf("invalid port specification: %v", err),
				})
			}
			
			for _, b := range binding {
				portBindings[b.Port] = []nat.PortBinding{b.Binding}
				exposedPorts[b.Port] = struct{}{}
			}
		}
		
		hostConfig.PortBindings = portBindings
		config.ExposedPorts = exposedPorts
	}

	if len(request.Volumes) > 0 {
		hostConfig.Binds = request.Volumes
	}

	err := h.docker.CreateContainer(serverName, config, hostConfig, request.Name)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.Status(201).JSON(fiber.Map{
		"message": "Container created successfully",
	})
} 