# Server Domme

A self-hosted Discord server-management and roleplay bot. Provides scheduled
cleanup, welcomes, announcements, anonymous confessions, roleplay tasks,
translation and media sharing. Short links can optionally use a public HTTPS
endpoint.

[Source code](https://github.com/keshon/server-domme) ·
[Deployment guide](https://github.com/keshon/server-domme/blob/main/docker/README.md) ·
[Bot setup and configuration](https://github.com/keshon/server-domme/blob/main/docs/running.md)

## Quick start

Install Docker Engine with Compose 2.24+ and create and invite a Discord bot
using the setup guide above. Download the deployment files:

```sh
mkdir server-domme && cd server-domme
curl -fsSLO https://raw.githubusercontent.com/keshon/server-domme/main/docker/compose.yaml
curl -fsSL https://raw.githubusercontent.com/keshon/server-domme/main/docker/.env.example -o .env
```

Edit `.env`: set `DISCORD_TOKEN` to your bot token and `IMAGE_TAG=edge`.
Then start the bot:

```sh
docker compose up -d
docker compose logs --tail=100 app
```

Public images do not require a Docker Hub login. Keep your Discord token private.
The base deployment needs no host ports, external Docker network or reverse proxy.
The deployment guide covers optional Traefik routing for short links.

## Tags and platforms

- `edge`: builds from GitHub `main`. Use this until the first container release.
- `latest`: the most recent stable release published by the container workflow.
- `v*`: exact Git release tags built by that workflow.
- `sha-<commit>`: a full commit SHA identifying a published build.

Images support Linux `amd64` and `arm64`. Both architectures are built and
smoke-tested before combined public tags are published. The image runs as
UID/GID `10001:10001`.

## Storage and updates

Compose stores application state in a named volume at `/usr/project/data` and
uploaded media in another at `/usr/project/assets/media`. The packaged default
task list is outside writable storage. Keep the Compose project name unchanged
to reuse those volumes across updates.

See the deployment guide for host directories, backups, migration from the old
deployment, source builds and rollback instructions.

To update, select the intended `IMAGE_TAG` in `.env`, then run:

```sh
docker compose pull
docker compose up -d
docker compose logs --tail=100 app
```

Replacement briefly restarts the bot. Verify its commands and persisted settings
in Discord afterward. Avoid `docker compose down -v` when retaining stored state.
