# CLI reference

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
| `rollback <env>` | Redeploy the previous successful version, or the deployment given by `--to <id>`. |
| `destroy <env>` | Remove containers and hostnames. `--purge` also deletes volumes. |
| `secrets list\|set\|unset <env>` | See [Secrets](manifest.md#secrets). |
| `servers list\|add\|update\|remove` | Manage nodes. |
| `servers health <name>` | Check that a node is reachable, and its Docker and ingress status. |
| `servers ingress <name>` | Install or update the node's Traefik. `--port` (default 8880), `--bind`. |
| `servers prune <name>` | Remove dangling images. `--all` removes every image no container uses, including ones Docktrine didn't create. |
| `keys list\|create\|rename\|revoke` | Manage API keys. See [Users and API keys](../deploy/README.md#users-and-api-keys). |
| `containers list\|start\|stop\|restart` | Act on individual containers of a node directly. |
