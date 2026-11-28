# Development

Prerequisites: Go 1.23+, Node.js 18+, and Docker.

```sh
go run ./cmd/api                        # API and built UI on :3000
cd web && npm install && npm run dev    # UI with hot reload on :5173
go run ./cmd/cli status                 # the CLI, against DOCKTRINE_API_URL
go test ./...
```

- The dev UI proxies `/api` to `http://localhost:3000`. Set
  `DOCKTRINE_API_PROXY` to proxy to another API.
- On first start the API creates an API key and writes it to
  `data/initial-api-key`.
- API documentation is served at `/api/swagger`. After changing handler
  annotations, regenerate it with `./swagger-gen.sh`.
- `./build.sh` builds the UI, API and CLI for this machine. `./build.sh cli`
  cross-compiles the CLI. `./build.sh image` builds the server image.

## Project layout

| Path | Contents |
| --- | --- |
| `cmd/api` | HTTP API (Fiber) and the UI's static files |
| `cmd/cli` | The `docktrine` CLI (Cobra) |
| `internal/manifest` | `docktrine.yaml` parsing and resolution |
| `internal/deploy` | Deploy engine: single-image swaps, compose, ingress, previews |
| `internal/catalog` | Known apps for the launch form |
| `internal/console` | Shells in containers and on nodes, over WebSockets |
| `internal/npm` | Nginx Proxy Manager API client |
| `internal/secrets` | Secret encryption |
| `internal/database` | SQLite storage |
| `web` | Svelte UI |
| `deploy` | Running Docktrine on the gateway |
