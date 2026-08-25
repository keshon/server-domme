# Architecture

Server Domme is a Discord bot for server management: scheduled channel purges,
roleplay tasks, anonymous confessions, announcements, short links and
reaction-triggered translation.

It shares its Discord plumbing with [melodix](https://github.com/keshon/melodix)
— the `internal/discord` tree, the command adapter, the middleware chain and the
storage layer are deliberately the same shape in both, so a fix in one can be
lifted into the other. What melodix has and this bot does not is the playback
engine; there is no voice code here, and no vendored `discordgo` fork either.

## Lifecycle

`main` owns everything long-lived. Nothing durable is started from a gateway
event, because `onReady` fires again on every reconnect: anything launched there
is either duplicated or outlives the shutdown signal.

```
main
 ├── runSessionLoop      RunSession, reconnecting until rootCtx ends
 ├── RunCooldownCleaner  sweeps elapsed task cooldowns
 ├── purge.RunScheduler  waits for bot.Ready(), then replays stored purge jobs
 └── shortlink.RunServer HTTP redirects + health endpoint
```

Every one of these takes `rootCtx`, which `signal.NotifyContext` cancels on
SIGINT/SIGTERM, and every one is in the `sync.WaitGroup` that `main` waits on
before closing the store.

`RunSession` builds a **fresh** `*discordgo.Session` on each call. Anything that
outlives one session must therefore resolve the session per use rather than
capture the pointer — `Bot.Session()` exists for exactly this, and
`purge.SessionFunc` is how the scheduler consumes it. A captured session goes
stale on the first reconnect and its writes then target a closed connection.

`Bot.Ready()` closes once, on the first successful connect. It is the signal for
services that need a live gateway before their first action.

## Health and restarts

Two watchdogs decide a session is unhealthy, and both funnel into the same
notifier:

- **`watchdog.WSSilence`** — trips when dispatch traffic *and* heartbeat ACKs
  have both been stale past `WS_SILENCE_TIMEOUT`. Requiring both matters: a
  quiet guild legitimately sends no events for minutes, and the heartbeat is
  what separates "nothing to say" from "nobody home".
- **API probe** — calls `User("@me")` on a timer and trips after three
  consecutive failures.

`DISCORD_UNHEALTHY_GRACE` lets the first N signals inside
`DISCORD_UNHEALTHY_WINDOW` pass before a restart actually happens.

Note what is *not* used: `discordgo.Session.HeartbeatLatency()`. Upstream reads
`LastHeartbeatAck` and `LastHeartbeatSent` together while different locks guard
them, so it is a data race. `lastHeartbeatAck` in `session_health.go` reads the
ACK alone, under the lock that covers it. (melodix carries a forked discordgo
where this is fixed; this bot uses upstream and avoids the accessor instead.)

## Commands

A command is a struct implementing `cmdadapter.Handler` — `Name`, `Description`,
`Run`, plus the `Meta` classification (`Group`, `Category`, `UserPermissions`).
It opts into surfaces by implementing more interfaces:

| Interface | Gives the command |
|---|---|
| `SlashProvider` | a `/slash` definition |
| `ContextMenuProvider` | a right-click context entry |
| `ReactionProvider` | reaction-triggered dispatch |
| `ComponentInteractionHandler` | button and select handling |

`cmdadapter.Register` wraps the handler and puts it in `command.DefaultRegistry`.
Dispatch reads that registry: `handlers_interactions.go` for slash and component
interactions, `handlers_messages.go` for mentions and reactions.

Component custom IDs are matched by prefix against command names — `"name"`,
`"name:..."` or `"name_..."`. A chooser already posted to a channel keeps sitting
there, so its ids come back long after a restart: **custom ID formats are
effectively frozen once shipped.** Add new ids rather than repointing old ones,
and let an unrecognised one fail closed.

Every command runs under `execguard`, which timeboxes it at `COMMAND_TIMEOUT`
and caps concurrency at `COMMAND_PARALLELISM`.

### Middleware

Applied in order, outermost first:

1. `WithGroupAccessCheck` — refuses commands whose group the guild disabled.
   `/settings <feature>` resolves to that *feature's* group, so disabling a
   group disables its settings subtree too.
2. `WithGuildOnly` — no DMs.
3. `WithUserPermissionCheck` — enforces `UserPermissions()`.
4. `WithCommandLogger` — records the invocation, logging the full subcommand
   path (`settings commands disable`), not just the root name.

## Storage

`internal/storage` wraps [`keshon/datastore`](https://github.com/keshon/datastore):
a write-ahead log plus periodic snapshots, in a directory the process locks for
its lifetime. A second process opening the same directory fails with
`datastore.ErrLocked`.

Six collections, each registered before `Open` so the schema is described in
exactly one place:

| Collection | Key | Indexed by |
|---|---|---|
| `guild_settings` | `<guildID>` | — |
| `command_log` | `<guildID>:<020d id>` | guild |
| `purge_jobs` | `<guildID>:<channelID>` | guild |
| `short_links` | `<shortID>` | guild |
| `tasks` | `<guildID>:<userID>` | guild |
| `task_cooldowns` | `<guildID>:<userID>` | guild |

Two key shapes, for two reasons. Append-only rows zero-pad their id so
lexicographic key order equals chronological order — that is what lets an index
read return history oldest-first without sorting, and what makes the
command-log trim keep the newest 50. Everything else keys on the id the guild
already has for the thing.

`short_links` is the exception that keys **without** a guild prefix: the redirect
server resolves an incoming path with no guild in hand. Short ids are therefore
global, and `AddShortLink` refuses a collision rather than silently repointing
someone else's link.

Reads return freshly decoded copies, so mutating a result and putting it back is
safe. Writes that depend on what they just read take a transaction —
`SetCommand` (append plus trim) and `IncrementClicks` (concurrent redirects)
both do.

### Migration from the pre-v1 store

The old store was a single `datastore.json` holding one blob per guild, where
every write rewrote the whole guild. `cmd/migrate-store` converts it:

```bash
go run ./cmd/migrate-store -in ./data/datastore.json -out ./data/store
```

Run it with the bot stopped, then re-run with `-verify` to check the result
against the source. It never modifies the input, and refuses a non-empty output
directory rather than merging into one.

Records are written through `Storage.ImportGuild`, not the ordinary setters:
those stamp their own values — `SetCommand` takes `time.Now()`, `AddShortLink`
starts a link at zero clicks — which is right for a live invocation and would
silently rewrite every migrated record's history to the moment the migration
ran.

## Testing

`go test -race ./...` is the bar. The storage layer has the most coverage
because it is where the interesting invariants live — history trimming, guild
isolation, cooldown expiry, short-link ownership.

See [conventions.md](conventions.md) for the house rules.
