# Architecture

Server Domme is a Discord bot for server management: scheduled channel purges,
roleplay tasks, anonymous confessions, announcements, short links,
reaction-triggered translation, and guided member welcomes.

It shares its Discord stack with [melodix](https://github.com/keshon/melodix)
— `internal/discord` (disgo session, adapter, reply, queue, slashsync,
middleware), the command catalog shape and the storage layer are deliberately
the same in both, so a fix in one can be lifted into the other. What melodix
has and this bot does not is the playback engine; there is no voice code here.

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

`RunSession` builds a **fresh** disgo client on each call. Anything that
outlives one session must therefore resolve the connection per use rather than
capture the API — `Bot.SessionAPI()` exists for exactly this, and
`purge.APIFunc` is how the scheduler consumes it. A captured API goes stale on
the first reconnect and its writes then target a closed connection.

`Bot.Ready()` closes once, on the first successful connect. It is the signal for
services that need a live gateway before their first action.

## Health and restarts

One watchdog decides a session is unhealthy: **`watchdog.WSSilence`** trips
when dispatch traffic *and* heartbeat ACKs have both been stale past
`WS_SILENCE_TIMEOUT`. Requiring both matters: a quiet guild legitimately sends
no events for minutes, and the heartbeat is what separates "nothing to say"
from "nobody home".

`DISCORD_UNHEALTHY_GRACE` lets the first N signals inside
`DISCORD_UNHEALTHY_WINDOW` pass before a restart actually happens.

disgo delivers heartbeats as events, so the ACK is recorded on arrival and
read without contending with anything — there is no session lock to wedge and
nothing to time out reading. Teardown is still bounded and abandonable
(`closeWithin`): a step that blocks would strand the restart loop with it, and
the bot a watchdog just correctly declared dead would never come back.

## Commands

A command is a struct implementing `adapter.Handler` — `Name`, `Description`,
`Run`, plus the `Meta` classification (`Group`, `Category`, `UserPermissions`).
It opts into surfaces by implementing more interfaces:

| Interface | Gives the command |
|---|---|
| `SlashProvider` | a `/slash` definition |
| `MenuProvider` | a message context-menu entry under the same name |
| `ReactionHandler` | reaction-triggered dispatch (`/translate`) |
| `ComponentInteractionHandler` | button handling |
| `ModalSubmitHandler` | a modal editor's submission (`/welcome template`) |
| `MessageCommandHandler` | a context-menu invocation (`/announce`) |
| `Unlogged` | exclusion from the audit log (`/confess`) |

`catalog.Register` wraps each handler in `adapter.Adapter` with the middleware
and puts it in `command.DefaultRegistry`. Dispatch reads that registry:
`handlers.go` for slash, menu, component, modal and reaction events.

Component and modal custom IDs are matched by prefix against command names —
`"name"`, `"name:..."` or `"name_..."`. A button already posted to a channel
keeps sitting there, so its ids come back long after a restart: **custom ID
formats are effectively frozen once shipped.** Add new ids rather than
repointing old ones, and let an unrecognised one fail closed.

Command bodies run off the gateway goroutine on per-guild queue lanes, so the
socket stays read while a command works. Waiting for a slot is bounded by the
interaction acknowledgement deadline; running is not. Concurrency is capped at
`COMMAND_PARALLELISM` across every guild.

### User-facing copy

Three voices share one bot, and each command picks one on purpose:

- **Domme/brat roleplay** — `discipline`, `task` and its phrase lists. Teasing,
  pet-names, scolding. This voice never leaves those two commands.
- **Snarky mortal** — `ask` only. Dry, second-person, mildly slangy.
- **Neutral utility** — everything else, including all errors and all of
  `/welcome`'s coaching. No jokes in destructive paths: a purge warning reads
  like a warning.

The mechanical rules, so review catches drift:

- Command and option descriptions: capitalized, statement, no trailing period.
- Option names are snake_case; the person is always `user` — except
  `discipline`, where `target` names the brat rather than the invoker.
- Settings groups come in two shapes: singletons (one channel, one role) use
  `set/show/reset`; collections (many channels, categories) use
  `add/remove/list` plus `clear`.
- Every user sentence ends with a period. Straight apostrophes, never curly.
- Errors carry the raw cause backticked: `Failed to …: \`%v\``.
- Every embed sets `Color` (`reply.EmbedColor` unless there is a reason).
- Ephemeral embeds carry no title. Public posts do: emoji prefix + Title Case.
- Component custom IDs are `<command>:<args...>` with colons, e.g.
  `ask:<asker>:<target>:<type>:accept`. The format is frozen once shipped, so
  new buttons copy it rather than inventing another.

### Middleware

Applied in order, outermost first:

1. `WithGroupAccessCheck` — refuses commands whose group the guild disabled.
   `/settings <feature>` resolves to that *feature's* group, so disabling a
   group disables its settings subtree too.
2. `WithGuildOnly` — no DMs.
3. `WithUserPermissionCheck` — enforces `UserPermissions()`, except for
   `DEVELOPER_ID`, which runs everything on every guild so the maintainer can
   exercise admin commands without holding a role. It is a total bypass of
   this middleware, so the id is a credential; `config.IsDeveloper` fails
   closed when either side is empty, because an unset `DEVELOPER_ID` would
   otherwise match an event carrying no user id.
4. `WithCommandLogger` — records the invocation, logging the full subcommand
   path (`settings commands disable`), not just the root name.

### Welcoming a member

`/welcome member user:@x` posts the introduction and the welcome a server has
written for the role that person has, each in its own channel, tagging them,
with a gif picked at random under the welcome. Everything is per role: a sub
and a Domme are pointed at different channels and told different things, and
any role can be given its own pair.

It is run by an administrator, not triggered by an event, by request. Roles on
these servers are picked after joining, changed, and occasionally given by
mistake; a person deciding "this one is ready" is the guard no event can
replace. The command does the fiddly part and refuses rather than guesses:

- the person has to be in the server, not a bot, and actually have the role
  they are being welcomed as — welcoming a sub as a Domme is the mistake it
  most needs to refuse. With no role given, it uses the one configured role
  they have and asks when they have several;
- each part needs a channel and a text, and a channel the bot can see and
  post in; a text over Discord's 2000 characters once the name is in it is
  refused, not cut;
- only the person being welcomed is notified, whatever the text says — a
  template with a role mention or @everyone in it would otherwise ping a
  whole server for one newcomer;
- each part is recorded when it goes out (`storage.Welcomed`), so running it
  twice does not post twice unless `again:True` is given, and a run that
  failed half way can simply be run again to finish the other half;
- a gif is attached as a file, which needs Attach Files — a permission
  View Channel and Send Messages do not carry and the check before posting
  cannot see refused at the guild level. Discord refusing the upload is not
  the welcome failing: it goes out again with the link, and the reply says
  which permission was missing;
- `part:intro` or `part:welcome` posts one of the two and leaves the other
  alone, for the half that failed on a permission the administrator has
  since fixed. Whatever was found while planning the part left out goes
  unreported with it: posting the welcome alone does not need a warning
  about the intro's channel.

Every check runs before anything is posted, and the reply lists each part with
a link to what went out or the reason it did not.

Texts are written in a modal (`/welcome template`), because they are
paragraphs and a command option is one line. They are pasted straight from
Discord, and copied text carries a channel as "#introduction" rather than the
"<#id>" a message needs, so `Render` turns "#channel-name" back into a link for
any channel that exists, longest name first. A name that matches nothing is
left as written and flagged when the text is saved and in `/welcome preview`.
Placeholders: `{user}` (the tag), `{name}`, `{server}`, `{role}` — the role as
text, never as a mention.

Modal submissions route like components, by a customID starting with the
command name (`adapter.ModalSubmitHandler`). A submission arrives without
the command's permission check, which only ran when the modal was opened, so
the handler checks the submitter itself.

## Storage

`internal/storage` wraps [`keshon/datastore`](https://github.com/keshon/datastore):
a write-ahead log plus periodic snapshots, in a directory the process locks for
its lifetime. A second process opening the same directory fails with
`datastore.ErrLocked`.

Eight collections, each registered before `Open` so the schema is described in
exactly one place:

| Collection | Key | Indexed by |
|---|---|---|
| `guild_settings` | `<guildID>` | — |
| `command_log` | `<guildID>:<020d id>` | guild |
| `purge_jobs` | `<guildID>:<channelID>` | guild |
| `short_links` | `<shortID>` | guild |
| `tasks` | `<guildID>:<userID>` | guild |
| `task_cooldowns` | `<guildID>:<userID>` | guild |
| `welcome_roles` | `<guildID>:<roleID>` | guild |
| `welcomed` | `<guildID>:<userID>:<roleID>` | — |

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
