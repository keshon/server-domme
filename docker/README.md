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

That service sits behind a compose profile, so set it in `.env` before
deploying — `build-n-deploy.sh` exports this, and it has to be set for
`compose down` as well as `up`: a service whose profile is inactive is not
stopped by `down`, it is an *orphan*, and `--remove-orphans` deletes it.

```
COMPOSE_PROFILES=g4f
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

`probe-providers.sh` answers which of them work here:

```bash
./probe-providers.sh          # every provider, one request each
./probe-providers.sh all      # named models too
```

Three things it knows. `/v1/models` returns provider names as well as model
names, flagged `"provider": true`, and a provider name is itself valid as a
`model` — it resolves to that provider's own default. That is the way past a
model name like `gpt-4o-mini` that turns out to be paywalled at whoever serves
it. g4f lists what it *believes* works, which is not what answers from this
host today, so the script asks rather than reads.

And a 200 is not an answer. Of nine providers that returned one on a real
server, four were useless: two image generators and a text-to-speech model that
answer any prompt with a markdown image or an `<audio>` tag, and one that
replied "Sign up and repeat your request." with a perfectly good status code.
Those are reported as `NOT TEXT` and `SUSPECT` and kept out of the generated
line — a pool containing an image generator means the bot occasionally posts a
picture instead of speaking.

Whatever survives that is asked a second time with a prompt the size of a real
character, because answering "say OK" proves almost nothing: pollinations
served a two-word prompt and refused a real one with 402. Only what carries a
full prompt reaches the printed `CHAT_BACKENDS` line.

Expect a lot of `HTTP 500 Request execution failed` in that output, and do not
read it as a fault in the deployment. It is the API's catch-all for any
exception that is not ModelNotFound, ProviderNotFound or MissingAuth — and the
slim image contains no browser at all, while a large share of g4f's providers
need one to clear Cloudflare or to use a logged-in session. `401
MissingAuthError` is the separate, honest "needs credentials" case.

The generic message is only what goes over HTTP; the real traceback is logged.
To see what is actually failing, and how often:

```bash
docker compose logs g4f 2>&1 | grep -hoE "^[A-Za-z][A-Za-z0-9_.]*(Error|Exception)"     | sort | uniq -c | sort -rn | head
```

The full image (`hlohaus789/g4f:latest`, built `FROM selenium/node-chrome`)
does carry a browser, and unlocks some of those providers. It costs 1.4 GB
against slim's 399 MB, wants `shm_size: 2gb`, and runs Chrome continuously —
and it does not help with providers that also want an account, nor reliably
with Cloudflare on a datacentre address. Worth it only after the slim image has
been shown to have nothing usable.

`/chat status` quotes the last error from any backend that has never succeeded,
which is the faster way to see what a provider is actually saying. The startup
log names the pool — `ai_pool_ready backends=4 names=g4f,g4f:openrouter.ai,…` —
which is how to tell a backend that was dropped for a malformed spec from one
that was never configured.

Turn `CHAT_USE_G4F` off once the local one works. Leaving it on keeps three
hosted relay backends in the pool that will answer 402 from a server, and their
errors are what fills the log. Keep a second
entry in `CHAT_BACKENDS` so she has somewhere to fall back to.

The compose file overrides the image's command. Its own default is
`python -m g4f --port 8080`, which the current CLI rejects — `--port` moved
under the `api` subcommand, so the bare `8080` is read as the *mode*, fails,
and the fallback path starts **tray** mode, which dies looking for an X server
with `Xlib.error.DisplayNameError: Bad display name ""`. The traceback names
pystray and Xlib and looks nothing like an argument error, so it is worth
knowing what it really is. Naming the mode explicitly avoids it, and keeps
working if upstream repairs its Dockerfile.

Cookies and routing config live in `./data/g4f/har_and_cookies`, owned by uid
1000 because that is who the container runs as. `build-n-deploy.sh` creates it.
A `config.yaml` there defines named models with provider fallback — see
[config-yaml-routing.md](https://github.com/xtekky/gpt4free/blob/main/docs/config-yaml-routing.md).