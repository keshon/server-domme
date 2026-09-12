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

## The chat persona

Off unless `CHAT_ENABLED` is set, and then still silent until an admin runs
`/chat here` in a specific channel. Two gates rather than one, because turning
it on sends the contents of those channels to third-party relays — a different
privacy posture from the rest of this bot, and not one to acquire by default.

Three packages, split by what they need to know:

| Package | Knows about | Holds |
|---|---|---|
| `internal/mind` | nothing but its own types | character, grounding, prompt assembly, the decision to speak |
| `internal/chat` | Discord, storage, `internal/ai` | the running service: workers, deferrals, observation |
| `internal/ai` | HTTP | OpenAI-compatible clients and the failover pool |

`mind` is pure so that every decision about *when* she speaks is testable
without a gateway. The premise it is built on — borrowed from the cognitum
experiment — is that the language model is a speech cortex, not a brain: it is
handed an assembled picture and only puts it into words.

### Speaking is separate from deciding to speak

`Observe` runs on the gateway handler goroutine and does only in-memory work
plus one storage write. It never calls a backend: a free relay can take most of
a minute, `COMMAND_TIMEOUT` is thirty seconds, and a reply generated inline
would either be killed or hold a command slot for the duration. It classifies
the approach, asks `mind.Decide`, and hands the rest to two workers that `main`
owns.

Four things can address her, and she answers each at different odds:

| Trigger | What it is |
|---|---|
| `mention` | a direct `@mention` |
| `reply` | a Discord reply to something she said |
| `named` | her name in a message spoken **to** her, without an `@` |
| `about` | her name in a message spoken **about** her, to the room |
| `follow-up` | the next thing said by whoever she is mid-conversation with |

`named` and `about` both start from a plain string match, not a classifier —
this runs on every message in a watched channel, and a model call there would
cost a request per *message* rather than per *reply*, which is the difference
between affordable and not on free relays.

Telling them apart is `mind.ClassifyAddress`, a word-list heuristic: a name
used as a vocative ("Domme, what do you reckon"), second-person pronouns, or a
question mark read as being spoken to; a third-person verb after the name
("Domme would hate this") or third-person pronouns read as being spoken about.
Ambiguity resolves to `about`, because overhearing is much the commoner case
and the wrong answer there is silence rather than an interruption. It handles
English and Russian, being the two languages this bot is spoken to in.

The split exists because one number could not serve both. Being named while
someone speaks to you is a question in all but punctuation and is answered
readily (0.65); being mentioned in passing is not an invitation at all (0.25).
Merged, she either barged into conversations about her or ignored people
addressing her by name.

`follow-up` is what stops her answering once and then going deaf. People drop
the tag as soon as a conversation is running; re-addressing every line is what
you do with a machine. It fires only when she spoke **last** in the channel,
**recently**, and the message is from the **same person** she was answering.
All three conditions carry weight: once anyone else has spoken the thread is no
longer hers to assume, which is what keeps her out of conversations between two
other members.

A reply is detected two ways and needs both. `ReferencedMessage` carries the
author but discordgo documents it as best-effort — *"the backend did not
attempt to fetch the message that was being replied to"* — and replies arriving
without it were being silently dropped. `MessageReference` is always present
but carries only an id, so her own sent message ids are recorded on the turn
(`mind.Turn.MessageID`) and matched against it.

Inside an open exchange, `EngagedBoost` carries a mention or a reply to
certainty. Deliberate silence is for cold approaches: dropping a direct
question mid-conversation does not read as reticence, it reads as a fault.

She does not always answer. That is the point, and it is also the most
dangerous behaviour here, because from outside a deliberate silence and a
broken bot look identical. Two rails keep them apart, and neither is
probabilistic: she always answers a first approach, and she never ignores the
same person twice running. Every ignored approach logs `chat_approach_ignored`
at info level, which is the only way to tell afterwards which one happened.

### Failing to answer is not the same as choosing not to

They produce the same silence and must never share a path. An ignored approach
is final and is never retried. An approach that no backend would answer becomes
a `mind.Deferred` and is retried until it succeeds or ages out — she answers
late, the way someone who was busy does, rather than posting a machine apology
about backend availability into a conversation.

A late answer goes out as a Discord reply to the message it answers, and the
message's age is stated in the transcript handed to the model, which is what
makes her acknowledge the gap. Instructing her to do so in the system prompt
did not work and neither did a trailing system message; the timestamp did. See
the comment on `mind.labelled` and `cmd/chatprobe`, which is how that was
measured.

At most one approach is held per channel. A queue would deliver a burst of
catch-up chatter the moment a relay recovered, which reads more like a machine
than the silence it is making up for.

Retries are bounded by `mind.MaxDeferralAttempts` as well as by the TTL, and
the typing indicator is shown on the first attempt only. Both exist because of
the same production failure: with every backend refusing, one unanswerable
message came round every `DeferralRetry` for the whole fifteen-minute TTL and
showed a typing indicator each pass, so the bot appeared to be typing, on and
off, for a quarter of an hour. On a retry the indicator is a lie — the last
attempt failed and this one may too — and past a few attempts silence is the
honest outcome.

### What a role is worth to her

`/chat role` sets what a Discord role means to the persona: a regard from -1 to
+1, and optionally a note. Per role rather than per person, because a server
that has roles has already decided who is what, and asking an operator to rate
three hundred members one at a time is asking them not to use it.

The note is the part that earns its place. A scalar can only produce a sentence
derived from a number — "You think well enough of cass" — where an operator
writing about their own server can say "a submissive here, speak to them as
one", which is a thing no number encodes. It goes into the prompt verbatim, and
the bands are the fallback for a role set without one.

Regard sums across the roles someone holds and clamps, so two roles that both
count for something add up while three do not make her more forthcoming than
any one role could, and roles that disagree cancel. It moves their odds by at
most `RegardNudge` — smaller than the mood, about the size of irritation.
Standing should colour how she treats someone, not decide whether they can talk
to her: a member who cannot get an answer because of a role they were given has
no way to tell that from a broken bot.

It sits above irritation in the prompt. Standing is the settled fact about
someone and irritation is the passing one, so the passing one goes later and
wins where they disagree.

### Being annoyed with someone

Irritation is held **per person**, not per guild, because that is the whole
difference between someone annoyed and a bot in a bad mode: being short with
one member and perfectly ordinary with the next is what a person does. The mood
is the guild-wide layer; this sits under it and applies to one name.

It rises on countable behaviour — pressing again inside `PesterWindow` after
being passed over — and not on tone. Deciding whether a message was rude needs
a model call per message, which is not affordable, and the experiment this
design came from showed small models answering that kind of question
confidently and arbitrarily. Pushing is observable in any language with no
interpretation at all.

Stored undecayed with a timestamp and decayed on read, like everything else
here, with a 90-minute half-life: coming back an hour later still finds her
cool, and by the next day it is gone.

It lowers that person's odds of an answer by at most a fifth, and adds one
named directive to their reply. Deliberately small, and the rails still
guarantee a first approach and a second-in-a-row are always answered — a
character who simply stops responding reads as a broken bot rather than an
irritated person, which is the failure this whole layer keeps having to avoid.

When irritation first crosses into mattering, the episode is written to the
ordinary memory store as a deterministic memory — no backend call, because the
bot watched it happen and knows who did what. That is what gives the feeling a
cause: irritation on its own is a number, and asked what is wrong she would
have had nothing to point at. The directive only fires while that person is in
the room, and the memory outlives their leaving it.

Going through `mind.Memory` rather than a field of its own means it decays on
the same curve as everything else, resurfaces when that person returns or the
subject comes up, and needs no special case in recall. Once per episode rather
than once per push, or the store fills with the same sentence.

`/chat state` shows all of it: the drives as bars, what they are nudging, who
she is short with, and both sets of directives verbatim. The bars go inside a
fenced block because Discord renders labels proportionally, so "Energy" and
"Interest" are different widths and the columns after them do not line up
otherwise.

`/chat forget confirm:yes` wipes what she remembers about a guild, and what she
holds against the people in it. Irritation goes with the memories rather than
outliving them: clearing one and not the other leaves her short with someone
for a reason she can no longer name, which is the failure the irritation memory
was added to prevent. Message counts survive, because they are how she knows a
regular from a stranger and wiping them makes everyone in the server new — a
much larger thing than being asked to forget what happened. The confirmation is
a typed word rather than a button, because this cannot be undone and there is
no copy.

### What she remembers

`mind.Memory` is one thing that happened in a channel, written once and never
rewritten. The obvious design — re-summarising a memory shorter as it ages, so
the stored text shrinks — costs a model call per memory per age bracket and
drifts: summarising a summary pulls towards the blandest available reading
every time, which is visible in the cognitum logs as one observation
reappearing in three slightly different wordings. Storing it once and rendering
less of it gives the same gradient for one call, and what comes back is what
went in.

Brightness is computed on read, never stored:

```
brightness = 0.5 ^ (age / halflife)          halflife grows with Weight
           + topicBoost    when its words overlap what is being said now
           + presenceBoost when someone who was there is here again
```

Two different things decay, and both are wanted: how *likely* a memory is to
come back at all, and how *much* of it does. Above 0.6 it renders with its full
detail; above 0.3 the detail is clipped; below that only the gist survives, and
without a timestamp — someone who barely remembers a thing does not know
exactly when it was. Under `brightnessFloor` it is not rendered at all.

`Weight` slows the fade rather than raising the level, which is cognitum §5.6's
"emotional peak decays more slowly". A charged day still registers a fortnight
later while the small talk around it has gone, but it never reads as more
present than something that just happened.

The boosts are cognitum's P6 — "old thoughts can return when a trigger matches
their topic" — with `People` added, so a memory can also surface because of who
is in the room. Matching is keyword-set overlap rather than embeddings, which
would need a model call per memory per message and somewhere to keep the
vectors. It matches words rather than meanings, so a memory about "the purge
rules" will not surface for "channel cleanup": a worse recall than a real one
and a much better one than none.

`chat.Service.rememberLoop` writes them, on its own goroutine under the
service's context and never on the path of a reply. Every five minutes it looks
for channels whose conversation has been quiet for six and runs to at least a
few turns, and asks one backend to summarise it. Everything about it is
best-effort: a failed summary means one conversation goes unremembered, which
nobody can see, where a summary that delayed an answer would be obvious to the
whole channel. Failures are not held or retried — a deferral exists so a person
gets their answer late rather than never, and nobody is waiting on a memory.

Waiting for the pause matters. Summarising a conversation still in progress
produces a memory of half an argument, and then a second memory of the other
half when it finishes.

Whether a conversation has already been remembered is derived from the stored
memories rather than from a marker in memory, so a restart cannot pay for the
same memory twice.

The sweep checks the opt-in itself rather than trusting that `Observe` did.
Summarising sends a channel's contents to a relay, which is precisely what
`/chat here` governs, and the conversation outlives the permission: silencing a
channel leaves its turns in the buffer where the sweep would still find them.
`/chat silence` now also drops what she is holding, because being told to stop
reading a channel has to take the conversation with it and not just the right
to read on.

The summary is asked for as two labelled lines, `GIST:` and `DETAIL:`, not as
JSON. The experiment this design came from asked for JSON and failed to parse
37% of the replies against a local model it controlled; these backends are
weaker. Prefixed lines split on a colon, survive a model's preamble and
markdown, and degrade to "no memory this time" rather than to an error.

The summariser also reports a `TONE` — one of warm, ordinary, tense or hostile.
It is the only judgement in this design a model is asked to make, and it is
affordable because it is asked once per conversation rather than once per
message, and it rides along with a call that was being made anyway. A closed
vocabulary because it is acted on rather than displayed; anything outside it
reads as ordinary and changes nothing, which is the safe direction when the
alternative is a misread word moving a dial nobody can trace.

Tone does two things. It adjusts the memory's weight, because length and
headcount miss the short brutal exchange entirely and that is the kind a person
remembers longest — and a warm conversation is charged too, so it lengthens as
well. And when the tone was unpleasant **and exactly one other person was
there**, it carries into how she feels about them.

That condition is the whole of the attribution problem. A four-way row that
went badly does not say who made it go badly, and a model asked "who was
unpleasant" answers that worse than not asking. With company it moves nothing
and the conversation is merely remembered as a heavy one.

`Weight` and the participants are otherwise computed in Go from the transcript,
not asked of the model. Asking costs another line to parse and "rate the emotional weight
of this conversation" is exactly the question a small model answers confidently
and arbitrarily; length, how many people were drawn in and how much was aimed
at her are observable and about as predictive.

The summariser is not given the character file. This is a note being taken, not
her speaking, and a persona in that prompt produces a memory that is
entertaining and vague rather than one that is useful weeks later.

### How she is doing

`mind.Drives` is three numbers — Social, Energy, Interest — derived on read
from a timestamp and two counts. It is the cognitum experiment's symbolic core
with its price removed: that design kept state in the same way and spent around
eighty model calls an hour doing it, a third of which failed to parse. Nothing
here calls a backend.

Derived rather than ticked. cognitum recomputed on a one-second timer, which
needs a goroutine, loses everything on restart and burns cycles in an empty
server; computing from elapsed time when the value is wanted gives the same
curves for nothing and survives a redeploy, because the timestamps do.

Three drives rather than four: Social, Energy and Interest each have an input
this bot can observe, and cognitum's Coherence had none. A drive fed by nothing
drifts convincingly and means nothing.

Energy follows an anchored day curve rather than a cosine, because a 24-hour
cosine is symmetric — putting the trough at 04:00 necessarily makes 08:00 just
as dark. `CHAT_TIMEZONE` is the community's zone, not the host's: a bot yawning
through someone's prime time is worse than one with no clock at all.

They reach the reply twice. `Drives.Nudge` moves the odds of answering by up to
a fifth — but only for the indirect approaches. A direct mention or a reply is
answered on its own terms whatever the hour: someone tired still answers when
spoken to, and making that conditional is how "she ignored my direct question"
returns with a better excuse. The zero value nudges by nothing, so an unset
mood is not a bad one.

`Drives.Directives` is the other half, and the shape of it is the point. The
first version stated the mood as a fact in the grounding — "Right now: it is
the dead of night and you are running on fumes" — and measured **no effect at
all**: asked the same question at 3am and at 8pm, the exhausted run produced
the longest and liveliest reply of the set while the wide-awake one answered
"which film?". Stating a mood asks the model to infer a writing style from it,
and it does not. This is the third time that has been measured here, after the
late-reply note and the anti-assistant rule.

Phrased as instructions and placed last — after the output rules, where nothing
follows it — the same states produce "haven't seen it" at 3am against full
sentences when awake. At most two directives, and only when the state is
pronounced: a list of qualifications on every reply is how a strong instruction
becomes a weak one.

Above the mood sits `mind.SpeechStyle`, the settled temperament, read from the
character file's `## Temper` section as dials on 0..1. Same machinery, one
layer up and one layer more stable: temperament is what she is like generally,
mood is what she is like today, and the more transient of the two takes the
later and stronger position so it wins when they disagree.

Dials rather than a named reference. "Write like <famous person>" is a large
prior in very few tokens, which is its whole appeal, but it composes badly —
set against a mood directive the model picks whichever prior is stronger
instead of combining them — and it imports everything else about that person,
which here would contradict a character whose first paragraph is that she is
not a performer. A dial is tunable; a name is a coin flip on which version of
them the model has in mind. Only dials set away from the middle produce a line,
so a file that configures two of them costs two lines rather than five.

This is what keeps the character file from being a constant. The persona is
static by design, because its properties were measured and are worth keeping
fixed; what varies per message is the block underneath it.

### Making it clear who she is answering

A reply is sent as a Discord reply when a bare message would leave people
guessing: a held answer, an answer to someone who replied to her, or a channel
where somebody else has spoken since the line she is answering.

Not otherwise, and that restraint is the point. Anchoring every reply is
unambiguous and wrong — in a quiet two-person exchange, formally quoting each
line is exactly the machine tell the rest of this design keeps removing. People
reach for the reply affordance when the thread has moved on and simply talk
when it is obvious who they mean. `mind.NeedsAnchor` is that rule.

A message that has aged out of the buffer counts as needing one: it is gone
because time passed, which is when an unanchored reply lands with nothing
around it to explain what it answers.

She does not @mention the person either. Her allowed-mentions are empty so it
would not ping, and an inline mention reads like a bot addressing a ticket
where the reply anchor shows the same thing without touching what she says.

### How much of the conversation counts as live

`Conversations.Recent` bounds the live context two ways and takes whichever
gives more: everything within `TurnStaleAfter` of the newest turn, or the last
`MinLiveTurns` whatever their age. A busy channel is decided by the window and
a quiet one by the count.

The time window alone described a busy channel and destroyed a quiet one. Three
messages across an afternoon are one conversation, and cutting at thirty
minutes left her answering a single line with nothing around it — which is
exactly the shape of a small server, where the persona is most likely to be
used.

The floor would have been a bad idea when the window was written, and it is
worth knowing why it is safe now. The transcript then carried no time at all,
so every line read as equally recent and old context genuinely did make her
answer the wrong thing. `labelled` now stamps the age into any line older than
`StaleTurnAge`, so an old turn arrives visibly old.

`MaxTurnAge` is the absolute cut. Past it a turn is not context at any count,
and what is older belongs to `mind.Memory`, which renders it as something
remembered rather than as something just said.

### What she knows she missed

The conversation buffer is in memory and starts empty, so on its own she knows
only what has been said since the process came up. That is wrong twice over: a
redeploy mid-conversation leaves her answering a question whose subject she
never saw, and being tagged into a discussion that has been running twenty
minutes shows her the tag and nothing else. Everyone else in the channel can
scroll up.

`chat.Service.backfill` reads the channel's own history from Discord the first
time she speaks there — one REST call per channel per process, and no model
call at all. That ratio is why it comes before any other memory work: every
other way of giving her context spends a backend request she may not get.

It runs on the worker rather than in `Observe`, which is on the gateway
goroutine and may not block on the network. The triggering message is therefore
already recorded when the fetch returns, and is also in what Discord sends
back, so `mind.Conversations.Seed` merges on message id rather than appending —
otherwise the message she is answering appears twice, once as itself and once
as its own echo. A channel that cannot be read is marked attempted anyway: a
missing `READ_MESSAGE_HISTORY` fails identically every time, and retrying would
spend a request per reply to keep learning it.

### Who she thinks she is

Three names can disagree, and all three are visible to her: `CHAT_NAME`, the
account username, and a per-guild nickname. Discord expands a mention to the
**account username**, so a bot configured as `Dev` but named `DevBot` reads
`@DevBot test`, is told only that it is Dev, and answers *"wrong door. DevBot
is not in here"*. That reached production.

Discord is therefore authoritative. `Service.namesFor` resolves the nickname,
display name and username per guild and puts them ahead of the configured
names; `CHAT_NAME` is a comma-separated list of extra spellings
(`ServerDomme,Server-Domme,Server Domme`), not the identity. The grounding
block opens by stating what she is called here and that every spelling means
her.

Name matching is `mind.SaysName`, a plain word-boundary scan rather than a
regex or a model call: it runs on every message in a watched channel, and a
classifier there would cost a backend request per message. It is stateless
because the name list is per guild and per message, so there is nothing to
compile once or cache — an earlier version cached compiled patterns in a map
and was a data race.

### The character file

`data/character.md` is authored content read at startup: prose, hard limits,
example exchanges, and a `## Notes` section that is parsed and discarded so
editing guidance costs nothing at runtime. Everything else in it is sent on
every message.

The assembled prompt is around 670 tokens against context windows in the
hundreds of thousands, so length is not the constraint people expect it to be.
What costs you is **position**. Two measurements, both made with
`cmd/chatprobe` and both easy to undo by accident:

- Folding "You are not an assistant" into the paragraph above it, rather than
  leaving it standing alone and last, dropped refusals on an assistant-shaped
  request from 6/6 to 2/6 over six runs of an identical prompt.
- The instruction to acknowledge a late reply was ignored in the system
  message and ignored again as a trailing system message. Stamping the
  message's age into the transcript worked. See `mind.labelled`.

The same shape a third time: "you never explain your own workings" sat as the
fourth clause of the anti-assistant paragraph, and under a question it could
not answer the character explained its own memory architecture in four runs out
of four. Its own paragraph fixed it.

The shape is consistent: an instruction buried among others stops being acted
on, at any length.

`cmd/chatprobe -backends` takes the same specs as `CHAT_BACKENDS`, and tuning a
character without it is measuring the wrong thing: every number recorded here
was taken against the g4f relay's donated servers, which are not the models a
self-hosted deployment runs and do not read the same way.

Measuring any of this needs care. The relay hands out a different donated
server per session, so two runs a day apart are two different models, and some
backends answer an identical prompt with a byte-identical reply — which makes
`-repeat` print one sample several times and look like agreement. `cmd/chatprobe`
names the answering backend and flags a repeated reply for exactly that reason. Examples are the other half of it — they are under a
quarter of the prompt and do more for the voice than any prose describing it,
which is why `mind.Build` replays them as real conversation turns rather than
quoting them inside the system message.

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

Two settings exist because of this case. `CHAT_REQUEST_TIMEOUT` raises the
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
