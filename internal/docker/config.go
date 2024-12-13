package docker

import (
	"encoding/json"
	"os"
)

type ServerConfig struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Description string `json:"description,omitempty"`
	Default     bool   `json:"default,omitempty"`
}

type Config struct {
	Servers []ServerConfig `json:"servers"`
}

func LoadConfig(path string) (*Config, error) {
	defaultConfig := &Config{
		Servers: []ServerConfig{
			{
				Name:        "local",
				Host:        "unix:///var/run/docker.sock",
				Description: "Local Docker daemon",
				Default:     true,
			},
		},
	}

	if path == "" {
		return defaultConfig, nil
	}

	file, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return defaultConfig, nil
		}
		return nil, err
	}

	var config Config
	if err := json.Unmarshal(file, &config); err != nil {
		return nil, err
	}

	hasDefault := false
	for _, server := range config.Servers {
		if server.Default {
			hasDefault = true
			break
		}
	}

	if !hasDefault && len(config.Servers) > 0 {
		config.Servers[0].Default = true
	}

	return &config, nil
} 