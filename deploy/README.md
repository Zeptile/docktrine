# Running Docktrine on the gateway

Docktrine runs as one container on the gateway, next to Nginx Proxy Manager.
It reaches each node over SSH, so nodes need only Docker and an SSH user; no
agent and no exposed Docker port.

```
developer CLI ──HTTPS──▶ NPM ──▶ Docktrine (gateway, 127.0.0.1:3000)
                                    │  ssh://deploy@node
                                    ▼
                         node: Docker + Traefik ingress ◀── NPM proxy hosts
```

## 1. Prepare each node

Create a user Docktrine connects as, with access to Docker:

```sh
sudo adduser --disabled-password deploy
sudo usermod -aG docker deploy
```

Membership in the `docker` group is equivalent to root on that node, so this
user should only accept the Docktrine key (next step).

## 2. SSH key and host keys

On the gateway, next to this `docker-compose.yml`:

```sh
mkdir -p ssh data
ssh-keygen -t ed25519 -N "" -C docktrine -f ssh/id_ed25519
cp ssh/config.example ssh/config   # then edit the Host entries
```

Add `ssh/id_ed25519.pub` to `~deploy/.ssh/authorized_keys` on every node.

Record each node's host key so connections are verified (Docktrine runs ssh
in batch mode and refuses unknown hosts):

```sh
ssh-keyscan prd1-tor.zeptile.software >> ssh/known_hosts
```

Check the fingerprints against the nodes (`ssh-keygen -lf
/etc/ssh/ssh_host_ed25519_key.pub` on the node). The `known_hosts` entries
must use the names Docktrine connects with: for a `Host prd1-tor` alias in
`ssh/config` with `HostName prd1-tor.zeptile.software`, use the HostName.

At startup the container copies `ssh/` into place with the permissions ssh
requires, so the files' owner and mode on the gateway don't matter.

## 3. Configure

```sh
cp .env.example .env
```

Fill in the registry and NPM settings. Keep `.env` and `data/` private:
`data/` holds the database, the secrets master key (`secret.key`, unless
`DOCKTRINE_SECRET_KEY` is set) and the first API key.

Every node pulls images from `DOCKTRINE_REGISTRY`, and developers push to it
with their own `docker login`. If pulling needs credentials, set
`DOCKTRINE_REGISTRY_USER` and `DOCKTRINE_REGISTRY_PASSWORD`.

## 4. Get the image and start

Build it on a machine with this repository and push it to the registry:

```sh
IMAGE=registry.zeptile.software/docktrine:latest ./build.sh image
docker push registry.zeptile.software/docktrine:latest
```

Then on the gateway:

```sh
DOCKTRINE_IMAGE=registry.zeptile.software/docktrine:latest docker compose up -d
docker compose ps        # should become "healthy"
cat data/initial-api-key # the first API key; delete the file once saved
```

The first key is named "Default API key". Rename it after whoever holds it,
then create one key per person and system (see [Users and API keys](#users-and-api-keys)).

Building on the gateway itself also works (`./build.sh image` there, then
`docker compose up -d`).

## 5. Expose the UI through NPM

Create a proxy host such as `docktrine.zeptile.software` forwarding to
`127.0.0.1:3000`, with SSL. If NPM itself runs in Docker it can't reach the
gateway's loopback: uncomment the `networks` section in `docker-compose.yml`
to join NPM's network and forward to `docktrine:3000` instead.

Consider an NPM access list on top of the API key, since the key controls
every node. Log streaming already disables proxy buffering
(`X-Accel-Buffering: no`), so it works through NPM unchanged.

## 6. Add nodes

From any machine with the CLI (`./build.sh cli` builds it into `dist/`):

```sh
export DOCKTRINE_API_URL=https://docktrine.zeptile.software
export DOCKTRINE_API_KEY=<key>

docktrine servers add --name prd1-tor --host ssh://deploy@prd1-tor \
  --ingress-host <host> --ingress-port <port>
docktrine servers health prd1-tor
docktrine servers ingress prd1-tor --port 8880
```

- `--host` uses the name from `ssh/config` (or the real hostname).
- `servers ingress` starts Traefik on the node, listening on `--port`. Add
  `--bind 127.0.0.1` when the gateway reaches the node through an SSH tunnel.
- `--ingress-host`/`--ingress-port` are where NPM on the gateway reaches that
  Traefik port: the node's address directly, or the gateway end of the
  node's tunnel. One tunnel per node covers every app on it.

The Servers page in the UI does the same.

## Users and API keys

People sign in to the UI with a username and password; scripts and the CLI
use API keys. On first start there are no users yet, so sign in to the UI
with the key from `data/initial-api-key`, open Users, and invite yourself.
An invite is a link, valid for 7 days, where the person chooses their
password.

Each user has a role:

| Role | Can |
| --- | --- |
| viewer | See apps, logs, containers and servers |
| deployer | Also deploy, roll back, manage secrets, start and stop containers |
| admin | Also manage servers, users and everyone's API keys |

Each person creates their own API keys (on their Account page, or with the
CLI); a key acts with its owner's role and stops working when the owner is
removed. Keys made before there were users, or with `create-key` below, have
no owner and act as admin:

```sh
docktrine keys create alex-laptop   # prints the key once
docktrine keys create ci
docktrine keys list
docktrine keys revoke alex-laptop
```

Keys are stored hashed, so a copy of the database doesn't contain usable
keys. Until someone can sign in with a password, the last key can't be
revoked.

If you're locked out, from a shell on the gateway either make yourself an
admin (prints a fresh invite link) or create an admin key:

```sh
docker compose exec docktrine /app/docktrine-api invite zeptile
docker compose exec docktrine /app/docktrine-api create-key zeptile
```

## Upgrades and backups

```sh
docker compose pull && docker compose up -d
```

Some releases change the node ingress. Reinstalling it on each node takes a
second, and running apps keep running (their hostnames are briefly
unreachable):

```sh
docktrine servers ingress <node> --port <port> [--bind 127.0.0.1]
```

A node whose ingress predates route checks still deploys, falling back to a
fixed wait; deploy logs say when a reinstall would help.

Back up `data/` (and `.env`). Losing `secret.key` or `DOCKTRINE_SECRET_KEY`
makes every stored secret unreadable.

## Current limitations

- A compose service that changes is stopped before its replacement starts,
  so it is briefly down. Single-image apps swap with no downtime.
