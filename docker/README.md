# Docker deployment

The public Docker Hub overview is maintained in [DOCKERHUB.md](DOCKERHUB.md).
The `Docker Hub overview` workflow syncs it on changes to that file or the
workflow on `main`, using `DOCKERHUB_TOKEN`. It can also run manually from
`main`. The short description is configured in that workflow. Description
updates require a token with Read, Write & Delete repository permission;
Docker Hub groups repository management under that permission. These syncs
do not build images.

Server Domme supports two image sources: a published Docker Hub release and a local
build of this checkout. Both use the same configuration, Compose project and
persistent storage. Docker Engine with Compose 2.24+ is required; source builds
also need Git. Create and invite a Discord bot using the
[running guide](../docs/running.md).

## Run a published image

Download `compose.yaml` and `.env.example` from this directory into a deployment
folder, or use this directory in a checkout. Run the following commands from
that folder:

```sh
cp .env.example .env
# Edit .env and set DISCORD_TOKEN.
docker compose up -d
docker compose logs --tail=100 app
```

The image is [`keshon/server-domme`](https://hub.docker.com/r/keshon/server-domme). Public
images can be pulled without a Docker Hub login. No Go compiler, Git checkout,
external Docker network or reverse proxy is needed for the base deployment.
The example enables initial slash-command registration.

`IMAGE_TAG=latest` selects the most recently published stable release.
`IMAGE_TAG=edge` selects a build from `main`. Use an exact Git release tag, such
as `v2026.11.10.1`, after that tag has a container build, to select a particular
release. Before the first container release, use `edge` after its workflow
finishes. Older binary releases do not automatically have container images.

## Build this checkout

Clone the repository and select the intended revision. From its `docker/`
directory:

```sh
cp .env.example .env
# Edit .env and set DISCORD_TOKEN.
docker compose -f compose.yaml -f compose.build.yaml build
docker compose -f compose.yaml -f compose.build.yaml up -d --no-build
```

The Dockerfile builds from the repository root. There is no nested `src/`
checkout and no deployment script that deletes source. Local builds use
`server-domme-local:dev`; published images keep their separate Docker Hub tags.
Build failure leaves the running container in place. Use the same `-f` arguments
for subsequent source-build operations; plain `docker compose up` selects the
published-image configuration again.

The images run as UID/GID `10001:10001`. Both Linux amd64 and arm64 images are
built and smoke-tested on native runners. Local builds target the builder's
native architecture.

## Configuration and storage

Compose reads `.env` and passes its settings through `env_file`. The
[configuration reference](../docs/running.md#configuration) remains the full
application reference; copy individual settings from it when needed. Do not
source `.env` as shell code. Values containing `$` can be single-quoted to keep
them literal. The application supplies defaults for settings omitted from the
small Docker example.

The default project name is `server-domme`. Set `COMPOSE_PROJECT_NAME` when running
another independent installation. Keep it unchanged when switching between
source and published images so the same named volumes are reused.

| Container path | Contents | Default storage |
| --- | --- | --- |
| `/usr/project/data` | Database, session recovery state and uploaded task lists | Project's `data` named volume |
| `/usr/project/assets/media` | Uploaded media | Project's `media` named volume |

The default task list is packaged at
`/usr/project/defaults/default_task.list.json`, outside writable volumes.
`TASKS_PATH` can select a custom list under `/usr/project/data`; uploaded
per-server lists stay under that writable directory. Static embed images remain
part of the image.

For host-visible storage, set `DATA_DIR=./data` in `.env`. Relative paths are
resolved from `compose.yaml`; absolute paths are also supported. The existing
folders must be writable by UID/GID 10001. For a new Linux bind directory:

```sh
sudo install -d -o 10001 -g 10001 -m 0750 ./data
```

`MEDIA_DIR=./media` similarly selects host storage for uploads. Prepare its
ownership in the same way. A media bind mount should contain the old
`assets/media` contents directly, including guild subdirectories.

Named volumes need no host-directory ownership setup on a fresh installation.
Back up the whole data volume with the bot stopped, along with `.env` and the media volume.
Do not use `docker compose down -v` when retaining application state. An image
rollback does not automatically reverse a database or configuration migration.

## Update, verify and roll back

For a published deployment, select the intended `IMAGE_TAG` in `.env`, then:

```sh
docker compose pull
docker compose up -d
docker compose logs --tail=100 app
```

Pulling happens before replacement. For source deployment, select the intended
Git revision and repeat the explicit build and `up --no-build` commands above.
There is a restart during replacement; neither flow promises zero downtime.
No old image is forcibly removed and no unrelated Docker images are pruned.

Check Discord command registration and persisted settings, task-list access and any configured public short links. A running
container alone does not establish Discord readiness. The image smoke checks
exercise offline packaging and persistence, without using a bot token.

To return to a published image after a local build, use only `compose.yaml`,
select the previous release tag, pull it and recreate the service. Keep a known
working Git revision for local builds; the `-local:dev` tag is replaced by the
next build. Check data compatibility before downgrading and restore a compatible
data backup when required.

## Migrate the old deployment

Before replacing old deployment files, stop its original Compose stack using
those files and back up the host `data/` folder and uploaded `assets/media` from the old container. The new project name does not
adopt the old containers automatically. Keep the old image or source revision
for recovery.

Set `DATA_DIR=./data` to reuse the old host directory, and grant UID/GID 10001
write access to the existing tree. Review its contents and ownership first;
do not apply ownership changes to a shared or unrelated directory. Start only
one bot process against a datastore directory.

Copy uploaded media out of the old container before removing it, place its
contents in a host `media/` directory, and set `MEDIA_DIR=./media`. Preserve any
edited default task list: set `TASKS_PATH=/usr/project/data/default_task.list.json`
to continue using it rather than the image's packaged default.

The old `ALIAS`, `GIT` and `GIT_URL` deployment settings are no longer used.
The `build-n-deploy.sh` script and `docker-compose.yml` were replaced by the
explicit Compose files described above. Keep the old stack stopped until the
new configuration and stored data have been verified.

## Optional Traefik routing

Short links need a public origin; other bot features do not require an inbound
port. Without `SHORTLINK_BASE_URL`, shortlink commands explain that the service
is unavailable. The base Compose file publishes no ports.

For an existing Traefik installation, add to `.env`:

```dotenv
HOST=links.example.com
SHORTLINK_BASE_URL=https://links.example.com
TRAEFIK_NETWORK=proxy
TRAEFIK_HTTP_ENTRYPOINT=http
TRAEFIK_HTTPS_ENTRYPOINT=https
TRAEFIK_CERT_RESOLVER=letsencrypt
```

Use your actual external network, entrypoints and certificate resolver. They
must already exist in that Traefik deployment. Then run:

```sh
docker compose -f compose.yaml -f compose.traefik.yaml up -d
```

Add `-f compose.build.yaml` before the Traefik file when using a source build.
The overlay sets the service port to 8787 and defines its HTTP redirect
middleware; it does not depend on an undefined headers middleware. It attaches
to the selected external network without publishing a host port.

For another reverse proxy, provide your own override with an appropriate
network or a loopback-only port mapping. `/ping` is a shallow HTTP liveness
endpoint, not a Discord readiness check.

## Image publishing and checks

The `Container` workflow tests Go code and renders the Compose configurations,
then builds native amd64 and arm64 images. Each image is checked offline for
its packaged assets, non-root execution and persistent storage and default task list. Only after
both architectures pass does CI publish the public multi-platform tags.

The Docker Hub account is `keshon`. Add a Docker Hub personal access token with
Read, Write & Delete permission as the GitHub repository Actions secret
`DOCKERHUB_TOKEN` to support both image publishing and overview sync. Image
publishing alone needs only Read & Write. Do not put the token in
`.env`, source files or workflow YAML. Fork pull requests build and check images
without publishing or using that secret.

Pushes to `main` publish `edge`; `v*` tags publish the exact tag, and tags without
a prerelease suffix also update `latest`. Every published build has a full
`sha-<commit>` tag. Architecture-specific tags are used while assembling the
manifest. Workflow dispatch builds and publishes the selected revision without
changing `latest` unless it is a stable release tag. Rebuilds can update OS
packages, so use a registry digest when an exact image must be retained.

Local verification from the repository root:

```sh
bash docker/check-compose.sh
docker build -f docker/Dockerfile -t server-domme:check .
bash docker/check-image.sh server-domme:check
```

The checks do not connect to Discord or certify playback, command permissions
or compatibility with an existing datastore. Verify those after deployment.
