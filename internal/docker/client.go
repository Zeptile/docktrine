package docker

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/Zeptile/docktrine/internal/database"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/gofiber/fiber/v2"
)

type DockerClient struct {
	db *database.DB
}

func NewDockerClient(db *database.DB) *DockerClient {
	return &DockerClient{
		db: db,
	}
}

func (d *DockerClient) newClient(serverName string) (*client.Client, error) {
	var server *database.Server
	var err error

	if serverName != "" {
		server, err = d.db.GetServerByName(serverName)
	} else {
		server, err = d.db.GetDefaultServer()
	}

	if err != nil {
		return nil, err
	}

	if server == nil {
		return nil, fmt.Errorf("server not found")
	}

	return client.NewClientWithOpts(
		client.WithHost(server.Host),
		client.WithAPIVersionNegotiation(),
		client.WithTimeout(15*time.Second),
	)
}

func (d *DockerClient) ListContainers(serverName string) ([]fiber.Map, error) {
	cli, err := d.newClient(serverName)
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	containers, err := cli.ContainerList(context.Background(), container.ListOptions{
		All: true,
	})
	if err != nil {
		return nil, err
	}

	var containerDetails []fiber.Map
	for _, c := range containers {
		inspect, err := cli.ContainerInspect(context.Background(), c.ID)
		if err != nil {
			continue
		}

		containerDetails = append(containerDetails, fiber.Map{
			"id":      inspect.ID,
			"name":    inspect.Name,
			"image":   inspect.Image,
			"state":   inspect.State,
			"created": inspect.Created,
			"status":  inspect.State.Status,
			"ports":   c.Ports,
			"labels":  inspect.Config.Labels,
		})
	}
	
	return containerDetails, nil
}

func (d *DockerClient) RestartContainer(containerID string, serverName string, pullLatest bool) error {
	cli, err := d.newClient(serverName)
	if err != nil {
		return err
	}
	defer cli.Close()

	if pullLatest {
		inspect, err := cli.ContainerInspect(context.Background(), containerID)
		if err != nil {
			return err
		}

		if err := cli.ContainerStop(context.Background(), containerID, container.StopOptions{}); err != nil {
			return err
		}

		if err := cli.ContainerRemove(context.Background(), containerID, container.RemoveOptions{
			RemoveVolumes: false,
			Force:        true,
		}); err != nil {
			return err
		}

		imageName := inspect.Config.Image
		if len(imageName) > 0 && !containsImageTag(imageName) {
			imageName = imageName + ":latest"
		}

		reader, err := cli.ImagePull(context.Background(), imageName, image.PullOptions{
			All:           false,
			RegistryAuth: "",
		})
		if err != nil {
			return err
		}

		inspect.Config.Image = normalizeImageName(imageName)

		defer reader.Close()
		io.Copy(io.Discard, reader)

		createResp, err := cli.ContainerCreate(context.Background(),
			inspect.Config,
			inspect.HostConfig,
			nil,
			nil,
			inspect.Name,
		)
		if err != nil {
			return err
		}

		return cli.ContainerStart(context.Background(), createResp.ID, container.StartOptions{})
	}

	return cli.ContainerRestart(context.Background(), containerID, container.StopOptions{})
}

func (d *DockerClient) StartContainer(containerID string, serverName string) error {
	cli, err := d.newClient(serverName)
	if err != nil { 
		return err
	}
	defer cli.Close()

	return cli.ContainerStart(context.Background(), containerID, container.StartOptions{})
}

func (d *DockerClient) StopContainer(containerID string, serverName string) error {
	cli, err := d.newClient(serverName)
	if err != nil {
		return err
	}
	defer cli.Close()

	return cli.ContainerStop(context.Background(), containerID, container.StopOptions{})
}

func (d *DockerClient) GetContainer(containerID string, serverName string) (fiber.Map, error) {
	cli, err := d.newClient(serverName)
	if err != nil {
		return nil, err
	}
	defer cli.Close()

	inspect, err := cli.ContainerInspect(context.Background(), containerID)
	if err != nil {
		return nil, err
	}

	return fiber.Map{
		"id":      inspect.ID,
		"name":    inspect.Name,
		"image":   inspect.Image,
		"state":   inspect.State,
		"created": inspect.Created,
		"status":  inspect.State.Status,
	}, nil
}

func (d *DockerClient) CreateContainer(serverName string, config *container.Config, hostConfig *container.HostConfig, name string) error {
	cli, err := d.newClient(serverName)
	if err != nil {
		return err
	}
	defer cli.Close()

	_, err = cli.ImagePull(context.Background(), config.Image, image.PullOptions{})
	if err != nil {
		return err
	}

	config.Image = normalizeImageName(config.Image)

	resp, err := cli.ContainerCreate(
		context.Background(),
		config,
		hostConfig,
		nil,
		nil,
		name,
	)
	if err != nil {
		return err
	}

	// Start container if requested
	return cli.ContainerStart(context.Background(), resp.ID, container.StartOptions{})
}

func normalizeImageName(imageName string) string {
	if imageName == "" || !strings.Contains(imageName, ":") {
		return imageName
	}

	lastIndex := strings.LastIndex(imageName, ":")
	portionBeforeColon := imageName[:lastIndex]
	if !strings.Contains(portionBeforeColon, "/") {
		return portionBeforeColon
	}
	
	parts := strings.Split(imageName, "/")
	lastPart := parts[len(parts)-1]
	if strings.Contains(lastPart, ":") {
		parts[len(parts)-1] = strings.Split(lastPart, ":")[0]
		return strings.Join(parts, "/")
	}

	return imageName
}

func containsImageTag(imageName string) bool {
	for i := len(imageName) - 1; i >= 0; i-- {
		if imageName[i] == ':' {
			return true
		}
		if imageName[i] == '/' {
			return false
		}
	}
	return false
} 