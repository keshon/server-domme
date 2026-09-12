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

Values are read literally, not evaluated. `build-n-deploy.sh` reads the three
settings it needs rather than sourcing the file, because sourcing executes every
line as shell: a `CHAT_BACKENDS` value contains pipes and was parsed as a
pipeline, and a token containing `$(...)` would have been run. Quote any value
containing `|`, `$` or spaces anyway — anything else that reads this file may
not be as careful.

Copy `.env.example` to `.env` in this directory and set at least:

- `DISCORD_TOKEN` — your bot token (required)
- `ALIAS` — container name and image tag (e.g. `server-domme`)
- `GIT` / `GIT_URL` — set `GIT=true` to clone the repo into `./src`; set `GIT=false` to use an existing `./src` directory

Other variables (e.g. `STORAGE_PATH`, `INIT_SLASH_COMMANDS`, `DEVELOPER_ID`, `DISCORD_GUILD_BLACKLIST`, `WS_SILENCE_TIMEOUT`, `DISCORD_UNHEALTHY_MODE`, `DISCORD_UNHEALTHY_GRACE`, `DISCORD_UNHEALTHY_WINDOW`, `COMMAND_TIMEOUT`, `COMMAND_PARALLELISM`) are optional and match the main app config.

`docker-compose.yml` passes an explicit list of variables to the container, so
a setting that is not named there cannot be set from `.env` at all — it will
silently use its default. `TestEveryConfigVarIsPassedThroughDockerCompose`
keeps that list in step with the app config; if you add a setting, add it to
the compose file too.

Notes on recovery modes:

- `DISCORD_UNHEALTHY_MODE=restart-session` restarts the Discord gateway session when watchdogs or API probes mark it unhealthy.
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

Data is persisted in `./data` (mounted at `/usr/project/data` in the container).

## The chat persona

Optional and off by default. To turn it on, set `CHAT_ENABLED=true` and make
sure `./data/character.md` exists next to this file — it is read from the
mounted volume, not baked into the image, so it can be edited and the container
restarted without a rebuild.

A missing character file is not fatal: the bot logs `chat_character_load_failed`
and starts without the persona, which looks exactly like the feature being off.
If `/chat` answers "No chat backend is configured on this bot" while
`CHAT_ENABLED=true`, that log line is the first thing to look for — the other
cause is `chat_backend_build_failed`, meaning no relay could be reached at
startup.

Enabling it is only the first of two gates — she stays silent until an
administrator runs `/chat here` in a specific channel. That second gate matters,
because replies are produced by third-party relay services, so every message in
an opted-in channel is sent to them.

### Running gpt4free alongside the bot

The hosted g4f relay grants its free allowance as proof-of-work credited to the
IP that earned it, so a server has none and every call returns 402. The same
project self-hosted has no such accounting: it talks to the upstream providers
directly.

```bash
docker compose --profile g4f up -d
```

Then point the bot at it — note **port 8080**, not the 1337 in the g4f docs,
which is a host-side mapping that does not apply inside the compose network:

```
CHAT_BACKENDS=g4f|http://g4f:8080/v1|gpt-4o-mini
```

The container publishes no ports and carries no traefik labels, and that is not
an oversight. It speaks the OpenAI API with no authentication of its own, so
anything able to reach it can spend whatever quota the providers behind it
allow; people scan for exactly this. Keep it on the internal `chat` network.

What this does **not** solve is whether those providers answer from your host.
They are the same sites that challenge datacenter addresses — self-hosting
moves the request out of the relay's credit system, not out of Cloudflare's
view. Some providers will work and some will not, and which is which can only
be learned on the machine itself:

```bash
docker compose exec app wget -qO- http://g4f:8080/v1/models | head -c 400
```

`/chat status` quotes the last error from any backend that has never succeeded,
which is the faster way to see what a provider is actually saying. Keep a second
entry in `CHAT_BACKENDS` so she has somewhere to fall back to.

Cookies and routing config live in `./data/g4f/har_and_cookies`, owned by uid
1000 because that is who the container runs as. `build-n-deploy.sh` creates it.
A `config.yaml` there defines named models with provider fallback — see
[config-yaml-routing.md](https://github.com/xtekky/gpt4free/blob/main/docs/config-yaml-routing.md).