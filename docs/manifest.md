# docktrine.yaml

Each repository describes how it runs in a `docktrine.yaml`. `docktrine init`
writes a starting one.

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
| `backup` | When deploys back up the volumes first, and how many backups to keep. See [Backups](operations.md#backups). |
| `ttl` | Preview environments only. How long a preview lives after its last deploy, e.g. `72h`. |

Paths in the file, such as the build context and compose file, are relative to
`docktrine.yaml`. Unknown fields are rejected, so a typo fails loudly.

## Images and tags

With `build`, `docktrine deploy` builds and pushes `<registry>/<app>:<tag>`.
The registry comes from the server's configuration. The tag defaults to the
short git commit, with `-dirty` added when there are uncommitted changes.
`--tag` sets it explicitly.

`--no-build` deploys a tag that has already been pushed. For example, you can
deploy to prod exactly what you tested on dev.

## Compose apps

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

## Preview environments

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
