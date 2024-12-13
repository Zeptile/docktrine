# Docktrine

A Docker container management tool with both REST API and CLI interfaces.

## Features

- REST API for container management
- Interactive CLI with auto-completion
- Swagger documentation at `/swagger`
- Container operations:
  - List containers
  - Start/Stop containers
  - Restart containers with optional image pull
  - Get detailed container information
- Multi-server support via configuration
- Request logging and telemetry

## Installation

Install using Go (WIP, not functional yet):
`go install github.com/Zeptile/docktrine/cmd/...@latest`

## Usage

### API Server

`docktrine-api` # Starts server on :3000

### CLI Tool

```bash
`docktrine containers list` # List containers
`docktrine containers start <id>` # Start container
`docktrine containers stop <id>` # Stop container
`docktrine containers restart <id>` # Restart container
`docktrine interactive` # Interactive mode
```

### Configuration

Create a config.json file to specify Docker servers:

```json
{
  "servers": [
    {
      "name": "local",
      "host": "unix:///var/run/docker.sock",
      "default": true
    }
  ]
}
```

## Development

Prerequisites:

- Go 1.23+
- Docker
- Swag (for docs)

Run locally:
`go run cmd/api/main.go` # API
`go run cmd/cli/main.go` # CLI

## License

MIT License - See LICENSE file
