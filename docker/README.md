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

Copy `.env.example` to `.env` in this directory and set at least:

- `DISCORD_TOKEN` — your bot token (required)
- `ALIAS` — container name and image tag (e.g. `server-domme`)
- `GIT` / `GIT_URL` — set `GIT=true` to clone the repo into `./src`; set `GIT=false` to use an existing `./src` directory

Other variables (e.g. `STORAGE_PATH`, `INIT_SLASH_COMMANDS`, `DEVELOPER_ID`, `DISCORD_GUILD_BLACKLIST`, `WS_SILENCE_TIMEOUT`, `DISCORD_UNHEALTHY_MODE`, `DISCORD_UNHEALTHY_GRACE`, `DISCORD_UNHEALTHY_WINDOW`, `COMMAND_TIMEOUT`, `COMMAND_PARALLELISM`) are optional and match the main app config.

### Media storage (rclone)

Media files are stored through **rclone Remote Control** using a **crypt remote** (filenames and contents encrypted on the backend).

1. Copy `rclone.conf.example` to `rclone.conf` in this directory.
2. Generate an obscured password: `rclone obscure "your-strong-password"`.
3. Create the plain media directory: `mkdir -p data/media-plain`.
4. For local development, run rclone RC alongside the bot:

   ```bash
   rclone rcd --rc-addr :5572 --config docker/rclone.conf
   ```

5. Set in `.env`: `MEDIA_RCLONE_RC_URL=http://127.0.0.1:5572` and `MEDIA_RCLONE_REMOTE=crypt-media`.

Docker Compose starts an `rclone` sidecar automatically. The app connects to it at `http://rclone:5572` on the Compose network.

For cloud backends (S3, Google Drive, etc.), replace the `media-plain` remote in `rclone.conf` with your provider; keep the `crypt-media` remote name so the bot config stays the same.

**Production:** enable RC auth (`--rc-user` / `--rc-pass` on rclone, `MEDIA_RCLONE_USER` / `MEDIA_RCLONE_PASS` on the bot). Do not commit `rclone.conf` with real passwords.

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