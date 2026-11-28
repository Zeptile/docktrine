# Launching apps from the UI

Apps you didn't write, such as Sentry, Plausible or Uptime Kuma, don't need a
repository of your own or a `docktrine.yaml`. Under **Apps → Launch an app**,
pick one of:

- **Container image**, e.g. `postgres:16`. Docktrine reads the image's
  config from its registry, fills in the port, and points out where it keeps
  data so you can put that in a volume.
- **Compose from Git**, a public repository such as
  `https://github.com/getsentry/self-hosted`. You can paste the address of a
  branch, tag or file from your browser. Docktrine reads the stack, suggests
  the service hostnames route to, and lists every variable the stack reads,
  with the defaults from its `.env` and descriptions from its comments.
- **Paste compose**, for a single compose file.

Or start from a **known app**: Uptime Kuma, n8n, Gitea, Umami, Plausible or
Sentry. Picking one fills the form with a pinned, tested version, the service
and port to route to, volumes, the setup command, and the variables it needs,
with which are required and which are secrets. Variables like Plausible's
`BASE_URL` follow the first hostname you give. Everything stays editable. The
known apps are also in the **Go to** palette (Ctrl+K) as "Launch Sentry" and so
on, and at `#/launch/<name>`.

The catalog is `internal/catalog/presets.yaml`, built into the server and
served at `GET /api/catalog`. To add an app, copy an entry of the same kind,
pin it to a released tag, launch it once, and check it against its upstream
with `DOCKTRINE_CATALOG_LIVE=1 go test ./internal/catalog/`. The plain
`go test` keeps every entry well-formed.

## Secrets and generated values

Variables can be stored as encrypted secrets with the lock next to each one.
Credentials such as `*_PASSWORD` and `*_TOKEN` are locked by default.

Next to each secret is a dice button that fills in a random value, made in
your browser with `crypto.getRandomValues`. It is 32 letters and digits, 50
for names like `SECRET_KEY`, or hex or base64 when the stack's comments or
`.env.example` suggest that. The value is shown so you can copy it, since it
isn't shown again after launch. When several secrets are empty, one button
generates them all. When you reconfigure an app, the button asks before it
replaces a stored secret, because the app may already be using the old one.

## Where stacks live

Each deploy of a stack from Git fetches the repository at the given branch,
tag or commit and records the commit it ran, so redeploying or rolling back
to that deployment runs exactly that commit again. The repository's files go
to the node, into `/var/lib/docktrine/stacks/<app>-<env>`, so the configs it
mounts with relative paths (`./nginx.conf:/etc/nginx/nginx.conf`) work there.
Docktrine adds and replaces files in that directory but never deletes any,
since stacks often keep data next to their compose file. Destroying the
environment with the volumes deleted removes the directory too.

**Configure** on an environment opens the same form, filled in from what
runs now, to change and redeploy it. An app deployed with the CLI can be
changed there too, but its next CLI deploy replaces those changes.

## Preflight checks

Before a launch, the form checks the chosen server and shows the results beside
the Launch button. The checks update as the form changes.

- **Resources.** Memory, CPUs and the Docker version are checked against what
  the stack needs. The minimums come from its install script: Sentry's
  `install/_min-requirements.sh` asks for 14000 MB and 4 CPUs, or 7000 MB and
  2 CPUs with `COMPOSE_PROFILES=errors-only`. The memory and CPUs its compose
  services reserve are checked too.
- **Ports.** Ports the stack publishes on the server are checked against other
  containers and other processes listening there.
- **Hostnames.** With hostnames, the server's ingress must be running, the
  server must have an ingress host and port for the gateway, and Nginx Proxy
  Manager must be configured.

A failed check stops the launch unless you choose to launch anyway. Warnings
don't stop it.

## Setup commands

Some stacks need a step before `docker compose up`. Sentry's `install.sh`,
for example, creates its configuration, builds images and migrates its
database. A stack from Git can have a **setup command** for this. When the
repository has an `install.sh`, the form fills it in, with whichever of its
options keep it from stopping to ask questions. For Sentry it also offers to
create the first user from `SENTRY_ADMIN_EMAIL` and `SENTRY_ADMIN_PASSWORD`.

The command runs on the node before the services start, in the stack's
directory there, in a `docker:cli` container with bash, the node's Docker,
and the environment's variables and secrets. `COMPOSE_PROJECT_NAME` is set
to `<app>-<env>`, the project Docktrine runs the stack as, whatever the
stack's own `.env` says. The command stops at the first step that fails, and
the deploy fails with it, before anything restarts. Its output goes to the
deploy log.

It runs on the first deploy, and again whenever the stack's commit or the
command changes, which is when install scripts expect to run. It can also
run on every deploy, or on one deploy when you ask for it under Configure.

Files the command creates in the stack's directory stay there across
deploys. Files from the repository are copied there again on each deploy,
so a command shouldn't edit them: anything it changed would be overwritten
while the setup command doesn't run again for the same commit.

## Updates

Docktrine looks for newer versions of what each environment runs: new
commits on a stack's branch, or on its tag if the tag was moved, and newer
images behind tags like `latest` or `16`, for apps and for the services of
compose stacks. A stack pinned to a commit, and images pinned by digest,
never update. Previews and images built on the node aren't checked.

Checks run in the background every six hours, and when you press **Check
now** on an environment. They ask Git with `git ls-remote` and registries
with a `HEAD` request for the tag's digest, which doesn't count against
Docker Hub's pull limit. Requests to the same host are spaced out, a host
that answers "too many requests" is left alone for as long as it asks, and
a check that fails waits longer each time before it runs again. For GitHub
and Codeberg, the update panel says how many commits are new and links to
the changes.

The apps list marks environments with an update, and the environment page
shows what changes. **Update** deploys the running settings again at the
newest version, as an ordinary deployment with its log, and a rollback to
go back. For a stack with a setup command, a new commit runs the command
again, which the page warns about: a migration it runs isn't undone by
rolling back. Unless backups are turned off for the environment, Docktrine
[backs it up](operations.md#backups) first.
