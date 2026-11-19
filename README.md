# Docktrine

Docktrine deploys apps to your own Docker servers. Each repository describes
what it needs in a `docktrine.yaml`; `docktrine deploy prod` builds the image,
pushes it to your registry, starts it on the right node with zero downtime,
and points its hostnames at it through Nginx Proxy Manager.

```sh
docktrine deploy dev           # build, push, deploy, route hostnames
docktrine deploy preview       # a temporary copy of the current branch
docktrine logs prod -f
docktrine rollback prod
```

A web UI shows every app, its deployment history and logs, and lets you
redeploy, roll back, manage secrets and nodes.

## How it works

```
 developer laptop / CI                     gateway
 ┌──────────────────┐   HTTPS + API key   ┌───────────────────────────────┐
 │ docktrine CLI    │────────────────────▶│ Docktrine (API + UI)          │
 │ docker buildx    │                     │ Nginx Proxy Manager           │
 └────────┬─────────┘                     └──────┬──────────────┬─────────┘
          │ push                                 │ ssh          │ proxy hosts
          ▼                                      ▼              ▼
 ┌──────────────────┐   pull      ┌─────────────────────────────────────────┐
 │ central registry │────────────▶│ node: Docker + Traefik ingress          │
 └──────────────────┘             │   routes by hostname to app containers  │
                                  └─────────────────────────────────────────┘
```

- **Apps, environments, deployments.** An app (one repository) has
  environments such as `dev` and `prod`, each running on one node. Every
  deploy is recorded, with who made it, so any earlier version can be
  redeployed.
- **Nodes over SSH.** Docktrine reaches each node's Docker daemon with
  `ssh://user@host`. Nodes need Docker and an SSH user, nothing else.
- **One ingress per node.** Each node runs a Traefik container that routes
  requests by hostname to app containers, configured through container
  labels. The gateway's Nginx Proxy Manager forwards every hostname of a
  node to that one port, so adding an app never needs a new tunnel or port.
- **Hostnames and certificates.** Docktrine creates the NPM proxy host for
  each hostname, with a Let's Encrypt certificate (or an existing wildcard
  certificate), and removes it when the environment is destroyed. DNS must
  already point the hostnames at the gateway; a wildcard record such as
  `*.dev.zeptile.software` covers every app.
- **Zero-downtime deploys.** A new container starts next to the old one.
  The old one is stopped only once the new one passes its health check and
  the ingress confirms it's routing traffic to it. If either doesn't happen,
  the new container is removed and the old one keeps serving. When a deploy
  reports success, its hostnames answer.

Containers Docktrine creates carry `docktrine.app` and `docktrine.env`
labels, so they're easy to tell apart from anything started by hand.

## Getting started

To run the Docktrine server on the gateway, follow
[deploy/README.md](deploy/README.md). Then, on your machine:

1. Get the CLI: `./build.sh cli` builds it for macOS and Linux into `dist/`.
   Put the binary for your machine on your `PATH` as `docktrine`.
2. Point it at the server with a key someone created for you
   (`docktrine keys create <your-name>`):

   ```sh
   export DOCKTRINE_API_URL=https://docktrine.zeptile.software
   export DOCKTRINE_API_KEY=dkt_...
   ```

3. Log in to the registry with Docker (`docker login <registry>`), since the
   CLI builds and pushes images from your machine with `docker buildx`.
4. In a repository, run `docktrine init`. It writes a `docktrine.yaml`,
   using your compose file if it finds one. Review the nodes and hostnames.
5. `docktrine deploy dev`.

## docktrine.yaml

```yaml
app: invoicer                  # lowercase letters, digits and dashes

# Exactly one of build, image or compose:
build:
  dockerfile: Dockerfile       # default: Dockerfile
  context: .                   # default: .
  platform: linux/amd64        # default: linux/amd64
  args:
    NODE_ENV: production
# image: ghcr.io/acme/tool:1.4  # run a prebuilt image instead
# compose: docker-compose.yml   # see "Compose apps" below

environments:
  dev:
    node: onsite               # a server name in Docktrine
    hostnames: [invoicer.dev.zeptile.software]
    port: 8080                 # container port the hostnames route to
    env:
      LOG_LEVEL: debug

  prod:
    node: prd1-tor
    hostnames: [invoicer.zeptile.software, www.invoicer.zeptile.software]
    port: 8080
    secrets: [DATABASE_URL, STRIPE_KEY]
    volumes:
      - data:/app/data         # named volume, kept across deploys
    healthcheck: /healthz
    command: ["node", "server.js", "--cluster"]

  preview:                     # optional, see "Preview environments"
    node: onsite
    hostnames: ["{branch}.invoicer.dev.zeptile.software"]
    port: 8080
    ttl: 72h
```

| Field | Meaning |
| --- | --- |
| `node` | Which server the environment runs on. Required. |
| `hostnames` | Public hostnames. Needs `port` (or `expose` for compose). |
| `port` | The port the app listens on inside the container. |
| `env` | Plain environment variables, committed with the repository. |
| `secrets` | Names of secrets to inject as environment variables. Values live in Docktrine, never in the repository; only the names listed here are injected. |
| `volumes` | `name:/path` creates a named volume scoped to the environment (`data` becomes `invoicer-prod-data`), kept across deploys. `/host/path:/path` bind-mounts a directory of the node. |
| `healthcheck` | An HTTP path that must answer successfully before the new version takes traffic. It runs `wget` or `curl` inside the container, so the image needs one of them (distroless and scratch images don't have either). Without a health check, the container only has to stay up for 10 seconds. |
| `command` | Overrides the image's command. |
| `ttl` | Preview environments only: how long a preview lives after its last deploy, e.g. `72h`. |

Paths in the file (build context, compose file) are relative to
`docktrine.yaml`. Unknown fields are rejected, so a typo fails loudly.

### Images and tags

With `build`, `docktrine deploy` builds and pushes
`<registry>/<app>:<tag>`. The registry comes from the server's configuration.
The tag defaults to the short git commit, with `-dirty` added when there are
uncommitted changes; `--tag` sets it explicitly. `--no-build` deploys a tag
that has already been pushed, e.g. to deploy to prod exactly what was
tested on dev.

### Compose apps

```yaml
app: analytics
compose: docker-compose.yml
expose:
  service: web                 # the service hostnames route to
  port: 3000
environments:
  prod:
    node: prd1-tor
    hostnames: [analytics.zeptile.software]
    env:
      REDIS_PASSWORD: ...      # available as ${REDIS_PASSWORD} in the compose file
    secrets: [DATABASE_URL]
```

Docktrine runs `docker compose up` on the node with the compose file plus
an override that adds its labels and routing. `env` and `secrets` are
available to the compose file as `${VAR}` and are also set on the exposed
service. Only services whose configuration changed are recreated, and the
deploy succeeds once the ingress routes to the exposed service. Compose
stops a container before starting its replacement, so unlike single-image
apps a changed service is briefly down during a deploy.

### Preview environments

An app with a `preview` environment can deploy any branch as its own
temporary environment:

```sh
git checkout feature/login
docktrine deploy preview       # → preview-feature-login
docktrine destroy preview      # or let the ttl expire
```

`{branch}` in hostnames becomes the branch name in DNS-safe form
(`feature/login` → `feature-login`). Every deploy restarts the `ttl`; once it
runs out, the preview is removed within about ten minutes, volumes included.
All previews of an app share the secrets set on `preview`.

## Secrets

```sh
docktrine secrets set prod DATABASE_URL=postgres://... STRIPE_KEY=sk_live_...
docktrine secrets list prod
docktrine secrets unset prod STRIPE_KEY
```

Values are encrypted (AES-256-GCM) on the server and never shown again;
`list` shows names only. Changes take effect on the next deploy. A deploy
that needs a secret that isn't set fails right away, before touching the
running version. The Secrets tab of an environment in the UI flags any
missing secrets.

## CLI reference

Every command talks to the server named by `DOCKTRINE_API_URL` using
`DOCKTRINE_API_KEY` (or `--api-url` / `--api-key`). App commands read the
app name from `docktrine.yaml` in the current directory, or take `--app`.
For `preview`, they target the current git branch, or `--branch`.

| Command | |
| --- | --- |
| `init [name]` | Write a starting `docktrine.yaml`. |
| `deploy <env>` | Build, push and deploy. `--tag`, `--no-build`, `--branch`, `--detach`. |
| `status` | What runs where, for this app (or every app with `--all`). |
| `history <env>` | Deployments of an environment, newest first. |
| `logs <env>` | Container logs. `-f` to follow, `--tail N`. |
| `rollback <env>` | Redeploy the previous successful version. |
| `destroy <env>` | Remove containers and hostnames. `--purge` also deletes volumes. |
| `secrets list\|set\|unset <env>` | See [Secrets](#secrets). |
| `servers list\|add\|remove\|update` | Manage nodes. |
| `servers health <name>` | Check a node is reachable; Docker and ingress status. |
| `servers ingress <name>` | Install or update the node's Traefik. `--port` (default 8880), `--bind`. |
| `servers prune <name>` | Remove dangling images. `--all` removes every image no container uses, including ones Docktrine didn't create. |
| `keys list\|create\|rename\|revoke` | See [API keys](deploy/README.md#api-keys). |
| `containers ...` | Act on individual containers of a node directly. |

## Development

Prerequisites: Go 1.23+, Node.js 18+, Docker.

```sh
go run ./cmd/api                 # API and built UI on :3000 (PORT to change)
cd web && npm install && npm run dev   # UI with hot reload on :5173
go run ./cmd/cli status          # the CLI against DOCKTRINE_API_URL
go test ./...
```

The dev UI proxies `/api` to `http://localhost:3000`; set
`DOCKTRINE_API_PROXY` to use another API. The API keeps its state in
`./data` (or `CONFIG_PATH`). On first start it creates an API key and writes
it to `data/initial-api-key`.

API documentation is served at `/api/swagger`. After changing handler
annotations, regenerate it with `./swagger-gen.sh`.

`./build.sh` builds the UI, API and CLI for this machine. `./build.sh cli`
cross-compiles the CLI, and `./build.sh image` builds the server image.

### Layout

| Path | |
| --- | --- |
| `cmd/api` | HTTP API (Fiber) and the UI's static files |
| `cmd/cli` | The `docktrine` CLI (Cobra) |
| `internal/manifest` | `docktrine.yaml` parsing and resolution |
| `internal/deploy` | Deploy engine: single-image swaps, compose, ingress, previews |
| `internal/npm` | Nginx Proxy Manager API client |
| `internal/secrets` | Secret encryption |
| `internal/database` | SQLite storage |
| `web` | Svelte UI |
| `deploy` | Running Docktrine on the gateway |

## License

MIT License - See LICENSE file
