# Running

## Prerequisites

Go 1.26 for building. Nothing else — no CGO, no external binaries, no system
libraries. The release archives ship a static binary.

## Configuration

Configuration is environment-only, read at startup via
[`caarlos0/env`](https://github.com/caarlos0/env); a `.env` file in the working
directory is loaded if present. Copy `.env.example` and fill it in.

Two variables are required and the bot refuses to start without them:

| Variable | What it is |
|---|---|
| `DISCORD_TOKEN` | bot token |
| `TASKS_PATH` | path to the task list JSON (`./data/default_task.list.json` ships with the release) |

The rest have working defaults. The ones worth knowing:

| Variable | Default | Notes |
|---|---|---|
| `STORAGE_PATH` | `./data/store` | **A directory, not a file.** The datastore owns it and locks it. |
| `INIT_SLASH_COMMANDS` | `false` | Register slash commands on connect. Turn on for a first run or after changing a command definition. |
| `DEVELOPER_ID` | — | Your Discord user id; unlocks developer-only actions. |
| `DISCORD_GUILD_BLACKLIST` | — | Comma-separated guild ids the bot leaves on sight. |
| `PROTECTED_USERS` | — | Comma-separated user ids that `/discipline` and `/task` refuse to target. |
| `COMMAND_TIMEOUT` | `30s` | Hard timebox for one command. |
| `COMMAND_PARALLELISM` | `16` | Concurrent command cap. |
| `WS_SILENCE_TIMEOUT` | `2m` | Gateway staleness before the session is called unhealthy. |
| `DISCORD_UNHEALTHY_MODE` | `restart-session` | Or `ignore` to only log. |
| `SHORTLINK_BASE_URL` | — | Public origin the short links are built from — no trailing slash. |
| `SHORTLINK_ADDR` | `:8787` | Listen address for the redirect server. |
| `HEALTHCHECK_PATH` | `/ping` | Shallow GET/HEAD endpoint for uptime monitors; empty disables it. |
| `LOG_LEVEL` | `info` | |
| `LOG_FILE` | — | Empty means stderr only, pretty-printed. Set it for rotated JSON. |

Only one process may hold `STORAGE_PATH` at a time. A second one fails at
startup with `datastore.ErrLocked` — that is the lock doing its job, not a bug.

## Migrating from the pre-v1 store

Older builds kept everything in a single `data/datastore.json`. The current
build wants a directory. Convert once, with the bot stopped:

```bash
go run ./cmd/migrate-store -in ./data/datastore.json -out ./data/store
```

It never modifies the input, and refuses to write into an existing output
directory — so if the result looks wrong, delete the directory and run it again.
Keep the old JSON until you have seen the bot come up clean against the new
store.

Then check the result against the file it came from:

```bash
go run ./cmd/migrate-store -verify -in ./data/datastore.json -out ./data/store
```

Verification reopens the store and compares it through the same read paths the
bot uses — settings, per-guild record counts, the newest command in each log,
and every short link's target and click count. It exits non-zero on any
mismatch.

Two things change on the way through, both deliberate:

- **The command log is trimmed to the newest 50 per guild**, the same cap the
  running bot applies. Older stores commonly sit at 51.
- **A record whose guild id is empty is skipped**, and the tool says so. Those
  come from invocations that carried no guild at all; they cannot be stored,
  because the datastore rejects an empty key and rows with an empty guild term
  are left out of the by-guild index, so anything written under one would be
  unreadable.

## Running locally

```bash
go run ./cmd/discord
```

On Windows, `build-n-run.bat` builds with version metadata baked in and then
runs the result.

Regenerate `README.md` from the command registry after adding or changing a
command — it is generated from `README.md.tmpl`, never hand-edited:

```bash
go run ./cmd/discord -readme
```

## Tests and lint

```bash
go test -race ./...
```

`test.bat` does the same on Windows. Linting uses the curated set in
`.golangci.yml`:

```bash
golangci-lint run ./...
```

Both should come back clean; see [conventions.md](conventions.md#formatting--ci).

## Docker

`docker/` holds a multi-stage build, a compose file with Traefik labels, and its
own `.env.example`. Mount `./data` so the store and the task list survive a
container replacement:

```bash
cd docker && docker compose up -d --build
```

The compose file publishes the shortlink port and routes it through Traefik at
`HOST`. Point `SHORTLINK_BASE_URL` at that same hostname, or the links the bot
hands out will not resolve.

## Shutting down

SIGINT or SIGTERM cancels the root context. The session loop, cooldown cleaner,
purge scheduler and redirect server all stop on it, and the store is closed —
which compacts the log and releases the directory lock — before the process
exits. Killing the process outright is survivable (the write-ahead log replays
on next open) but skips compaction.
