# Running Server Domme

Run from the release directory or the repository root. Relative configuration,
data and asset paths are resolved from the working directory.

## Requirements and first setup

Use a release archive for your OS/architecture, or Go 1.26+ to build. The bot
does not require CGO or external executables. Keep the supplied `assets/` and
`data/` directories beside the binary; `TASKS_PATH` must point to a readable
JSON task list. The release includes `data/default_task.list.json`.

1. Create an application and bot in the
   [Discord Developer Portal](https://discord.com/developers/applications).
   Keep the bot token private.
2. Enable the privileged **Server Members** intent. The bot uses member roles
   for permission checks; Presence and Message Content are not requested.
3. Use the generated [invitation link](../README.md#bot-permissions), replacing
   the application ID. Check the bot's channel permissions as well as guild
   permissions, particularly before configuring purges or welcomes.
4. Copy `.env.example` to `.env`, replace `DISCORD_TOKEN`, check `TASKS_PATH`,
   and set `INIT_SLASH_COMMANDS=true` for first-run command registration.
5. Build and start from the repository root:

```bash
go build -o server-domme-discord ./cmd/discord
./server-domme-discord
```

A downloaded binary can be started directly. On Windows, use the `.exe`
filename; `build-n-run.bat` builds the source with version metadata and runs it.

## Configuration

Environment variables are read at startup. A `.env` file in the working
directory is loaded if present; existing process environment values take
precedence. Copy the [example](../.env.example) rather than starting with an
empty configuration. Restart the process after changing these values.

`DISCORD_TOKEN` and `TASKS_PATH` are required. Defaults below come from
`internal/config/config.go`; example files may explicitly set other values.

| Variable | Default | Meaning |
|---|---|---|
| `DISCORD_TOKEN` | (none) | Required bot token. |
| `DISCORD_SESSION_BUDGET` | `1000` | Fresh-session allowance per rolling 24 hours (20-1000). |
| `TASKS_PATH` | (none) | Required readable task-list JSON path. |
| `STORAGE_PATH` | `./data/store` | Datastore directory, locked to one process. |
| `INIT_SLASH_COMMANDS` | `false` | Register commands on connect; enable for first run or changed definitions. |
| `DEVELOPER_ID` | (none) | User ID that bypasses shared command permission checks across guilds. Treat as privileged configuration. |
| `DISCORD_GUILD_BLACKLIST` | (none) | Comma-separated guild IDs the bot leaves. |
| `PROTECTED_USERS` | (none) | Comma-separated user IDs that discipline/task actions refuse to target. |
| `COMMAND_PARALLELISM` | `16` | Global concurrency cap; each guild's command lane is serial. |
| `WS_SILENCE_TIMEOUT` | `2m` | Gateway-event and heartbeat staleness threshold. |
| `DISCORD_UNHEALTHY_MODE` | `restart-session` | Restart unhealthy sessions, or `ignore` to leave them running. |
| `DISCORD_UNHEALTHY_GRACE` | `0` | Unhealthy signals allowed within the counting window. |
| `DISCORD_UNHEALTHY_WINDOW` | `1m` | Window for the grace count. |
| `SHORTLINK_BASE_URL` | (none) | Public origin for generated links; omit the trailing slash. |
| `SHORTLINK_ADDR` | `:8787` | Redirect server listen address. |
| `HEALTHCHECK_PATH` | `/ping` | Shallow GET/HEAD health path; empty disables it. |
| `LOG_LEVEL` | `info` | Diagnostic logging level. |
| `LOG_FILE` | (none) | Empty uses console stderr; a path enables rotated JSON logs. |
| `LOG_MAX_SIZE_MB` | `10` | Rotated log-file size threshold. |
| `LOG_MAX_BACKUPS` | `3` | Rotated log-file retention count. |
| `LOG_MAX_AGE_DAYS` | `0` | Age limit; zero disables age-based deletion. |
| `LOG_COMPRESS` | `false` | Compress rotated logs. |

Keep `.env`, exports and diagnostic logs private. Command audit records are
separate from process logs; see [Architecture](architecture.md#storage).

## Verify and configure features

After startup, run `/help` in a test guild. Check a harmless command such as
`/roll`, then `/settings commands status` with an authorized account. Confirm
the configured groups and permissions before enabling destructive operations.
A successful HTTP health response does not prove Discord command dispatch works.

`go run ./cmd/discord -check` uses the real token to connect, reports what the
gateway sees and exits without registering commands or sending messages. It is
a live connection check, not an offline test. Review its output for sensitive
deployment information before sharing it.

Single-action feature settings are under `/settings`: `announce`, `confess`,
`discipline`, `media`, `task` and `translate`. Management commands carry their
own setup actions: `/purge`, `/welcome` and `/shortlink`. Use their help and
current generated [command reference](../README.md#available-commands).

`/settings commands` controls group availability. `/welcome access` configures
who may welcome members or preview messages; administrators configure templates
and recipient roles. Caller-access roles and recipient roles are independent.
See [welcome behavior](architecture.md#welcoming-a-member) for delivery and
retry semantics.

### Discord connection budget

Fresh-session attempts share a persistent budget in
`STORAGE_PATH/discord-recovery.json`, including the `--check` connection test.
Keep this file across deployments. The connection check also acquires the
storage lock; stop the running bot before using `-check` with its storage
directory. The lock prevents two processes using the same directory. Run only one deployment per bot token; separate
storage directories or hosts do not share reservations.

Server Domme uses the more exhausted of Discord's reported session allowance and
`DISCORD_SESSION_BUDGET`. Reservations are saved before connecting, so failed
handshakes count conservatively. Successful session resumes do not consume a
reservation. Fresh attempts are spaced by at least five seconds; the Discord
library also applies its reconnect backoff.

At 75% consumption, the minimum delay grows from five seconds to five minutes
at 90%, then to fifteen minutes near 95%. At 95%, fresh sessions wait until
reservations expire or Discord's allowance resets. Five fresh attempts within
ten minutes trigger a fifteen-minute cooldown after the last attempt even
below 75%. An unavailable budget lookup prevents fresh connections until it
can be checked again. Unreadable state or failed persistence stops recovery.

While connected, the bot shows a short presence line such as
"Connection budget low | retries slowed". It clears the line when protection
is no longer needed. Presence cannot be updated while disconnected; logs record
recovery transitions, reservation counts and the next eligible attempt instead.
No channel or message setup is required.

Invalid tokens and non-retryable Gateway errors stop automatic recovery while
the process remains running, including under Docker's restart policy. Correct
the configuration/token and restart deliberately. After a Discord token reset,
replace `DISCORD_TOKEN` and recreate the container with the new environment.

## Persistent data and backups

`STORAGE_PATH` is a directory, not the old JSON file. Only one process can own
it; a second instance fails with `datastore.ErrLocked`. Stop the owning process
before copying the entire directory for backup or restoring it. Preserve the
matching release and configuration separately.

Also preserve operator-edited task lists under `data/` and uploaded media under
`assets/media/`. Treat backups as private server data. Restore into private
staging, review the paths/configuration, and start the matching release only
after replacing the intended state. A restore discards changes newer than the
backup; verify command access and persisted settings before normal use.

## Docker

The [Docker guide](../docker/README.md) owns build/deploy commands, explicit
environment forwarding and mounted paths. Set `SHORTLINK_BASE_URL` to the
public hostname routed to the redirect service. A local shallow health check
only verifies that HTTP endpoint, not gateway health or Discord permissions.

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

## Shutting down

SIGINT or SIGTERM cancels the root context. The session loop, cooldown cleaner,
purge scheduler and redirect server all stop on it, and the store is closed —
which compacts the log and releases the directory lock — before the process
exits. Killing the process outright is survivable (the write-ahead log replays
on next open) but skips compaction.
