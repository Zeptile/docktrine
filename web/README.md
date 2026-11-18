# Docktrine Web UI

A modern Svelte 5 SPA for managing Docker containers and servers.

## Features

- **Container Management**: View, start, stop, and restart Docker containers
- **Server Management**: Add and manage multiple Docker servers
- **Modern UI**: Built with Tailwind CSS and shadcn/ui-inspired components
- **Real-time Updates**: Live status updates for containers
- **Multi-server Support**: Manage containers across different Docker hosts

## Development

### Prerequisites

- Node.js 18+
- npm

### Setup

```bash
# Install dependencies
npm install

# Start development server
npm run dev
```

The development server will run at `http://localhost:5173`

### Building

```bash
# Build for production
npm run build
```

Built files will be output to `dist/` directory.

## Architecture

- **Svelte 5**: Using the latest Svelte features including runes ($state, $effect)
- **TypeScript**: Full type safety
- **Tailwind CSS**: Utility-first styling
- **Vite**: Fast build tool and dev server
- **API Integration**: Communicates with Go backend API

## API Integration

The UI communicates with the Docktrine API backend:

- Container operations: `/api/containers/*`
- Server management: `/api/servers/*`
- API documentation: `/api/swagger/`

## Components

- `ContainerList.svelte`: Main container management interface
- `ServerList.svelte`: Server configuration management
- `ui/Button.svelte`: Reusable button component
- `ui/Card.svelte`: Card layout component
