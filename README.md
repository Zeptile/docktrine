# Docktrine

Deploy apps to your own Docker servers from the command line.

Each repository describes what it needs in a `docktrine.yaml`. Then
`docktrine deploy prod` builds the image, pushes it to your registry, starts
it on the right node with zero downtime, and points its hostnames at it
through Nginx Proxy Manager.

```sh
docktrine deploy dev           # build, push, deploy, route hostnames
docktrine deploy preview       # a temporary copy of the current branch
docktrine logs prod -f
docktrine rollback prod
```

A web UI shows every app, its deployment history and logs, and lets you
redeploy, roll back, manage secrets and nodes.

## Contents

- [How it works](#how-it-works)
- [Quick start](#quick-start)
- [docktrine.yaml](#dockertrineyaml)
- [Secrets](#secrets)
- [CLI reference](#cli-reference)
- [Server configuration](#server-configuration)
- [Development](#development)

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

- **Apps and environments.** An app is one repository. It has environments
  such as `dev` and `prod`, and each environment runs on one node. Every
  deploy is recorded with who made it, so any earlier version can be
  redeployed.
- **Nodes.** Docktrine reaches each node's Docker daemon over SSH
  (`ssh://user@host`), or over a `unix://` or `tcp://` host. A node over SSH
  needs only Docker and an SSH user: no agent and no exposed Docker port.
- **One ingress per node.** Each node runs a Traefik container that routes
  requests by hostname to app containers, configured through container
  labels. The gateway's Nginx Proxy Manager (NPM) forwards every hostname of
  a node to that node's one ingress port, so adding an app never needs a new
  tunnel or port.
- **Hostnames and certificates.** Docktrine creates an NPM proxy host for
  each hostname, with a Let's Encrypt certificate or an existing wildcard
  certificate. It removes them when the environment is destroyed. DNS must
  already point the hostnames at the gateway. A wildcard record such as
  `*.dev.zeptile.software` covers every app.
- **Zero-downtime deploys.** The new container starts next to the old one.
  The old one stops only after the new one passes its health check and the
  ingress routes traffic to it. If either step fails, the new container is
  removed and the old one keeps serving. A successful deploy means its
  hostnames answer.

Containers that Docktrine creates carry `docktrine.app` and `docktrine.env`
labels, so they are easy to tell apart from containers started by hand.

## Quick start

You need a Docktrine server on your gateway. See
[deploy/README.md](deploy/README.md) for setup.

**On your machine:**

1. **Install the CLI.** Run `./build.sh cli`, which builds binaries for macOS
   and Linux into `dist/`. Put the one for your machine on your `PATH` as
   `docktrine`.
2. **Point it at the server** with an API key that someone created for you
   (`docktrine keys create <your-name>`):

   ```sh
   export DOCKTRINE_API_URL=https://docktrine.zeptile.software
   export DOCKTRINE_API_KEY=dkt_...
   ```

3. **Log in to your registry** with `docker login <registry>`. The CLI builds
   and pushes images from your machine with `docker buildx`.
4. **Write a manifest.** In a repository, run `docktrine init`. It writes a
   `docktrine.yaml`, using your compose file if it finds one. Review the
   nodes and hostnames.
5. **Deploy.** Run `docktrine deploy dev`.

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
| `node` | The server the environment runs on. Required. |
| `hostnames` | Public hostnames. Requires `port`, or `expose` for compose apps. |
| `port` | The port the app listens on inside the container. |
| `env` | Plain environment variables, committed with the repository. |
| `secrets` | Names of secrets to inject as environment variables. Values live in Docktrine, never in the repository. Only the names listed here are injected. |
| `volumes` | `name:/path` creates a named volume scoped to the environment (`data` becomes `invoicer-prod-data`) and keeps it across deploys. `/host/path:/path` bind-mounts a directory on the node. |
| `healthcheck` | An HTTP path that must answer successfully before the new version takes traffic. Docktrine runs `wget` or `curl` inside the container, so the image needs one of them. Distroless and scratch images have neither. Without a health check, the container only has to stay up for 10 seconds. |
| `command` | Overrides the image's command. |
| `ttl` | Preview environments only. How long a preview lives after its last deploy, e.g. `72h`. |

Paths in the file, such as the build context and compose file, are relative to
`docktrine.yaml`. Unknown fields are rejected, so a typo fails loudly.

### Images and tags

With `build`, `docktrine deploy` builds and pushes `<registry>/<app>:<tag>`.
The registry comes from the server's configuration. The tag defaults to the
short git commit, with `-dirty` added when there are uncommitted changes.
`--tag` sets it explicitly.

`--no-build` deploys a tag that has already been pushed. For example, you can
deploy to prod exactly what you tested on dev.

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

Docktrine runs `docker compose up` on the node with your compose file plus an
override that adds its labels and routing. `env` and `secrets` are available
to the compose file as `${VAR}`, and they are also set on the exposed service.
Only services whose configuration changed are recreated. The deploy succeeds
once the ingress routes to the exposed service.

Compose stops a container before starting its replacement. A changed service
is therefore briefly down during a deploy, unlike single-image apps.

### Preview environments

An app with a `preview` environment can deploy any branch as its own
temporary environment:

```sh
git checkout feature/login
docktrine deploy preview       # → preview-feature-login
docktrine destroy preview      # or let the ttl expire
```

`{branch}` in a hostname becomes the branch name in DNS-safe form, so
`feature/login` becomes `feature-login`. Each deploy restarts the `ttl`. When
it runs out, the preview is removed within about ten minutes, volumes
included. All previews of an app share the secrets set on `preview`.

## Secrets

```sh
docktrine secrets set prod DATABASE_URL=postgres://... STRIPE_KEY=sk_live_...
docktrine secrets list prod
docktrine secrets unset prod STRIPE_KEY
```

Values are encrypted with AES-256-GCM on the server and are never shown
again. `list` shows names only. Changes take effect on the next deploy. A
deploy that needs a secret which isn't set fails immediately, before touching
the running version. The Secrets tab of an environment in the UI flags any
missing secrets.

## CLI reference

Every command talks to the server named by `DOCKTRINE_API_URL`, using
`DOCKTRINE_API_KEY`. You can also pass `--api-url` and `--api-key`.

App commands read the app name from `docktrine.yaml` in the current
directory, or take `--app`. Preview commands target the current git branch,
or `--branch`.

| Command | What it does |
| --- | --- |
| `init [name]` | Write a starting `docktrine.yaml`. |
| `deploy <env>` | Build, push and deploy. Flags: `--tag`, `--no-build`, `--branch`, `--detach`. |
| `status` | What runs where, for this app. `--all` shows every app. |
| `history <env>` | Deployments of an environment, newest first. |
| `logs <env>` | Container logs. `-f` follows, `--tail N` limits lines. |
| `rollback <env>` | Redeploy the previous successful version. |
| `destroy <env>` | Remove containers and hostnames. `--purge` also deletes volumes. |
| `secrets list\|set\|unset <env>` | See [Secrets](#secrets). |
| `servers list\|add\|update\|remove` | Manage nodes. |
| `servers health <name>` | Check that a node is reachable, and its Docker and ingress status. |
| `servers ingress <name>` | Install or update the node's Traefik. `--port` (default 8880), `--bind`. |
| `servers prune <name>` | Remove dangling images. `--all` removes every image no container uses, including ones Docktrine didn't create. |
| `keys list\|create\|rename\|revoke` | Manage API keys. See [Users and API keys](deploy/README.md#users-and-api-keys). |
| `containers list\|start\|stop\|restart` | Act on individual containers of a node directly. |

## Server configuration

The server reads its settings from environment variables. The template with
comments is [deploy/.env.example](deploy/.env.example).

| Variable | Purpose |
| --- | --- |
| `DOCKTRINE_REGISTRY` | Registry every node pulls from and the CLI pushes to. |
| `DOCKTRINE_REGISTRY_USER`, `DOCKTRINE_REGISTRY_PASSWORD` | Registry login, if pulls need one. |
| `DOCKTRINE_NPM_URL`, `DOCKTRINE_NPM_EMAIL`, `DOCKTRINE_NPM_PASSWORD` | NPM admin API, used to create proxy hosts and certificates. |
| `DOCKTRINE_LETSENCRYPT_EMAIL` | Contact address for new Let's Encrypt certificates. |
| `DOCKTRINE_NPM_CERTIFICATE_ID` | Optional. Reuse an existing certificate, such as a wildcard, instead of requesting one per hostname. |
| `DOCKTRINE_SECRET_KEY` | Master key for secrets: 64 hex characters from `openssl rand -hex 32`. If unset, Docktrine generates `data/secret.key` on first start. Losing this key means losing every stored secret. |
| `CONFIG_PATH` | Where the database and keys are kept. Defaults to `data`. |
| `PORT` | Port the API and UI listen on. Defaults to `3000`. |

## Development

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

### Project layout

| Path | Contents |
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

MIT. See [LICENSE](LICENSE).
