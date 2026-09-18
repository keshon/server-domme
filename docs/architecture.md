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
you do with a machine. It fires only when **nobody else** has spoken since she
last did, she did so **recently**, and the message is from the **person she
was answering**. All three conditions carry weight: once anyone else has spoken
the thread is no longer hers to assume, which is what keeps her out of
conversations between two other members.

"Nobody else since" rather than "she spoke last", because people type in
bursts. The stricter version counted only the first line of one: once she let
that pass, "hey" and "stop ignoring me" after it were not approaches at all
until the person tagged her. And a burst gets one answer — while an answer to
someone is queued or being written, further lines from them are not a second
approach, since the answer is built from the conversation as it stands when a
worker picks it up and will have them in front of it.

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

### Her bond with each person

How she stands with someone is one `mind.Bond`: **closeness** (slow — builds
over conversations, fades over weeks), **tension** (fast — gone in an
afternoon), and **welcome** (learned from how they take it when she comes to
them, starting neutral and drifting back to it). Missing them is not stored; it
is read from time and closeness (`mind.FeelLonging`). Role regard stays
separate, as standing an operator sets rather than a feeling.

Everything that moves a bond is an `Event` in one table (`appraisals` in
`internal/mind/bond.go`): pestering, a brush-off, a laugh, a pan, a remembered
conversation by tone and company, and how a reach-out was taken. Before this
they were separate mechanisms — irritation, warmth, brush-off, reactions — each
with its own write path and its own step, and two of them happened to use the
same `.34` without either knowing. Now a new reason to feel something is a row
in the table, and `chat.Service.appraise` is the only thing that writes a bond:
read, changed and put back in one transaction, with the event kept as the last
thing that moved her, so `/chat about` can say why she is the way she is with
someone. The values are the ones the separate mechanisms had; this changed the
structure, not the behaviour.

An event restamps only what it moves, so being asked to back off does not
reset how long tension has been fading. The stored field names are unchanged
(`irritation`, `warmth`), so no data migrated.

### Being annoyed with someone

Irritation is held **per person**, not per guild, because that is the whole
difference between someone annoyed and a bot in a bad mode: being short with
one member and perfectly ordinary with the next is what a person does. The mood
is the guild-wide layer; this sits under it and applies to one name.

It rises on countable behaviour — pressing again inside `PesterWindow` after
being passed over on a direct approach — and not on tone. A direct approach
only: tagging her after she let an untagged line go is how anyone repairs a
message that was not noticed, and counting it had her ignore someone and then
grow annoyed that they noticed (`Encounters.IgnoredDirectly`). Deciding whether a message was rude needs
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

`/chat state` shows how she is right now, for whoever runs the server: her
mood in words (`mind.MoodWords`) over the drives as bars, what they make her
want (`mind.Wants`), her stance towards each person in the conversation in a
word — short with, cool towards, fond of, likes, well disposed to, has little
time for, neutral (`mind.Attitude`) — with the closeness, tension and role
regard behind it, how her last line landed while that still counts, and every
instruction about her state the next reply would carry, verbatim
(`Grounding.Told`, built by the same functions as the prompt, and tested to
match it). The words are for the reader; the model only ever gets the
instructions.

It used to list the character file's temperament too. That only changes when
the file does, and on a panel about how she is right now it was the one thing
that never moved. The bars go inside a fenced block because Discord renders
labels proportionally, so "Energy" and "Interest" are different widths and the
columns after them do not line up otherwise.

`/chat forget confirm:yes` wipes what she remembers about a guild, and what she
holds against the people in it. Irritation goes with the memories rather than
outliving them: clearing one and not the other leaves her short with someone
for a reason she can no longer name, which is the failure the irritation memory
was added to prevent. Message counts survive, because they are how she knows a
regular from a stranger and wiping them makes everyone in the server new — a
much larger thing than being asked to forget what happened. The confirmation is
a typed word rather than a button, because this cannot be undone and there is
no copy.

### Coming after someone who asked for it

A member runs `/attention enabled:true` to let her come after them when she
wants their attention, and `/attention enabled:false` — or tells her to leave
them alone, go away, stop pinging them (`mind.WantsPeace`) — to end it.
`/attention` on its own shows where they stand. An administrator adds
`whole_server:false` to switch it off for the server, whatever anyone opted
into.

Consent is on or off. It used to come in three levels — light, keen,
insistent — each a fixed daily cap and cooldown, and that was a pattern: pick
"keen" and she came every four hours like a reminder. Consent does not create
the behaviour; it removes a guard. Without it she never goes looking for
anyone. With it, whether she does is up to how she feels, like everything
else here:

- **Missing them** (`mind.FeelLonging`) grows from the time since they last
  spoke to her, on a curve that rises faster for people she is close to —
  absence is felt sooner for someone who matters. It is computed on read from
  `LastExchangeAt`, never stored.
- **Being ignored** sharpens it: they have been active somewhere in the server
  in the last half hour and still not spoken to her in three. For people who
  opted in, a message in a channel she does not read records a timestamp and
  nothing else — not where, not what.
- **The urge** (`mind.Urge`) is missing them weighted by closeness, plus being
  ignored and being alone, minus being tired, minus any tension with them —
  annoyed with someone, she does not want their attention at all. It is scaled
  by how welcome she has learned she is (`0.5 + welcome`) and rolled against
  squared, so a middling urge rarely acts.

How often she comes is **learned, not chosen**: welcome is part of her bond
with them. An answer to a reach-out raises it (+0.10, +0.15 inside the hour),
leaving one unanswered lowers it (−0.12), and being told to back off drops it
by 0.30. The shortest wait between reaches runs from 2h for someone glad to
hear from her to 12h for someone who is not (`mind.ReachGap`), doubles with
every reach left unanswered, and varies by a quarter either way so it is never
a clock. Someone she is indifferent to may opt in and hear nothing for days,
which is the point.

Reaching out also spends and is slowed by her initiative fatigue, like
everything else she starts. The guards that stay are safety limits rather than
a personality: at most four a day, never between 23:00 and 09:00 in the community's timezone, never when
she is worn out, never while they are already talking to her, and nothing more
after three unanswered until they speak to her. Speaking to her resets the
count.

A timer of about ten minutes, varied by a third each time so it never lands
on the same minute, looks at the people who opted in — the one thing she does on
a timer, because absence is only noticeable over time, and only for people who
asked for it. A backend call happens only when she decides to act. She reaches
them in the channel they last talked to her in, tagging them (and only them;
the tag is added if the model leaves it out). The reason goes after the
transcript, as a trailing system message: stated only in the system prompt,
the relays answered whoever had spoken last in the channel seven times in
eight, since the person she is after is usually not in the conversation at
all. Measured with `cmd/chatprobe -only reach:` — fond of him and ignored:
"been watching you orbit everyone else for a day. bo still pretending you don't
exist or has he finally given up on you?"; indifferent, three days on: "you
alive or just being dramatic again?".

`/chat about` shows whether someone lets her come after them, how much she
misses them, whether they are around ignoring her, how many reaches are
unanswered, and the welcome gauge with the rest of the bond; every reach is in
the journal for `/chat why`.

### When a conversation has run out

"same", "ok", "lol", "yeah" after something she said close a topic rather than
open one. They were weighed like any other follow-up — eighty per cent odds of
an answer — and the answer was then forced to exist: told to say something
brief rather than pad, with nothing to say, she greeted again, echoed the word
back and added filler ("hey, same here. just another day in the server").

Three things now handle it:

- **Closers are recognised** (`mind.IsCloser`): a short list of
  acknowledgements and emoji-only messages, never anything with a question
  mark. Inside an exchange — a follow-up, or a reply to her — they are weighed
  at `CloserChance` (0.15) instead, and the rails that force an answer to a
  first or second approach do not apply: letting "same" go is how a
  conversation ends, not a bot failing to answer. Letting one go is not
  recorded as ignoring the person, so their next message is not forced through
  and a quick one does not count as pestering. A direct @mention is someone
  asking, whatever it says, and is never a closer.
- **She may still decline** (`mind.DeclineNote`) a follow-up or an overheard
  remark by answering `SKIP`: something likely to deserve an answer but not
  owed one. Measured on a follow-up that plainly asked something, she declined
  none of five. A mention or a reply to her is never offered it. Typing for a
  declinable answer shows only once the reply is certain, for the same reason
  an afterthought's does.
- **When she answers a closer, she is given something to bring**
  (`mind.FlatDirective`, `SomethingToBring`): one fact from the person's file
  or a memory still bright enough to recall, chosen at random among those, and
  told not to greet, echo or fill. Offered SKIP as well, she declined three
  times in four even with a fact to hand, so with something concrete the
  decision to speak stands and SKIP is not offered; with nothing to bring,
  letting it drop is her call. With the fact: "speaking of the usual, how's
  Bo? still think the ceiling fan is a threat?"

While measuring this, two replies in five answered things that were in none of
the prompts sent — "the rules from yesterday", "a link or article" — matching
other chatprobe scenarios not run at the time. The public relay appears to
return answers belonging to other requests now and then. Nothing here can
detect that reliably; it is worth knowing when she says something from
nowhere.

### How her last line landed

When someone answers her — a Discord reply to her, or the follow-up from the
person she was just talking to — `mind.ReadReception` reads the reaction from a
short word list: laughter or praise, being told it was bad, or being told she is
repeating herself. It moves her at once (`LikedWarmth` towards them, or
`PannedIrritation` against them) and her next reply to that person is told how
it went, as the last block of the system prompt before any reason for speaking
unprompted. The reaction is used once and forgotten after `ReceptionWindow`.

Everything else she felt moved slowly — pestering, or a summary written after
the conversation had gone quiet — so within a conversation nothing registered.
In production she told a joke, was laughed at, told another, was told it was
lame, told the first one again word for word, and was told she was repeating
herself: four replies from an identical inner state, and the model had no
reason to change course. Measured with `cmd/chatprobe -only reception`: told
she was repeating herself, she owned it in four runs of four ("fair. i walked
into that") and changed joke in three. Panned, she first came out
accommodating — "i have others if you are curious" — which is off for her, so
the directive now tells her not to apologise or offer more; after that, four of
five answered in character ("guess the standards are slipping").

It is crude and English-only, and applied only to a message answering her,
which is what keeps "lame" about somebody else's game from counting.

### Typed, not composed

Models write finished prose: a full stop at the end, em-dashes, typographer's
quotes, the odd semicolon. People in a chat do none of that, and it is the kind
of tell nobody names but everybody notices. `mind.Casual` evens it out on the
way to the channel, after every check that could drop a reply, so what is
recorded and compared against next time is what people actually saw:

- The final full stop goes. People mostly leave it off, which makes one a
  signal — "yeah." is not "yeah" — so it stays when she is short with the
  person or cold towards their roles (`mind.IsCurt`), where the curtness is
  the point. An ellipsis always stays.
- Em and en dashes become " - ", curly quotes and apostrophes straight ones,
  "…" three dots, and a semicolon between clauses a comma.
- With `CHAT_CASUAL_SLIPS` odds (0.2 by default) a message drops the
  apostrophes from casual contractions: "dont", "im", "thats", "youre". Only
  those that stay unambiguous — "we're" and "i'll" would become other words —
  and never when she is curt, where precision is part of the effect.

Anything with a code span or a link is left exactly as written: those are
copied, not typed. There are deliberately no typos. Invented ones look
invented, and once a reader has noticed the pattern the whole persona reads as
a trick.

Done here rather than asked for in the prompt: asked, a model half-complies and
then drifts back, and a rule about punctuation is prompt spent on punctuation.

### Not saying the same thing twice

Her own messages are replayed as the model's own turns, which is what lets her
see what she already said — and which a small model reads the other way, as
examples to copy. `mind.RepeatsHerself` checks a reply against her lines in the
live conversation before it is sent: the same line, the same opening five words
(one joke retold with a new tail), or mostly the same words. Short lines are
exempt; "no." twice is how people talk. On a repeat the reply is asked for once
more with what she already said quoted and ruled out. If that repeats too,
nothing is sent and nothing is held: a retry would build the same prompt and
get the same line, and silence is better than a loop.

### Being brushed off

If she asks someone a question and, within `BrushOffWindow`, they turn and
speak to somebody else — a Discord reply to another person's message, or an
@mention of someone else — without having said anything to her in between,
her irritation with them rises by `BrushOffStep` (`mind.BrushedOff`). That
is just past the band where she is a little cooler with them, and the
ninety-minute halflife brings it back under in about a quarter of an hour. Each
question counts once.

Cognitum had this idea and its version is what not to copy. It expected an
answer within ninety seconds of anything it said, grew anxious on silence, and
the anxiety fed on itself with no floor until it was the character's whole
mood. Silence is usually someone making tea, so it does not count here. Nor
does an untagged message: it may well be the answer to her question, and she
cannot tell. Only the unambiguous shape counts, because a snub read wrongly
means someone treated coldly for nothing. And it feeds the irritation that
already exists rather than a new feeling, so it decays like everything else
and cannot become a mood of its own.

Her own turns record who they answered (`Turn.To`), which is what ties a
question to the person who could ignore it. Turns read back from Discord
history do not have it, so nothing from before a restart is ever held against
anyone.

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
same memory twice. Only turns after the newest stored memory are considered,
and of those only the last session — the run since the last half-hour silence
(`mind.LastSession`). Without that, a buffer refilled from Discord could fold a
week of a quiet channel into one memory, part of it for the second time.

The buffer is in memory, so a restart used to cost her whatever had not been
remembered yet: the sweep only looks at channels in the buffer, and a channel
only came back into it when she next spoke there. On a bot redeployed several
times an afternoon that was most conversations. The sweep now starts by
reading back the history of every opted-in channel it has not seen this
process (`catchUp`), one REST call per channel, so a conversation interrupted
by a deploy is still remembered once it settles.

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

### What she knows about people

Beyond counting messages, she keeps three things per person per server, all on
the `MindPerson` record, all shown by `/chat about @user` and all cleared by
`/chat forget`:

- **Facts** — what someone plainly said about themselves: job, city, a pet, a
  plan. At most `MaxFacts`, newest value per key wins, oldest dropped. Only the
  newest four reach a prompt; twelve facts about one person reads as a dossier
  rather than as knowing someone.
- **An impression** — her one-line opinion of them, in her voice.
- **Warmth** — the counterpart of irritation: how much she has come to like
  them, 0..1, decayed on read with a three-week halflife.

Facts and the impression are written by one extra backend call after a
conversation is remembered (`mind.NotesPrompt`), never on the path of a reply.
It is separate from the summary so a relay that mangles one format does not
cost the other. Lines are tied back to people by the names they spoke under; a
name the model produced that was not in the conversation gets no file.

The prompt excludes health, sexuality, religion, politics, real names,
addresses and contact details. Everything here is kept and repeated back into
later conversations, possibly in public, and a character who casually brings up
someone's diagnosis does real harm whatever the server is about. The exclusion
is an instruction to a model, so it is a strong default rather than a
guarantee — which is the other reason `/chat about` exists.

Cognitum had both halves of this and both went wrong in instructive ways. Its
facts were first-come forever, so a changed job was never learned. Its
reflection ran every twenty seconds over the same fifteen memories and talked
itself into a fixation — a dozen near-identical observations in a row in its
log. Here the impression is revised at most once per remembered conversation,
and the call is given the previous one with an instruction to keep what still
holds. Measured with `cmd/chatprobe -notes`: "loud, loyal, easy to wind up"
became "loud, loyal, easy to wind up but also a softie" after a conversation in
which someone called him one.

The persona goes into that call. Without it the relays wrote neutral
caseworker's notes — "seems friendly and observant" — which then sat in her
prompt as "your take" pulling her voice the same way. With it: "protective
underneath the bluster".

Warmth moves without a model call, from the tone the summary already reads,
attributed the way irritation is (`mind.WarmthStep`): a warm one-to-one counts
most, a warm group conversation a little for everyone, an ordinary one-to-one a
little — people grow fond of whoever keeps turning up — and a hostile one-to-one
takes some away. It is rendered as an instruction for whoever present she likes
most (`mind.WarmthDirective`), phrased as something she would not admit to,
and never for the person she is also told to be short with: both at once is a
contradiction the model resolves at random.

### A private thought before she speaks

`CHAT_INNER_VOICE=true` has her write one private line before each answer —
her reaction to the moment and to the person — inside `<inner>…</inner>`, then
the message. The thought is split off and never posted; the latest one per
channel shows in `/chat state`. It is off by default.

Cognitum's liveliest behaviour came from this, as a separate call whose one
line was fed to the reply as "currently thinking". Here it is the same call:
the free relays are the bottleneck and a second request per reply would halve
what they can carry. It still costs something — every reply is longer to
generate and asks more of the relay's model — which is why it is a knob.

A tag rather than a `THOUGHT:` label, because `ai.Clean` strips a leading label
and cuts at any later line shaped like one, which would eat the message.
`mind.SplitThought` refuses to post anything whose thought has no clear end — an
unclosed tag, or a thought with no message after it — because a private thought
reaching the channel cannot be taken back. On that failure the reply is asked
for again at once without the thought, so a formatting slip does not turn into
a late answer. The closers relays actually wrote (`<inner>` again, `</ inner>`)
are accepted.

It is not asked for on an afterthought, whose reply is already a line or SKIP,
nor on something she volunteered, where the reason she is speaking is the
thought. Nor is a thought carried into the next prompt: a thought fed forward
is how cognitum's reflection found a subject and could not leave it.

Measured with `cmd/chatprobe -inner` against a baseline run in the same session,
because the relay's model changes between sessions. What it found:

- Asking what she "makes of this" produced plans — "they want a quick rename
  snippet; i'll give a minimal example" — and the reply followed the plan into
  assistant mode. Asking for her reaction to it and to them produced reactions,
  and some of the best lines seen from her ("google 'python rename files
  os.rename'. i don't write homework.").
- With it on, volunteered remarks lost their point: asked to greet a regular
  back, she thought about what they had missed and answered that instead.
  Hence the exclusion.
- It makes her less disciplined about length on some relays: a thought that
  runs to a paragraph tends to bring a longer, more formal reply with it.
- The first wording failed to split about one reply in four; the tolerant
  closers and the immediate plain retry cover both shapes seen.

It is worth trying on a given deployment and watching `/chat state`, not worth
switching on blind.

What the thought is not, since its name invites the mistake: it is not her
thinking. Her mind is the Go side — mood, bonds, fatigue, the decision to
answer — and all of that is settled before the thought is written. The thought
colours one message, decides nothing and is never fed back; the panels call it
her "first reaction, from the model" for that reason.

Asking for it on a reach-out was a bug, found while adding perception below: a
reach-out clears `Volunteering` and sets `Reaching`, so `Build` asked for the
thought and `speak`, which skips things she starts, never took it back out.
Both now use `Grounding.Answering`, and a test pins it.

### How their message came across

Her mind reacts only to what it can count, so it cannot tell "you're actually
funny" from "you're a bot, aren't you" unless somebody reacts with an emoji.
The model can, and it has the message in context in the call that answers it.
`CHAT_PERCEPTION=shadow` asks it, in that same call, for one word from a fixed
list — warm, playful, flirty, needling, hostile, neutral — between `<tone>`
tags (`mind.PerceiveNote`). A label from a list rather than free text, and read
by Go rather than acted on by the model: free text fed back is how cognitum
fixated, and a model setting her feelings is how its state came to mean
nothing.

In shadow mode the label is taken out of the reply, written to the journal —
`/chat why` shows "Read their message as", or `unreadable` when the model gave
nothing usable — and logged as `chat_perceived`, and nothing else happens. It
does not move a bond or her mood. The point is to check the labels against
what people meant, in real conversations, before any of them is allowed to;
the relays were measured answering "was this rude?" confidently and
arbitrarily once already. Only answers are labelled, like the thought: the
things she starts have no message of theirs to read.

Measured with `cmd/chatprobe -only perceive: -perceive`, six messages whose
tone a person would read one way, three runs each, twice. The first run showed
two of eighteen replies using the word itself as the tag — `<warm>`,
`<neutral>unfortunately</neutral>` — which would have been posted as they
were; `SplitPerception` now reads that shape too, and a reply that is only a
label is asked for again at once without one. After that fix, 27 of 33
readable labels matched the intended tone, and every miss was to a neighbour
(warm read as neutral twice, flirty as playful, needling as playful, hostile as
needling) rather than across — never warm for hostile.

That is good enough to watch and not yet good enough to act on. If the journal
agrees with what people meant over a week of real use, the next step is a small
weight: each label an event in the bond table, smaller than the counted events,
with Go deciding what it does.

### How she is doing

`mind.Drives` is four numbers — Social, Energy, Arousal and Mood — derived on
read from timestamps, counts and a few stored values. It is the cognitum experiment's symbolic core
with its price removed: that design kept state in the same way and spent around
eighty model calls an hour doing it, a third of which failed to parse. Nothing
here calls a backend.

Derived rather than ticked. cognitum recomputed on a one-second timer, which
needs a goroutine, loses everything on restart and burns cycles in an empty
server; computing from elapsed time when the value is wanted gives the same
curves for nothing and survives a redeploy, because the timestamps do.

Social, Energy and Arousal each have an input this bot can observe, and
cognitum's Coherence had none. A drive fed by nothing drifts convincingly and
means nothing. Mood is fed by what happens to her.

Energy follows an anchored day curve rather than a cosine, because a 24-hour
cosine is symmetric — putting the trough at 04:00 necessarily makes 08:00 just
as dark. `CHAT_TIMEZONE` is the community's zone, not the host's: a bot yawning
through someone's prime time is worse than one with no clock at all.

**Arousal** (it was called Interest) is how busy the room is and how much of
it is aimed at her, worn down by habituation: `mind.Repetition` is the share of
each line's keywords an earlier line already used, her own lines left out, and
a room circling the same words holds her up to 60% less. The fifth round of the
chicken joke interests her less than the first, which a count of messages
cannot see.

**Mood** is how her day is going, -1 to +1, one per server (`mind_guilds`). It
is the sum of three things:

- **her temperament's baseline** — the character file's `warmth` dial, so a
  reserved character settles a little below neutral and a warm one a little
  above (`SpeechStyle.MoodBaseline`);
- **the day's tone** (`mind.DayTone`), seeded from the guild and the date so it
  holds all day, survives a restart and differs between servers. Cubed, so
  most days are flat and about one in six is clearly good or clearly bad. It
  moves her energy by up to 0.12 and her mood by up to 0.35: noticeable but
  rare, a day regulars would call her being in a mood;
- **what has happened** (`mind.MoodSwing`): every event in the bond table also
  carries a mood shift, and a remembered conversation moves it once by its
  tone (`mind.ConversationMood`). The swing halves every three hours.

The spillover is what lets one bad exchange colour the next conversation
without becoming a grudge against someone who had nothing to do with it.
Someone pestering her raises her tension with them by 0.34 and lowers her mood
by 0.05: sharply short with them for an hour or two (tension halves every 90
minutes), and faintly off with everyone for a few hours after (mood halves
every three) — the funk that outlasts the argument. Irritation always spills
over less than it is felt towards its cause (tested). Conversations move her mood once however many people were in
them, and unlike the bond a group counts: a room that turned hostile sours her
even when nobody in it can fairly be blamed.

They reach the reply twice. `Drives.Nudge` moves the odds of answering by up to
a fifth — but only for the indirect approaches. A direct mention or a reply is
answered on its own terms whatever the hour: someone tired still answers when
spoken to, and making that conditional is how "she ignored my direct question"
returns with a better excuse. The zero value nudges by nothing, so an unset
mood is not a bad one. Mood is part of the nudge (±0.08), and closeness to the
person counts on every trigger, up to +0.08 (`mind.ClosenessNudge`) — smaller
than tension takes away, because being annoyed with someone is the sharper
feeling. Over a year of simulated afternoons the average odds per trigger moved
by a point (named 0.69 → 0.70, overheard 0.29 → 0.30, follow-up 0.84 → 0.85),
and now vary by about ±5 points with her state
(`TestAnswerRatesStayNearTodaysOnAverage`).

`Grounding.State` is the other half, and the shape of it is the point. The
first version stated the mood as a fact in the grounding — "Right now: it is
the dead of night and you are running on fumes" — and measured **no effect at
all**: asked the same question at 3am and at 8pm, the exhausted run produced
the longest and liveliest reply of the set while the wide-awake one answered
"which film?". Stating a mood asks the model to infer a writing style from it,
and it does not. This is the third time that has been measured here, after the
late-reply note and the anti-assistant rule.

Phrased as instructions and placed last — after the output rules, where nothing
follows it — the same states produce "haven't seen it" at 3am against full
sentences when awake. Only a pronounced state speaks: a list of qualifications
on every reply is how a strong instruction becomes a weak one. A mood beyond
±0.45 is part of it. Measured with `cmd/chatprobe -only "mood: a"` on
"finally finished that puzzle i was stuck on all week": the first wording ("let
a little of it through") read the same on good and bad days, the lesson this
section already records. Stated as behaviour — a short fuse and no playing
along; more generous, tease rather than dismiss — the bad days came back as
"took you long enough. hope it was worth the suffering." and the good ones as
"nice work, don't let it go to your head". A small difference, which suits a
character this dry.

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

### What is on her mind

Everything she starts on her own is an event meeting something that matters
to her: a regular walking back in, a subject coming round, someone she misses
staying away. The events were modelled; how much the thing they touch
matters to her was a constant per kind of event — 0.6 for a returning regular,
0.35 for an old subject — except for reaching out, which already computed it.
Salience is that second half, computed once, so that later every event can be
weighed by it instead: stimulus × salience, fed to the one initiative decision.

`mind.OnHerMind` ranks the people she knows and the subjects she remembers in a
server, at most five, above a floor of 0.15. A person's salience combines
separate pulls — fond of them, annoyed with them, just talked, misses them,
around and not talking to her — as independent chances, one minus the product
of what each leaves, so two reasons count for more than one without anything
running past 1. A conversation lingers about three hours, longer with someone
who stirs something in her either way; absence only registers for someone she
is at least a little close to, because nobody misses an acquaintance. A
subject is a memory's brightness raised by how much it mattered, and capped at
0.6: memories fade over days, so everything from today is near full
brightness, and uncapped the last conversation's subject outranked everyone
she knows.

It is read-only. `/chat state` lists it as **On her mind**, with the reasons
and the number, and `/chat about` gives one person's; nothing she does is
driven by it yet. The ranking has to match the reader's own sense of her
before anything leans on it — the same reason perception waits in shadow
mode. Run over the test server's store it read: *Big M kept pushing after
being left alone* (0.38, three hours ago) above **Big M** himself (0.29, just
talked, annoyed with him).

### Something they said they were about to do

A person who mentions a job interview tomorrow is, to anyone who cares about
them, someone whose interview was yesterday. `mind.Concern` is that: something
on her mind about someone, because of something they said they were going to
do. It is not a behaviour. Nothing tells her to ask how it went or to wish them
luck.

**Where they come from.** The notes call, which already reads each settled
conversation, has a third line shape: `PLAN Big M: clinic interview |
tomorrow`, for things a person said they themselves are about to do, with the
time from a fixed list — tonight, tomorrow, this weekend, next week, later. Go
dates it (`mind.DueFrom`: evenings for the day-sized words, Saturday afternoon
for a weekend); the model never does date arithmetic. And Go keeps it only if
it is in that person's own words: half the plan's words, and at least one,
must appear in something they said (`mind.NewConcern`). A stored concern is
repeated back to them, so an invented one would be a false belief she acted on
for days. Plans used to be an ordinary fact — `plans = job interview` — with
no time in them at all.

**How much it is on her mind.** `Concern.Salience` is timing × care × mood ×
how often she has let it pass. Timing is a little in the two days before (0.3
at most: people ask how something went far more than they wish luck
beforehand), highest just after, halving every two days, gone five days on.
Care is `0.25 + 0.75 × closeness`. Each time it was on her mind and she let it
pass leaves 0.6 of it. For a plan due tomorrow evening:

| When | Close to them (0.8) | Indifferent |
|---|---|---|
| The night before | 0.15 | 0.04 |
| The day after | 0.71 | 0.21 |
| Four days on | 0.21 | 0.06 |

**What it does.** When she is answering that person, the most pressing
concern is on her mind with the probability of its salience, and then the
state paragraph ends with it as a fact with its time: "On your mind: Big M's
clinic interview was yesterday." What she does with it — a question, luck, a
jab, nothing — is hers; the time phrase is what makes "good luck" or "so how
did it go" fit. Afterwards Go checks whether her reply raised it (half its
words): raised, it is done with; let pass, it weakens. Their own message
closes it too, once it is within six hours of being due — the news arriving
before she asks. Only for answers, only about the person being answered, and
only people she has a record of.

`/chat state` lists concerns in **On her mind**, `/chat about` lists
someone's with their salience and how often she let them pass, and `/chat
forget` clears them with the rest of her file.

**Measured** on a local KoboldCpp model (Gemma 26B), since the public relays
were refusing this machine at the time; the relays themselves gave one run,
right on all three.

- **Extraction** (`cmd/chatprobe -notes`, three runs of three
  conversations): his own interview tomorrow was kept and dated 3 of 3;
  a conversation with no plans gave nothing 3 of 3; cass's brother flying to a
  wedding gave nothing 2 of 3 — and once came back as `PLAN cass: brother
  flying to Porto`, which the own-words check passed, because those were
  cass's words. A plan that names a relation (brother, friend, boss…), or
  comes from a line about "my/his/her <relation>", is now refused as well
  (`aboutSomeoneElse`), and a test pins that case.
- **Surfacing** (`-only concern:`, four runs each, the concern forced on her
  mind): the day after, "hey. how did the interview go?" 4 of 4; the night
  before, "good luck tomorrow", "how's the nerves for tomorrow?" twice and
  "you're still up?" — luck came from the time phrase alone, nothing asks
  for it; while he asked something else, she answered his question 4 of 4 and
  left the interview alone, which in use counts as letting it pass.

Two things the run showed that are not about concerns: the local model gave
word-for-word identical replies across runs, because the bot sends no
temperature and KoboldCpp's default samples close to greedily; and asked
when movie night was, she answered "eight" — a time nothing in her context
contains, as the relays did earlier with "ten o'clock".

### One description of how she is

Her state used to reach the model as separate sentences, each written by the
feature that owned it — tired, lonely, absorbed, short with one person, fond of
another, in a mood — and each was measured and worked alone. Together they
could contradict each other: "a bad mood, less patience" beside "fond of cass,
more patience". A model handed a contradiction resolves it at random.

`Grounding.State` reads all of it and writes one paragraph of at most four
sentences under `Right now:`, with the contradictions settled in Go where the
rule is visible and tested (`state_test.go`):

- energy and mood are one sentence, not two that each assume the other is
  neutral — "a good day, if a tired one. Warm, but brief.";
- exhausted beats absorbed: "as few words as will do" and "say something with
  content" cannot both be followed;
- lonely and tired means brief, but staying in the conversation;
- a bad mood is not for the person she is fond of, and says so by name; if
  someone is getting on her nerves, the mood is for them;
- a good mood does not extend to the person spoiling it;
- one person is never both the one she is short with and the one she is fond
  of.

Still instructions rather than a description of feelings, in the same place
the separate lines were. Standing (`AboutThem`), how her last line landed, a
flat conversation and why she is speaking keep their own measured placements:
they are about this exchange, not about her. `/chat state` shows the paragraph
verbatim, and `/chat why` records it, because `Grounding.Told` is built from
the same function as the prompt.

Measured with `cmd/chatprobe -only state:` against the previous code and the
same scenarios, interleaved in one session, four runs each:

- **A bad day, and cass — whom she is fond of — shares a win.** The separate
  lines took the mood out on cass twice in four ("good for you. puzzles are
  boring anyway"); the one description never did ("nice work, cass. glad you
  cracked that puzzle.", "congrats. hope it was the right kind of stuck").
- **A good day, and Big M pestering her.** The separate lines let the good
  mood win twice in four ("you had me at hello"); the one description was short
  with him every time ("yeah i'm here. what do you want", "still waiting?").

Every other scenario was re-run twice against a baseline from the same
session, and none read worse. Only the state scenarios' prompts differ; the
one that changed most is exhausted and lonely at 3am, which used to carry both
"as few words as will do" and "engage with this" and now carries only the
first — "haven't seen it" and "name the film first." against the baseline's
"no idea which film you mean. i've spent the week being furniture."

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

A Discord reply that pings her also lists her among the message's mentions,
so the reply check runs before the mention check. The other way round, every
pinging reply was classified as a plain mention and never anchored.

She does not @mention people by default: an inline mention on every line reads
like a bot addressing a ticket, and the anchor shows the same thing. When she
does write "@Name" — because she chose to, or because someone asked —
`mind.ResolveMentions` turns it into a real mention for anyone in the
conversation. Only the person she is answering can be notified by it. Anyone
else she names renders as a mention and gets no notification, because letting
her ping whoever she names makes her an instrument for pinging someone else
with an insult. Roles, @everyone and @here are never parsed.

### Taking the initiative

Everything she starts rather than answers — speaking up when a regular comes
back or an old subject returns, a second line after her own reply, going after
someone who opted in — used to carry its own cap and cooldown: three a day
forty-five minutes apart, twenty minutes between double-texts, a daily
allowance and a cooldown per attention level. Each was reasonable alone;
together they were a timetable, and anyone watching long enough learns when she
cannot speak.

Now each is an opportunity with a pull of its own, and one thing decides them
all: her **initiative fatigue** (`mind.Fatigue`). It rises every time she takes
the initiative — 0.6 for speaking up unprompted, the most exposed thing she
does; 0.4 for reaching out; 0.3 for a second thought, when she is already
talking — and halves every four hours. Every opportunity's odds are multiplied
by how rested she is, cubed (`mind.Rested`), so a little fatigue barely matters
and half-tired she is at an eighth of her usual odds.

It is hers, per server, stored on `mind_guilds` and decayed on read: a mind
that has just chased one person is less inclined to chime in somewhere else,
and a quiet afternoon leaves her ready. `TestFatigueSpacesInitiativeUnevenly`
gives her a reason to speak every ten minutes for fourteen hours, two hundred
times over: she takes about four a day, near the three the caps allowed, with
gaps of 231 ± 123 minutes — spread out, never stacked at a cooldown.

Hard limits stay only where they are safety rather than personality: the daily
maximum per channel and per person, quiet hours, stopping after three
unanswered reaches. None of them should be what decides on an ordinary day.
`/chat state` shows the current fatigue, and `/chat why` records the chance and
roll behind each initiative, with the fatigue she decided under.

### What she gets out of it

Initiative fatigue is the cost of putting herself forward, paid whatever
happens. Reward is the other half, the part dopamine carries in a person: not
the outcome but how it compares with what she expected.

After a greeting, an old subject brought up or a second thought, she waits ten
minutes (`mind.PayoffWindow`) to see how it lands. The person it was aimed at
speaking to her again settles it — laughing (1.0), taking it up (0.7), or
panning it (0.1) — and nothing by the end of the window is being ignored (0).
A remark about a subject is to the room, so anyone taking it up settles it.
Reaching out learns the same way from its own answered and unanswered counts,
over hours rather than minutes.

- **Surprise moves her mood.** `mind.Surprise` is the outcome less what she
  expected, and her expectation of someone is her welcome with them — the one
  number she both learns and predicts from. A warm answer from someone she
  expected nothing from lifts her; the same answer from someone who always
  gives one barely does; silence from them stings more than from a stranger.
- **She learns who welcomes her.** Being taken up or let drop moves welcome a
  little (`EventInitiativeTaken` +0.05, `EventInitiativeDropped` −0.04) —
  less than reaching out, because a remark in a room they are already in says
  less about whether they want her than coming to find them does. Welcome now
  scales speaking up and second thoughts towards that person as it already
  scaled reaching out: unchanged at neutral, so nothing moves until she has
  learned something.
- **The need is satisfied by contact, not by speaking.** Her need for company
  used to reset whenever she spoke, which meant talking into a room that did
  not answer cured her loneliness. It now resets when someone speaks to her
  (`MindGuild.LastContactAt`). So when a greeting pays off, the need behind it
  is met and the next greeting comes less readily; when it falls flat, the need
  is still there, and whether she tries someone else is up to the roll.

One message moves her mood once. A laugh that also settles something she
started would otherwise have counted twice — as a laugh, and as the surprise
— and three times if it answered her reaching out as well, whose events once
carried a fixed mood share of their own. The surprise is now the only mood
effect of an outcome; the laugh or pan still moves the bond. An answer to her
reaching out is appraised by how it landed, not only by how fast: a prompt
"lame, stop" is not a welcome, and being told to back off is its own event
rather than an answer. A remark to the room is settled by the room taking the
subject up — two of her words in someone's message — since a room usually
answers among itself rather than to her. All three were found by an
independent audit and each has a regression test.

`TestAGreetingThatPaysOffSatisfiesTheNeedBehindIt` shows the loop end to end:
alone all afternoon, her odds of greeting the next person are 0.68; after a
greeting that was taken up, 0.53; after one that was ignored, 0.67. Fatigue
lowers both further, equally. `/chat why` records each payoff on the entry of
the thing she started — "laughed — expected 0.50, surprise +0.50" — and the log
carries it as `chat_paid_off`.

### Speaking first

`/chat proactive enabled:true` lets her speak without being addressed in one
channel. It is off by default and per channel: a general channel can take her
greeting someone back, a support channel cannot take her bringing up last
week's argument. Switching a channel off with `/chat silence` switches this off
with it.

There are two reasons she will, and both are something a person just did:

- **A regular comes back.** Someone she knows — not a newcomer — speaks after
  two weeks or more away (`Acquaintance.AwayFor`). Greeting a stranger's
  return reads as being watched rather than recognised.
- **An old subject comes round.** The live conversation shares enough words
  with the gist of a memory at least six hours old (`mind.Relevant`). Matched
  against the gist alone: overlap is a share of the memory's words, so counting
  the detail made a vividly remembered episode harder to recall than a vague
  one. The age floor keeps her from bringing up the last hour as history.

There is deliberately no third reason of breaking a silence. Nothing here runs
on a timer, so a quiet channel stays quiet; a bot that posts into an empty room
on a schedule is the most recognisable bot behaviour there is.

`mind.MayVolunteer` then decides it. The gates are about the moment:

- never while she is already in the conversation, where what she says next is a
  reply;
- never when she is worn out;
- for a memory, never into two other people's exchange — the same barging that
  kept `about` from being an approach. A returning regular is noticed however
  busy the room is;
- never past `VolunteerDailyMax` (six) in a channel in a day, counted in the
  community's timezone and persisted in `mind_channels`. A safety floor, set
  where her initiative fatigue should never let her reach it.

The odds are about her: the reason's pull (a returning regular 0.6, an old
subject 0.35), her mood, and how much she has put herself forward lately —
see [Taking the initiative](#taking-the-initiative). There is no cooldown.

Fatigue is spent when she decides, not when the message goes out, so a
remark that then fails to generate still counts: the error is towards saying
less. A volunteered remark is never held for a later retry either — nobody is
owed it, and arriving twenty minutes after its moment is stranger than not
arriving. The queue being full drops it for the same reason.

Why she is speaking goes last in the prompt, after the mood. Without it the
model reads the transcript, finds nothing aimed at her and answers the last
line as though it were. The wording of both directives was measured against the
relays with `cmd/chatprobe -only volunteer`, and two versions failed in ways
worth knowing about:

- Asked only to greet someone back, she answered their "what did I miss" by
  inventing a recap. The directive now says she does not know what happened.
- Telling her she did not know the answer to the question the room was on
  sent her into assistant mode — "I don't have visibility into server events".
  Stating what a model does not know invites a disclaimer. The recall
  directive now describes the one thing to say instead: a dry remark that the
  subject is back.

The same probe turned up a relay answering with the transcript's last line word
for word. `mind.Echoes` catches a reply that is only someone else's line and
treats it as a failed generation — an answer is retried, a volunteered remark
dropped — and `ai.Clean` now drops leaked `<tool_call>` scaffolding.

### A second thought

After a clipped answer she sometimes sends a second message a few seconds
later: "are you bored?" → "yes" → "then stop being boring." People double-text;
a bot that sends exactly one message per approach has a rhythm nobody has.

Go decides whether one is allowed (`mind.MayAddAfterthought`), gates before
the roll:

- only after an answer, never after something she volunteered, a late reply or
  another afterthought;
- only after a short reply — six words or fewer. A reply that said its piece
  has nothing to add;
- not when she is tired, irritated with the person or cold towards their role,
  where a curt answer is curt on purpose;
- not when others are talking, where a second line lands in their exchange.

What keeps it rare is her initiative fatigue rather than a cooldown: a second
thought tires her a little, so the next one is less likely for a while, and so
is speaking up anywhere else. In a rapid one-to-one of short answers that comes
to about two an hour, near what the old twenty-minute cooldown allowed, but
never on the twenty-minute mark.

The model then decides whether anything is said. The request is a separate
call made after a pause of three to eight seconds, and its
last message is her own; a trailing system message quotes it and asks for one
more line or the word `SKIP`. Declining has to be allowed, or every permitted
afterthought becomes a sent one. Measured with `cmd/chatprobe -only
afterthought`: the first wording let her open by repeating herself ("yes. you
people are the entertainment"), so it now says not to.

Typing shows only once the afterthought has passed every check, for a second
and a half before it is sent. Shown before generating, as an answer's is, it
announced lines the model then declined to write, or that were dropped because
the person had answered meanwhile: typing that stops with nothing posted, which
reads as her writing something and deleting it.

It is dropped rather than sent late if the person has spoken since her message
(`mind.LastWord`, checked before generating and again before sending), if it
repeats her first line, or if the model declined. A second line arriving after
their reply is exactly the tell this exists to avoid. Like a volunteered remark
it is never held for retry.

### Relay scaffolding in a reply

Some relays answer with something that is not a reply at all: a safety
classifier's verdict ("User Safety: unsafe / Response Safety: unsafe / Safety
Categories: Profanity, Harassment"), or `<tool_call>` markup. `ai.Clean`
empties those, which makes the backend's answer an `ErrEmptyReply`: the pool
counts it as a failure and tries the next backend, so the retry is silent and
nothing about it reaches the channel.

Posted, it does worse than look odd. Her own message is replayed to her as part
of the conversation, so once a verdict reached the channel she read it back as
something she had said and explained it in character ("the automated
moderation layer flagged my last reply"). That is why the guard empties the
reply rather than trying to reword it, and why the retry does not tell the
model that something was filtered: a character told about filters talks about
filters.

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

### Why she did that

Every question about her behaviour in testing was about one moment — why did
she ignore me, why that line, was that intentional — and the answer had to be
rebuilt from the code each time. The journal records it as it happens.

One entry per decision about a message that reached her (`storage.MindJournal`,
the last 50 per channel): who and what, how it reached her, the rule that
decided (`mind.DecideWhy`: first approach, ignored last time, a closer's odds,
the ordinary odds) with the chance and the roll, her mood and her stance
towards them. An answer carries its entry in the task (`Deferred.Journal`), so
the same entry is completed however it resolves — answered, declined, dropped,
held and answered late — with the state instructions it carried
(`Grounding.Told`), her thought, what the model returned, what was posted,
which relay answered (`Pool.GenerateNamed`) and how long it took. A reaction to
her reply is written onto the entry for that reply. Lines that arrive while an
answer to the same person is already coming get an entry saying so, since
"why did she not answer my second line" is the same question.

`/chat why` renders an entry in plain words: the latest in the channel, or the
one about a message given by link or id — theirs or her reply. It is kept in
the datastore, cleared by `/chat forget` with the memories since it carries
excerpts of what people said, and trimmed to excerpts for the same reason.

`/chat status` shows today's counts beside it (`storage.MindDay`, in the
community's timezone): answered, silent, declined, volunteered, afterthoughts,
liked, panned, told she was repeating, repeats and echoes caught, relay
failures. Counts rather than the journal, because a busy channel turns fifty
entries over in an hour and whether a change made her better is a question
about a day, not about a moment.

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
