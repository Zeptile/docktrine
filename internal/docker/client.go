package docker

import (
	"context"
	"fmt"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/client"
	"github.com/gofiber/fiber/v2"
)

type DockerClient struct {}


func (d *DockerClient) newClient(serverName string) (*client.Client, error) {
	var targetServer *ServerConfig
	var defaultServer *ServerConfig

	config, err := LoadConfig("config.json")
	if err != nil {
		return nil, err
	}

	for _, server := range config.Servers {
		if serverName != "" && server.Name == serverName {
			targetServer = &server
			break
		}
		if server.Default {
			defaultServer = &server
		}
	}

	if targetServer == nil {
		targetServer = defaultServer
	}

	if targetServer == nil {
		return nil, fmt.Errorf("no server specified and no default server found")
	}


	return client.NewClientWithOpts(
		client.WithHost(targetServer.Host),
		client.WithAPIVersionNegotiation(),
		client.WithTimeout(5*time.Second),
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

		_, err = cli.ImagePull(context.Background(), inspect.Config.Image, image.PullOptions{})
		if err != nil {
			return err
		}
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