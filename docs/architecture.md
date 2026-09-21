# Architecture

Server Domme is a Discord bot for server management: scheduled channel purges,
roleplay tasks, anonymous confessions, announcements, short links,
reaction-triggered translation, and an optional conversational persona.

It shares its Discord plumbing with [melodix](https://github.com/keshon/melodix)
— the `internal/discord` tree, the command adapter, the middleware chain and the
storage layer are deliberately the same shape in both, so a fix in one can be
lifted into the other. That now includes the vendored `discordgo` fork under
`pkg/discordgo-fork-dev`, which both bots carry byte-identical. What melodix has
and this bot does not is the playback engine; there is no voice code here.

## Lifecycle

`main` owns everything long-lived. Nothing durable is started from a gateway
event, because `onReady` fires again on every reconnect: anything launched there
is either duplicated or outlives the shutdown signal.

```
main
 ├── runSessionLoop      RunSession, reconnecting until rootCtx ends
 ├── RunCooldownCleaner  sweeps elapsed task cooldowns
 ├── purge.RunScheduler  waits for bot.Ready(), then replays stored purge jobs
 ├── shortlink.RunServer HTTP redirects + health endpoint
 └── chat.Run            persona workers + deferred-reply retries (optional)
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

Both watchdogs read the heartbeat ACK, and that read is the one place this
design has already failed in production. `discordgo` holds the session write
lock across gateway reads that carry no deadline, so a wedged session parks
every reader — including both watchdogs, which is how one session ran 22 hours
with a dead gateway and nothing in the log but the datastore compaction ticker.
`lastHeartbeatAck` therefore reads with a timeout and reports the give-up as
its own unhealthy signal (`session_lock_wedged`), and `closeSession` abandons a
session whose close will not return, so the restart loop is never stranded on
the same lock.

Note what is *not* used: `discordgo.Session.HeartbeatLatency()`. It is race-free
in the vendored fork, but it reports the last *completed* exchange, so on a dead
connection it goes stale and then negative rather than growing — the wrong shape
for a staleness check.

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
| `MessageObserver` | every message in a guild, not only mentions |

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
  failed half way can simply be run again to finish the other half.

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
command name (`cmdadapter.ModalSubmitHandler`). A submission arrives without
the command's permission check, which only ran when the modal was opened, so
the handler checks the submitter itself.

## The chat persona

Off unless `CHAT_ENABLED` is set, and then still silent until an admin runs
`/chat channel` in a specific channel. Two gates rather than one, because turning
it on sends the contents of those channels to a model provider — a different
privacy posture from the rest of this bot, and not one to acquire by default.

The design, and why v1 was retired, is in [persona.md](persona.md). In short:
the model is the mind and the code is the body. Each moment is two calls — a
private appraisal returned as JSON (what she makes of it, what she wants to do,
what she takes away) and her voice — and what she knows lives as Markdown under
`CHAT_MEMORY_PATH`, which she writes as she goes and rewrites when she reflects
at night.

| Package | Knows about | Holds |
|---|---|---|
| `internal/mind` | `ai`, `memory` | the character, prompts, appraisal, voice, initiative, reflection |
| `internal/memory` | the filesystem | self, dossiers, days, intentions — as Markdown |
| `internal/chat` | Discord, storage, `mind` | the running service: workers, deferrals, the rails |
| `internal/ai` | HTTP | OpenAI-compatible clients and the failover pool |

`mind` knows nothing about Discord, so every decision and every prompt is
testable without a gateway, and `cmd/chatprobe` can replay a conversation
copied out of Discord through her.

### What stayed from v1

The Discord plumbing did, because none of it was the problem:

- **`Observe` never calls a backend.** It runs on the gateway goroutine, where a
  call of most of a minute would outlive `COMMAND_TIMEOUT`. It records, classifies
  and queues; workers `main` owns do the rest.
- **How a message reached her** — a mention, a reply to her (detected by both
  `ReferencedMessage` and her own sent message ids, since the first is
  best-effort), her name, or the next line from the person she is talking to
  when nobody else has spoken since — is stated to the appraisal as a fact.
  It no longer carries odds; whether she answers is hers to decide.
- **A burst gets one answer.** She waits until its author stops typing, and a
  line arriving while an answer to them is on its way is part of that answer.
- **Failing to answer is not choosing not to.** An answer no backend would
  produce is held and retried, goes out late as a Discord reply to what it
  answers, and the message's age in the transcript is what makes her
  acknowledge the gap. At most one held per channel, a few attempts, typing
  shown only on the first.
- **Typed, not composed.** `mind.Casual` evens out the final full stop,
  em-dashes and typographer's quotes on the way out, and now and then drops an
  apostrophe.
- **Only the person she answers can be notified** by a mention she writes.
- **Echoes and repeats are caught** before posting; a repeat is retried once
  with what she already said ruled out, including a line that opens the way her
  last two did.

### The rails

The model decides; the code keeps the promises a model cannot be trusted with.
She never ignores the same person's direct approach twice running — a second
silence in a row is overruled into an answer, because from outside it is
indistinguishable from a broken bot. Nothing that is not speech (a control
word, JSON) reaches a channel. Nothing she starts arrives at night, more than
a few times a day, or to someone who has not agreed with `/attention`.

### Commands

`/chat channel mode:off|answers|speaks-first` sets how she behaves in a
channel. `/chat status` shows the backends and how she is in this channel —
her mood, how she has been lately in her own words, how she feels about the
people here and what she means to do. `/chat why` shows what she made of a
particular message and what she decided; `/chat about` shows her file on a
person; `/chat brief` and `/chat role` tell her what the server and its roles
are; `/chat forget` moves her memory of a server aside. `/attention` is a
member's consent to be sought out.

### Backends

The endpoints are donated public infrastructure with no guarantees, and they
behave accordingly: `g4f.space` relays volunteer servers that each allow only
their own model list and have been observed answering with a different model
than the one requested. `ai.Pool` therefore ranks backends on what they have
actually done and puts a failing one in cooldown rather than trusting a
configured order. `ai.PickModels` takes at most one model per donated server,
so the pool is not three entries on one machine.

`CHAT_BASE_URL` points at any other OpenAI-compatible endpoint — a local Ollama
or a paid API — and is tried first when set.

**The free tiers do not survive a move to a server.** g4f.space grants its
anonymous allowance as proof-of-work "cakes" baked in a browser and credited,
in its own words, *to the IP that baked it*; a host nobody browses from has
none, and every call returns 402. Pollinations refuses a prompt of this size
anonymously for the same sort of reason. Both work from a desktop and neither
works from a VPS, which is a difference that will not show up in testing.
`CHAT_G4F_API_KEY` sends an account token instead, which is bound to the
account rather than the address; a real deployment is better off pointing
`CHAT_BASE_URL` at something it controls.

`CHAT_BACKENDS` adds any number of further endpoints as
`name|baseURL|model|key` specs. More independent endpoints is the whole of the
resilience story here, and which ones are worth having goes stale faster than a
release: a list an operator can edit outlives any set compiled in.

What does **not** belong in it is most of what circulates as "free AI provider"
lists. Those are overwhelmingly web UIs rather than APIs — measured, one
evening: Cloudflare challenges on `chat.ai365vip.com` and `heck.ai`, a
client-computed request signature on `free2gpt` (`401 Invalid signature`),
plain 403s behind the redirects from `free.netfly.top` and `freegpt.es`, and no
such endpoint at all on `sur.pollinations.ai`. Reaching them means a
per-site scraper of the kind that breaks weekly, and for the large vendors it
also means automating a service whose terms forbid it. An endpoint qualifies
here only if it answers `POST {base}/chat/completions` with an OpenAI-shaped
body.

gpt4free itself is open source and its slim image serves the same
OpenAI-compatible route, so `docker compose --profile g4f up -d` puts a copy on
the internal network at `http://g4f:8080/v1` with none of the hosted relay's
per-IP credit accounting. It does not escape the providers' own bot detection,
which is a different obstacle in the same place; see
[docker/README.md](../docker/README.md).

### Reaching a model on someone's own machine

`koboldcpp` and `llama.cpp`'s `llama-server` both serve the same
OpenAI-compatible route the relays do, so pointing at one is a `CHAT_BACKENDS`
entry and nothing more. The work is networking, not code: the bot is on a
public host and the model usually is not.

A private network between the two — Tailscale, or plain WireGuard — is the
arrangement that survives contact with a home connection. It needs no inbound
port, no static address, and exposes nothing publicly; a reverse SSH tunnel
does the same job with no new software. Port-forwarding an inference server to
the internet does not belong on this list: these servers authenticate weakly if
at all, and anyone who finds one owns the GPU behind it.

Three settings exist because of this case. `CHAT_TEMPERATURE` sets how freely
every backend samples, sent with each request (`ai.Options.Temperature`);
empty, it is left out, and each backend samples the way it does by default,
which is what the character was tuned against on the relays. A local model's
default can be far more deterministic: KoboldCpp answered the same message
with the same words run after run, and a character who says exactly the same
thing to the same situation reads as a machine however good the line is.
Around 0.8 to 1.0 is a start. It is one setting for the pool rather than one
per kind of call, because nothing measured says her note-taking wants
different sampling from her speech. `CHAT_REQUEST_TIMEOUT` raises the
per-backend deadline, since a model on CPU can spend most of a minute on a
prompt a hosted GPU answers in two seconds, and the whole attempt is allowed
twice that so failover still fits. The compose file sets
`host.docker.internal` so a tunnel terminating on the host is reachable from
inside the container, where "localhost" otherwise means the container itself.

A home machine sleeps, reboots and loses power, which is the ordinary case
rather than the failure case: keep a second entry in `CHAT_BACKENDS` and the
pool fails over to it, then returns to the local one when it comes back.

A 401, 402 or 403 is therefore treated as `ai.ErrBackendRefused`: the backend
gets no second attempt and rests for `refusedCooldown` rather than 90 seconds,
because nothing this process does will change the answer and each attempt
spends a request to hear it again. 429 is deliberately excluded — a backend
that is merely busy should come back quickly.

## Storage

`internal/storage` wraps [`keshon/datastore`](https://github.com/keshon/datastore):
a write-ahead log plus periodic snapshots, in a directory the process locks for
its lifetime. A second process opening the same directory fails with
`datastore.ErrLocked`.

Seven collections, each registered before `Open` so the schema is described in
exactly one place:

| Collection | Key | Indexed by |
|---|---|---|
| `guild_settings` | `<guildID>` | — |
| `command_log` | `<guildID>:<020d id>` | guild |
| `purge_jobs` | `<guildID>:<channelID>` | guild |
| `short_links` | `<shortID>` | guild |
| `tasks` | `<guildID>:<userID>` | guild |
| `task_cooldowns` | `<guildID>:<userID>` | guild |
| `mind_people` | `<guildID>:<userID>` | guild |

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
