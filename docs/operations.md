# Operations

## How deploys work

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

## Backups

Rolling back redeploys older code, but it can't undo a database migration.
So before a setup command runs, Docktrine backs up the environment first:

- **What.** Every named volume of the environment: the ones compose creates
  for the `<app>-<env>` project, and `name:/path` volumes of image apps. For
  stacks from Git, the stack's directory on the node too, since stacks keep
  configuration there (Sentry's generated config, for example). Directories
  on the node that containers mount, and volumes the environment didn't
  create, are listed as not backed up and left alone.
- **How.** A `busybox` helper container on the node writes one gzipped
  tarball per volume into `/var/lib/docktrine/backups/<app>-<env>/<time>/`,
  readable only by root. This works over SSH like everything else. The
  environment's containers are stopped while it runs, so databases are
  copied whole, and started again after. The deploy log shows the sizes and
  how long each step took. If the node doesn't have room for the backup, the
  deploy fails before changing anything.
- **When.** Configure sets it per environment under Setup (or Backups for
  apps without one): before setup commands (the default), also before every
  deploy that changes the version, meaning another image or digest, a newer
  image behind a tag like `latest`, or another commit, or never. **Back up
  now** on the environment's Backups tab takes one on demand.
- **Keeping.** The last 5 per environment are kept, or the number set under
  Configure. The newest backup that finished is never deleted automatically.
- **Restoring.** Restore on the Backups tab stops the environment and
  replaces its volumes with the backup's. It then redeploys what ran when
  the backup was taken, so the code matches the data, or keeps what runs now
  if you choose.

Destroying an environment keeps its backups, even when its volumes are
deleted, unless you choose to delete them too. Deploying the environment
again shows them, so a purged environment can still be restored. Previews
that expire take their backups with them.

In `docktrine.yaml`, an environment can set the same:

```yaml
environments:
  prod:
    backup:
      before: upgrade          # setup (default), upgrade or never
      keep: 10
```

## Shells

The UI can open a terminal in a running container, or on a server itself.

- **In a container.** From an environment's **Shell** tab, which lists its
  running services, or from the terminal icon on a running container under
  Containers. It runs `docker exec` with bash if the container has it, else
  sh, as the container's default user. This needs the deployer role.
- **On a server.** From **Shell** on the server under Servers. This is a root
  shell on the host, so it needs the admin role. Docktrine starts a
  privileged helper container in the host's process namespace
  (`busybox`, labelled `docktrine.role=helper`) that enters the host's
  namespaces with `nsenter`. It works over SSH like everything else.

A shell ends when you close it or leave the page, and after 30 minutes
without input or 12 hours in all. Anything still running in it is stopped,
and a server shell's helper container is removed. If the API stops
mid-session, it removes the helpers it left behind when it starts again;
any helper older than 12 hours is removed regardless.

Every shell opened and closed is logged with who opened it, where, and for
how long. Shells are WebSockets at `/api/containers/<id>/shell?server=` and
`/api/servers/<name>/shell`, and the API refuses ones started by another
site. Behind a reverse proxy, the proxy must pass WebSocket upgrades and
keep the `Host` header, or set `X-Forwarded-Host`.
