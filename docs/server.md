# Server configuration

The server reads its settings from environment variables. The template with
comments is [deploy/.env.example](../deploy/.env.example). To run Docktrine
on a gateway, see [deploy/README.md](../deploy/README.md).

| Variable | Purpose |
| --- | --- |
| `DOCKTRINE_REGISTRY` | Registry every node pulls from and the CLI pushes to. |
| `DOCKTRINE_REGISTRY_USER`, `DOCKTRINE_REGISTRY_PASSWORD` | Registry login, if pulls need one. |
| `DOCKTRINE_NPM_URL`, `DOCKTRINE_NPM_EMAIL`, `DOCKTRINE_NPM_PASSWORD` | NPM admin API, used to create proxy hosts and certificates. |
| `DOCKTRINE_LETSENCRYPT_EMAIL` | Contact address for new Let's Encrypt certificates. |
| `DOCKTRINE_NPM_CERTIFICATE_ID` | Optional. Reuse an existing certificate, such as a wildcard, instead of requesting one per hostname. |
| `DOCKTRINE_SECRET_KEY` | Master key for secrets: 64 hex characters from `openssl rand -hex 32`. If unset, Docktrine generates `data/secret.key` on first start. Losing this key means losing every stored secret. |
| `CONFIG_PATH` | Where the database and keys are kept. Defaults to `data`. |
| `DOCKTRINE_STACKS_DIR` | Where nodes keep the files of compose stacks launched from Git. Defaults to `/var/lib/docktrine/stacks`. |
| `DOCKTRINE_UPDATE_INTERVAL` | How often to check environments for updates, e.g. `12h`. Defaults to `6h`; `off` leaves only the checks you ask for. |
| `DOCKTRINE_BACKUPS_DIR` | Where nodes keep backups of volumes. Defaults to `/var/lib/docktrine/backups`. |
| `DOCKTRINE_BACKUPS_KEEP` | How many backups each environment keeps, unless it sets its own. Defaults to `5`. |
| `PORT` | Port the API and UI listen on. Defaults to `3000`. |
