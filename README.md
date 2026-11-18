# Docktrine

A Docker container management tool with REST API, CLI interfaces, and a modern web UI.

## Features

- **Web UI**: Modern Svelte 5 SPA with real-time container management
- **REST API**: Full-featured API for container and server management
- **Interactive CLI**: Command-line interface with auto-completion
- **Swagger Documentation**: API docs at `/api/swagger`
- **Container Operations**:
  - List containers across multiple servers
  - Start/Stop containers
  - Restart containers with optional image pull
  - Get detailed container information
- **Server Management**: Add and manage multiple Docker servers
- **Multi-server Support**: Manage containers across different Docker hosts
- **Request Logging**: Comprehensive logging and telemetry

## Installation

Install using Go (WIP, not functional yet):
`go install github.com/Zeptile/docktrine/cmd/...@latest`

## Usage

### Web UI

Start the server and access the web interface:

```bash
go run cmd/api/main.go
# Open http://localhost:3000 in your browser
```

The web UI provides:

- Visual container management with status indicators
- Server configuration management
- Real-time updates
- Modern, responsive design

### API Server

`docktrine-api` # Starts server on :3000

- Web UI: `http://localhost:3000`
- API Documentation: `http://localhost:3000/api/swagger`

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
- Node.js 18+ (for web UI)
- Swag (for docs)

### API Development

```bash
go run cmd/api/main.go # Start API server
```

### Web UI Development

```bash
cd web
npm install
npm run dev # Development server at :5173
npm run build # Build for production
```

### Building

```bash
# Build web UI
cd web && npm run build && cd ..

# Build API server
go build -o bin/docktrine cmd/api/main.go

# Run
./bin/docktrine
```

## Architecture

- **Backend**: Go with Fiber web framework
- **Frontend**: Svelte 5 SPA with TypeScript
- **Styling**: Tailwind CSS with shadcn/ui-inspired components
- **Database**: SQLite for server configuration
- **Docker Integration**: Docker API client

## License

MIT License - See LICENSE file
