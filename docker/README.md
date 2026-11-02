# Docker Deployment

The deployment uses Docker Compose. The build expects the project source either to be cloned into `./src` by the script or to be present in `./src` when building locally.

## Prerequisites

- Docker and Docker Compose installed
- Git (if using the script to clone the repo)
- A Discord bot token from the [Discord Developer Portal](https://discord.com/developers/applications)
- **External network:** The Compose file uses a `proxy` network. Create it if it does not exist:

  ```bash
  docker network create proxy
  ```

## Configuration

`.env.example` here is in two parts, divided by a `BOT SETTINGS BELOW` line.
Above it is deploy-only — `ALIAS`, `GIT`, `GIT_URL` — which configures
`build-n-deploy.sh` and `docker-compose.yml` and never reaches the bot. Below it
is byte-identical to `.env.example` in the repository root, and a test keeps it
that way, so **edit the root file** and regenerate this one rather than changing
the copy.

Values are read literally, not evaluated. `build-n-deploy.sh` reads the settings
it needs rather than sourcing the file, because sourcing executes every line as
shell: a value containing `|` is a pipeline to anything that sources this file
rather than reading it. Quote any value containing `|`, `$` or spaces anyway —
anything else that reads this file may not be as careful.

Copy `.env.example` to `.env` in this directory and set at least:

- `DISCORD_TOKEN` — your bot token (required)
- `TASKS_PATH` — readable task list, normally `/usr/project/data/default_task.list.json` inside the container
- `HOST` and `SHORTLINK_BASE_URL` — the HTTP route hostname and its public origin
- `ALIAS` — container name and image tag (e.g. `server-domme`)
- `GIT` / `GIT_URL` — set `GIT=true` to clone the repo into `./src`; set `GIT=false` to use an existing `./src` directory

The [configuration reference](../docs/running.md#configuration) describes the
application settings. Keep storage/task paths under `/usr/project/data` for
persistence through the mounted volume. Uploaded `assets/media/` is not mounted
by the supplied Compose file; add persistent storage for it if you use uploads.

`docker-compose.yml` passes an explicit list of variables to the container, so
a setting that is not named there cannot be set from `.env` at all — it will
silently use its default. `TestEveryConfigVarIsPassedThroughDockerCompose`
keeps that list in step with the app config; if you add a setting, add it to
the compose file too.

Notes on recovery modes:

- `DISCORD_UNHEALTHY_MODE=restart-session` restarts the Discord gateway session when the gateway-silence watchdog marks it unhealthy.
- `DISCORD_UNHEALTHY_MODE=ignore` logs warnings only and leaves the session running.

## Deployment

**Option 1 — Build and deploy (recommended)**  
From this directory (`docker/`), run:

```bash
./build-n-deploy.sh
```

This loads `.env`, clones the repo into `./src` (or uses existing `./src`), builds the image, and starts the container.

**Option 2 — Compose only**  
If the image is already built:

```bash
docker compose -f docker-compose.yml up -d
```

Data under `/usr/project/data` is persisted in host `./data`. The build/deploy
script seeds the default task list only if it is missing. Review these paths
before recreating a container; a file written outside the mounts is not
preserved by this Compose configuration.
