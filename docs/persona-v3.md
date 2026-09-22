# The persona, v3 — research spec

> Don't make her more human by adding simulated human features. Make the
> system less perfect in the specific ways people are less perfect, and give
> the model enough persistent substance to make its own meaning.

Status: **architecture frozen, not built.** Written 21 September 2026,
revised twice the same day after outside review, and frozen; work starts
22 September. This is a research experiment: each part below is a hypothesis
with a way to measure it and a switch to turn it off, so it can be kept,
tuned or thrown away on evidence. It builds on [persona.md](persona.md) (v2),
which stays the description of what is running until a part of this lands.

**What frozen means.** The architecture is fixed: the principle and its
boundaries, the layers, provenance and its invariants, the lifetimes, and
the workstreams and their order. Changing any of those takes evidence from a
measurement, not a better argument — and is recorded in the change log at the
end. Constants (thresholds, half-lives, caps, intervals, probabilities) are
**not** frozen: they are starting values, to be tuned in `bodysim`, chatprobe
and production.

## Why a v3

v2 fixed what killed v1: she owns what she said, softens to an apology, and
never posts scaffolding. What it produces is *coherent* and *generic*. Replies
are appropriate, in character, and interchangeable with any other dry,
aloof, secretly-warm character card, of which a model has seen thousands.

People infer a mind from three kinds of signal:

| Signal | Meaning | v2 |
|---|---|---|
| **Coherence** | she makes sense and stays the same someone | strong: memory as prose, owning her words, rails |
| **Inner life** | she has state nobody in the chat caused | nearly none: she exists only when poked |
| **Limits** | she cannot take in everything, and is not always there | none: perfect reading, perfect recall, always online, never tired |

And under all three, two things she is missing:

- **Content.** The character card is all stance — how she treats people —
  and no substance: not one opinion about anything, not one thing she is
  into. The model fills the gap with the median of the archetype, freshly
  each time.
- **A stable substrate.** The backend pool answers each call from whichever
  relay ranks best right now, so the appraisal and the words of one reply
  can come from two different models, and the next reply from a third. Every
  model has a native voice; she is six people sharing a card.

## The principle

v2: *the model is the mind; the code is the body and the clock.* v3 keeps it
and makes the split exact:

> **Code controls mechanics. The model interprets. Code decides what
> persists.**
>
> Anything that changes while nothing is happening — sleep pressure, social
> energy, a feeling fading, a need building up — is code. Anything that
> asks what something *means* — an apology, a gift, a jab, what she wants — is
> the model's to interpret. Whether that interpretation becomes lasting state
> is the code's to decide.

The split in full:

| Code may | The model decides | Code must not |
|---|---|---|
| control access, timing, capacity, routing, eligibility and persistence; use mechanically observable facts (clock, counts, presence, battery) to constrain what the model gets | meaning, salience, emotion, intention, social reading | manufacture a semantic conclusion — tell the model what something means, what she feels, or what she should do |

"Here are four messages" is mechanics; "here are four messages because you
are tired" is a conclusion. "You have been awake since 09:00" is mechanics;
"you should feel tired" is a conclusion. Some mechanisms sit close to the
line and are allowed because they only decide eligibility, never meaning:
H3's keyword filter decides what she *notices*, not what it means or whether
she joins; B3 decides how much she is *given*, not how she should take it; a
reaction counting a third of a start is a rail, not a reading. The day one of
them starts deciding what she does, it has crossed the line.

Three boundaries make that exact:

- **The model proposes; the code commits.** Everything the model says about
  her — a feeling, a self-fact, a want, a line of her life, a note on
  someone — is a proposal. The code validates it, stamps where it came from,
  and only then persists it (workstream I). Without this, the model
  interpreting slowly becomes the model owning reality.
- **The code controls mechanics, never interpretation.** The code decides
  what is in her field of view and when — four lines of transcript, no
  recalled memories, a message three hours late — and never what that
  should mean.
- **Facts, not instructions, both ways.** The model hands the code numbers it
  chose: `weight`, and rarely `energy`. The code hands the model statements
  of fact: ages, clock times, counts, durations. Never "be brief", never a
  score.

v1 put meaning in the code (word lists, scalars rendered as directives) and
became a caricature. v2 gave time to nobody, and let the model write straight
into memory. v3 is the step between.

Three corollaries, the first two already learned once:

- **Imperfection is produced structurally, never requested.** A model told
  to be distracted performs distraction; a model given half the transcript
  genuinely misses things. The same reason there are no invented typos in
  `Casual`.
- **A number never enters a prompt.** Battery, sleep pressure and feeling
  intensity decide what the model is *given* and *when*, not what it is
  *told*.
- **Specificity is imported, not generated.** A life the model invents for
  her is the median again — "been reading a good book lately". A real input
  carries its own detail: a real sketch in a real channel, a real argument
  that comes back every month. What she has to talk about should come from
  things that actually happened, authored or observed.

## Layers

```
 body          code, numbers      sleep pressure · circadian clock · social battery
                                  · presence (online / away / asleep)
     │ decides when she sees anything, and how much
     ▼
 perception    code               transcript tail, recall size, associative drift,
                                  missed messages on return, reactions as facts,
                                  walks through channels she reads but not speaks in
     │
     ▼
 affect        model names,       active feelings with a cause; each fades on
               code fades         its own clock
     │
     ▼
 mind          model, prose       consider → speak, with specifics, self-facts,
                                  her life, what is on her mind, what she wants
     ▲
     │
 idle mind     model, cheap       between conversations: what is on her mind,
                                  and now and then a walk through the town
     │
     ▼
 commit        code               every proposal validated, stamped with its
                                  source, ranked by provenance, then written
```

This is a closed loop — body, perception, mind, action, environment, back to
perception — and the model is not the loop. It is the part of the loop that
interprets.

The parts below are grouped into workstreams. Each one lists the design, what
the prompt may see, the switch, and how to tell whether it worked.

---

## A. Backends: an order you choose, failover only when something fails

**Problem.** `ai.Pool` ranks by score; the configured order is only the
starting rank. Which model speaks is not controllable, and consecutive calls
wander between relays.

**Design.**

| Piece | Behaviour |
|---|---|
| **Priority mode** (default) | Backends are tried in configured order. One in cooldown is skipped and takes its place back when the cooldown ends. Score is kept for `/chat status`, not for ordering. `CHAT_BACKEND_ORDER=score` keeps today's behaviour. |
| **Escalating cooldown** | Each consecutive failure doubles the cooldown: 90s, 3m, 6m … capped at 30m. One success resets it. This, not score, is what stops a flaky preferred backend flipping her voice every 90 seconds. Refusals keep their fixed 30m. |
| **Voice order** | `CHAT_VOICE_BACKENDS=g4f-1,g4f-3` — names from `CHAT_BACKENDS`. `Speak` tries these first, in order, and falls back to the rest of the list only when all of them are down — a preferred voice, not a cage, because silence is worse than another voice. `Consider`, `Reflect`, `Initiate` and the idle mind use the full list. Empty means the full list for both. `Mind` gains a second provider, `Voice`, which shares the pool's record of backend health. |
| **Stickiness per presence session** | Whichever voice backend she is on stays hers for as long as she is ONLINE (B2), across every channel, even once a preferred one recovers. When she goes AWAY or ASLEEP, the order resets to the configured one. A change of voice then happens while she is gone, where nobody can hear it, and she never speaks in two voices in two channels at once. A channel is the wrong unit: ten unrelated people can talk in one over six hours. The service holds the current voice backend bot-wide and passes `ai.WithPrefer(ctx, name)`; the pool tries that one first unless it is cooling down. With `CHAT_BODY=off` there is no presence, and the session is an `engagedWindow` of silence everywhere. |
| **Runtime control** | `/chat backends` — list with state; `order a,b,c`; `off name`; `on name`. Stored in the datastore; env is the default, the command overrides. Bot-owner only: backends are shared by every server. |

Recommendation for the current list: keep `BraveSearch` out of the voice
order. It is a search answer engine and sounds like one; it is fine as a last
resort for thinking.

**Measure.** chatprobe gains `-voice-backends`. Run the same log through each
relay alone and rank them on *holding the voice* (see *Measuring*), not only
on answering. The ranking is the default voice order.

---

## B. Body: sleep, social battery, presence

All of it in `internal/chat/body.go` (state and dynamics) and
`internal/chat/presence.go` (what the service does about it). **One body for
the whole bot** — she is one person, and Discord presence is global. Memory
stays per guild as it is.

### B1. Three quantities

| Quantity | What it is | Dynamics |
|---|---|---|
| **S — sleep pressure** | how long she has been awake | rises linearly while awake, empty to full in about 16h; drains exponentially asleep, time constant about 3h |
| **C — circadian drive** | what time of day her body is set to | `0.5 + 0.5·cos(2π(t − peak)/24h)` in `CHAT_TIMEZONE`, peak about 16:00, trough about 04:00 |
| **B — social battery** | energy for people | 0..1; drains in conversation, recovers in quiet; fast, hours |

S and B are deliberately separate. Sleep from B alone fails both ways: a
quiet server never drains her, so she never sleeps; a lively evening drains
her into sleep at 21:00 mid-sentence. The model below is a deliberately
simplified body model *inspired by* the two-process model of sleep regulation
(homeostatic pressure plus circadian rhythm). It makes no claim to biological
fidelity, and the experiment does not need any: it needs sleep to follow from
state rather than from a schedule.

**Sleepiness** = `S − k·C`.

- She falls asleep when sleepiness > θ_sleep, and wakes when it drops below
  θ_wake.
- While she is engaged in a conversation, θ_sleep is raised by a margin: a
  good evening keeps her up past one, and the next morning shows it.
- **No random jitter.** Variation comes from history — a late night, a busy
  day, a long conversation — carried in S into the next day. On a quiet
  server she sleeps at the same time every night, as people do on quiet
  weeks. Dice deciding she fell asleep at 00:42 would be noise, not
  behaviour; there is randomness enough elsewhere (interruptions, initiative,
  the model).
- Constants are fitted, not guessed: a quiet week in `bodysim` (see
  *Measuring*) must put her to sleep around 00:30 and wake her around 09:30.

**B, the social battery.**

| Event | Effect |
|---|---|
| handling a moment | −0.02 |
| each person beyond the first in the scene | −0.01 |
| the appraisal's `energy`, when set (see B4) | + `energy`, −0.1..+0.1 |
| quiet while online | recovers toward 1, half-life about 40m |
| away | half-life about 15m |
| asleep | recovers toward 1, half-life about 90m — no reset on waking, so a short night after a draining evening leaves her short the next morning |

B has physical meaning, and must keep it: talking costs, more people cost
more, quiet and sleep restore. It must not become *how much she enjoyed
recent conversations* — that is `Mood = 0.73` under another name, the thing
v1 died of. So the model's hand on it is narrow: `energy` is empty for almost
every moment, and set only for something done *to* her (B4) or a rare moment
that genuinely drains or lifts her. Whether she likes someone lives in
feelings (C) and dossiers, not in B.

### B2. Presence

```
            sleepiness < θ_wake                 B < 0.15, or life interrupts
 ASLEEP ─────────────────────► ONLINE ◄──────────────────────────────► AWAY
   ▲                             │      B > 0.5 and interruption over,     │
   │     sleepiness > θ_sleep    │      or a mention gets through          │
   └─────────────────────────────┴─────────────────────────────────────────┘
```

- **ONLINE** — behaves as v2 does.
- **AWAY** — about today, not looking. Messages are recorded; nothing is
  handled.
- **ASLEEP** — the night. Replaces the fixed 23:00–09:00 quiet hours in
  `initiative.go`.
- **Life interrupts.** Even with a full battery she steps away now and then:
  interruptions arrive at random, about one every 90 minutes online, lasting
  a lognormal time with a median of 25 minutes. Without this, a full battery
  would mean always there.
- **Never mid-exchange without a word.** An interruption that falls while
  she is engaged in a conversation waits for a lull (no exchange with her for
  a few minutes), or goes out with the one-line exit of *Going* ("brb").
  Vanishing mid-sentence reads as broken, not as busy.
- **Discord status.** ONLINE shows as online; AWAY and ASLEEP both show as
  **idle**. Never invisible — the bot's other commands keep working, and an
  offline bot reads as a broken one.

**While not online.**

- `Observe` is unchanged: the conversation is still recorded, so she has the
  context when she comes back.
- A direct approach (mention, reply, name) goes to a **missed list**,
  separate from the backend-failure deferrals, so waiting for her does not
  spend retry attempts.
- While AWAY, a mention gets through with 35% probability, after a random 2
  to 15 minutes: a phone notification. While ASLEEP, never.

**Coming back.**

- Missed approaches are handled one per person, each through the existing
  late path (`Scene.Late`; `decided()` already says to acknowledge the gap as
  a person would).
- **Paced like scrolling, not flushed.** Six answers in the first minute
  after waking is the most bot-like thing she could do. The catch-up is
  spread over minutes — a channel at a time, oldest first, a pause between
  answers as if reading back — and a fresh message arriving meanwhile is
  handled live rather than queued behind the backlog.
- The late framing applies only past about 20 minutes; three minutes late is
  just normal.
- Consider may decide something is no longer worth answering. The person who
  asked "you around?" six hours ago and left does not need a reply.
- Coming online is also an opening for initiative — she may say something on
  arrival, and usually does not.

**Going.** When B runs out or sleep arrives, about half the time she sends a
one-line exit first: a moment whose scene carries the fact "you are about to
go" — a fact about the body, stated like the time of day. Otherwise she just
goes quiet.

### B3. What the battery changes

| B | Given to her, done for her |
|---|---|
| > 0.6 | everything, as today |
| 0.3–0.6 | settle and reply delay ×1.5; transcript tail capped at 10 lines; at most 4 recalled moments; no second thoughts; no overhearing |
| 0.15–0.3 | tail of 4 lines; no recall; voice examples sampled toward the short ones; no initiative |
| < 0.15 | the session ends and she goes AWAY (see *Going*) |

None of it is said to her. She is tired in what she can take in and in when
she answers, never in an instruction to act tired.

### B4. Things done to her: `energy`

People will write `_gives her a 9v battery_`, `*hands her coffee*`, or poke
her with an emoji. What that does is a question of meaning, so it is the
model's.

- **Not a word list.** Matching "coffee" is v1: it cannot tell a gift from a
  joke, from sarcasm, from someone farming a mechanic.
- **One appraisal field:** `"energy"`, −0.1..+0.1, and empty unless something
  was done to her or the moment genuinely drained or lifted her — like
  `remember`, almost always empty. A battery from someone she likes, +0.1 and
  amused; the same from someone who has been grating, refused, empty; being
  badgered for ten minutes, −0.1.
- **Diminishing returns, in code:** the k-th positive `energy` from the same
  person within an hour is multiplied by 0.5^(k−1). The fifth coffee does
  nothing.
- **Asleep is asleep.** She sees it when she wakes.

**Reactions** reach her as facts, not as moments: the service collects
reactions to her messages (a hook next to the existing
`onMessageReactionAdd`) and the next scene in that channel says "since your
last message here: ❤️ ×2 from Big M and Ava, 😂 from Rook". No model call per
emoji.

### B5. What the prompt may see from the body

| Allowed (facts) | Not allowed |
|---|---|
| clock time, day of week | B, S, C, sleepiness, any score |
| "you woke at 11:40" | "you are tired", "you are sleepy" |
| "you have been talking here for 2 hours, with 3 people" | "keep it short", "be brief" |
| "you are about to go" | "you are low on energy" |
| "Big M wrote this 3 hours ago; you were not around" | why she was not around |

### B6. Persistence

S, B, presence state, the time of the last transition and the missed list go
in the datastore as one small record. Operational, not mental — the same line
v2 drew. Restart resumes where she was; a bot down for six hours comes back
with S and B advanced by six hours of whatever she would have been doing.

---

## C. Affect: feelings with a cause, fading on their own clock

**Problem.** `Self.Mood` is a few words that every appraisal overwrites. Her
mood is whatever the last message made it: whiplash between messages, and no
lingering after them. The model cannot fade anything between calls; it has
no sense of time.

**Design.**

- The appraisal's `feel` becomes optional structure: `"feeling": {"what":
  "stung", "about": "Big M's jab about my taste"}`, set only when something
  actually registers. Its strength is the moment's `weight`.
- `self.md` keeps a short list of **active feelings**: what, about, person,
  strength, when, and its source — the message being appraised, stamped by
  the code (I). The model names the feeling (`what`) and says in a few words
  what it is about (`about`); the person, the message and the time are
  anchored by the code from the scene, so a feeling cannot point at someone
  who was not there or something that did not happen.
- Code fades each one: `strength · exp(−age / τ)`, with τ from about 2h at
  weight 0.1 to about 24h at weight 0.9 — heavier things fade slower, as
  memories already do. Below 0.1 it is gone. At most 5; a new feeling about
  the same person and cause replaces the old one.
- The prompt sees **all** the live ones, most recent first, each with its
  age: "stung by Big M's jab about her taste (2 hours ago)". All of them,
  because a person is annoyed with one friend, excited about a thing and
  wondering about another friend at once, and that coexistence is part of
  being someone. Most recent first rather than strongest first, so the order
  does not tell the model which one should win. The model can reason about
  each — *two hours ago, and he has apologised since* — which it never could
  about v1's "tension 0.7".
- `Mood` is retired in favour of the list. Reflection clears what the day
  has settled.

**Drives are facts the code tracks and wants the model forms.** The code
counts: hours since anyone talked to her; since anything new happened (a
moment with weight ≥ 0.5); how long she has been awake. The prompt gets those
as facts. "She is bored and might poke the channel" is the model's reading of
them, never the code's.

---

## D. Perception: she does not take in everything

Arguably more important than memory: a mind that sees and remembers
perfectly is an oracle, whatever it remembers. D is three separate pieces
that land at different times (see *Order of work*).

- **Attention follows the battery** (B3): transcript tail and recall size
  shrink as she tires. She then misses the second question, or the thing
  said twenty lines up, because it was not in front of her — and fails to
  recall something that would have been useful, which is the more human
  failure than recalling something odd. Depends on the body.
- **Associative drift in recall.** `memory.Recall` returns the most relevant
  moments, so the right memory always comes back — which no one's memory
  does. With probability 0.25 the last slot is filled by a *neighbour*
  instead: a moment sharing one person or one keyword but ranked low, or a
  heavy moment from the last week. Not marked. It is where "that reminds
  me…" comes from, and where a tangent comes from. Independent of
  everything else, and cheap; a first step, not a model of memory (see
  *Deferred*).
- **Face value by default.** The thinking rules ask her to read "what they
  mean under the words" in every message, which makes her a perceptive
  therapist on every line — itself an LLM tell. Changed to: *mostly take what
  people say at face value, the way someone skimming a chat does; read into
  it when something is off.* A prompt change, so it lands with G.

---

## E. Inner life: something happens when nobody is talking

### E1. On her mind

- An **idle mind tick** every 45–90 minutes while she is awake, varied, plus
  one on waking. One cheap call on the thinking backends.
- It sees her self-description, her life (F3), active feelings, today's
  moments so far, her wants, and the drive facts. It answers with one line:
  what is on her mind right now.
- Some ticks are a walk instead (F3): she passes through a channel and the
  tick is what she took from it. That is what keeps "on her mind" about
  something that happened rather than about her own mood.
- The line lasts until the next tick, and is usually *unrelated* to whatever
  conversation she walks into. That is the point: it is what makes her
  distracted, what she brings up unprompted, what she changes the subject
  to.
- Consider and Speak both see it, as content: "On her mind: …".
- Cost: about 15 calls a day. Asleep, none.

### E2. Wants

- 1–3 current wants in `self.md`, each with why: *get someone to actually
  finish a project; find out whether Rook's game is any good; be left alone
  about the rules for a week*. Set and retired by reflection.
- Consider sees them, and an intent may serve one instead of only answering.
  A conversation is two agendas meeting; v2 has one.
- A want is one of the things an impulse can come from (H1).
- Threads (follow-ups) stay as they are. A thread is a chore; a want is a
  reason.

---

## F. Identity content: something to say

The lever expected to matter most, and the cheapest to try.

### F1. Specifics in the character card

A new heading in `character.md`, `## Specifics`: 20–40 concrete, slightly
odd, sometimes contradictory facts. Opinions with a reason; things she is bad
at; a pet peeve; words she overuses; a couple of running bits; taste in
something outside the server.

**The authoring criterion:** a line is good if it lets you predict what she
would say about a thing before she says it. Specifics *constrain* what she
generates; they do not generate content themselves. "Hates Rust because the
compiler lectures her like a hall monitor" constrains; "likes books" does
not, and is useless.

Authored by hand. It is the single most important piece of writing in v3,
and a model writing it would produce the median again. A version of her with
excellent specifics and no body at all is expected to read as more of a
person than one with every mechanism below and generic specifics; the
additive ablation (see *Measuring*) tests exactly that.

### F2. Self-facts she has stated

What she says about herself becomes true of her. A new file per guild,
`me.md`: dated facts she has stated about herself — "hates Rust — told Big M,
21 Sep".

- Extracted **at reflection**, and **only from messages she actually sent**
  — the "I said" lines, with the words quoted. Never from an intent, an
  appraisal, her life, or an earlier reflection. No extra call per message;
  within the day, the transcript and recall already carry it.
- Each carries its source message (I). Reflection names which of the day's
  lines a fact came from; the code checks that line exists and is one of
  hers, and drops the fact otherwise.
- At most 40. When a new one contradicts an old one, reflection decides:
  she changed her mind (keep the new, note the change) or she was wrong (keep
  the old). When one contradicts the card (F1), the card stays canon, the
  statement stays as history, and the conflict is flagged for the author
  (I).
- Consider and Speak see the ones that match the conversation's words or
  people, at most 8 (see *Lifetimes*).

v2 keeps her *relationships* consistent. This keeps *her* consistent.

### F3. A life in the town

The server is a town. Today she lives in the few rooms she has been let into
and knows nothing of the streets between them, so nothing reaches her that
nobody aimed at her. v3 lets her walk.

**A third channel mode.** `/chat channel` gains `reads`, beside `off`,
`answers` and `speaks-first`: she sees the channel and remembers the gist,
and never speaks there — not a reply, not a reaction, not a tag.

- Opted in per channel by an administrator, like the other modes, and
  listed in `/chat status`, so people know where she reads.
- **The v2 gate does not move.** A channel in no mode stays as it is today:
  the only thing taken from it is that an opted-in person was around, never
  what they said — even when the bot can technically see it.
- Channels people use to vent, confide or ask for help are the wrong
  channels to opt in. The command's help text says so.

**This is a hypothesis with a named risk**, and the one most likely to go
wrong socially. "The bot may read this channel" is one thing; "the bot
brought up something it saw there in another conversation" feels far more
like someone — and far more like being watched.

- *Hypothesis:* cross-room observation increases perceived continuity.
- *Risk:* it increases perceived surveillance.
- *Measure:* the blind reading asks both — "felt like a person" and "felt
  like it was monitoring the server" — with walks on and off.

**Walks.** Some idle-mind ticks (E1) are a walk: she passes through one
`reads` channel, picked at random and weighted towards those with something
new since her last pass.

- The code hands the thinking backends the lines since her last walk there,
  at most about 30, rewritten by `chat.plain` like everything else.
- One call answers with what caught her, if anything: *passed through #art —
  someone posted a dragon sketch, the wings are wrong*. Usually one or two
  things; often nothing.
- What caught her is written as a light moment, weight at most 0.3, so it
  fades within a day or two unless it is recalled — the way passing
  something in the street does.
- Impressions, not transcripts. The moment is her gist; the lines are not
  kept.

**Using what she saw.**

- She can bring it up the way someone who happened to be in the room would:
  "saw the dragon sketch in #art" — or, to the person who made it, "the
  wings on your dragon are wrong".
- **No talking about people behind their backs.** An opinion about someone's
  work or words — anything that is a judgement of them — goes to that person
  or nowhere. To anyone else she mentions what she saw only neutrally. A bot
  that critiques one member's post to another, in a room the first is not
  in, is how drama starts, and a sharper form of feeling watched. This is a
  hard limit in the voice and thinking rules, like the card's `Avoid`, not a
  judgement the code makes.
- She never quotes someone from a `reads` channel, and never tags someone
  because of what she saw there. The mention rails already limit whom she
  can notify; a walk adds no one to that list.
- Recall treats walk moments like any others, so the associative drift (D)
  is how most of them resurface: sideways, hours later, in a different room.

**Her own life.** `self.md` gains a `## Life` section: 2–4 ongoing things in
her days — what has been going on in the town as she sees it, a thing she
keeps noticing, something that has been annoying her. Seeded from the card,
advanced at reflection **only from observed moments** — her walks and her
conversations with other people — each item citing the moments it rests on
(I). Never from self-facts and never from earlier Life: that is the loop by
which something she once made up becomes her history. Kept consistent with
F1 and F2.

This needs one rule rewritten. v2's thinking rules say she *"does not invent
channels, rules, events or facts about people"*. v3 keeps the substance and
widens the ground she stands on: *she does not invent events or facts, in the
server or outside it; what is written about her — her specifics, what she has
said about herself, her life, what she has seen around the server — is true
and hers to draw on.*

An outside world is deferred, deliberately; see *Deferred*.

---

## G. Voice

- **Examples, many and mixed.** Ten examples, all a dry two-clause quip in
  answer to someone addressing her, teach one template. The card grows to
  about 30, deliberately including the dull and ordinary: "wait what", "which
  one", "ok fair", an honest "no idea", a question back, half an answer, a
  long excited paragraph, a tangent about her own thing, group-chat exchanges
  with several speakers. Each call samples 8 at random; when B is low the
  sample leans short (B3). Examples remain what they are in v2: voice, not
  rules, replayed as turns to the voice only.
- **The voice stops receiving the appraisal's reading.** `decided()` passes
  `Read` and `Feel`, which turns the voice into a renderer of an
  emotionally-correct gist. v3 passes the intent, loosely — "roughly what you
  want to get across" — and not the reading.
- **The voice gets the specifics.** Today it sees only the "who" and
  "between" paragraphs. v3 adds the last 3 notes on the person, the top 3
  recalled moments, what is on her mind, and her specifics and self-facts.
  Details are what make a reply read as someone's.

---

## H. Initiative: speaking because she has something

**Problem.** Every opening `openings()` finds is a timer on an absence: a
thread come due, someone who has not talked to her in six hours, a room
silent for ninety minutes. The code finds a gap first and only then asks the
model to fill it, so the reason to speak comes from the clock, and she has to
make up something to say on the spot. The result is the most bot-like line
there is — "quiet in here today".

People do not start things that way. They speak when they have something:
they saw something, thought of someone, remembered something, want
something. And most of what people do unprompted on Discord is not starting
from silence at all; it is joining a conversation already going, or reacting
to something.

**The turn.** An impulse comes from something she has; the openings become
the rails an impulse must pass, not the source of it.

### H1. Impulses come from her inner life

- The idle mind (E1), and a walk (F3), may answer with an impulse as well as
  what is on her mind, usually empty:
  `"impulse": {"to": "Rook" | "a room" | "", "about": "the dragon sketch in #art — the wings"}`.
- Wants (E2), due threads, a feeling about someone (C), and something from a
  walk are all things an impulse can come from.
- The code checks the impulse against the rails — consent, the daily cap,
  the gap between starts, two unanswered reaches, presence, battery — and if
  it passes, it goes to Speak with its reason. The reason is written to
  memory with what she says, as v2 already does.
- `lookAround` stops generating openings and keeps only the rails. Threads
  come due into the idle mind rather than being an opening on their own.
- **A quiet room on its own is no longer an opening.** She speaks into a
  quiet room only when she has something to put there.

### H2. Follow-ups when the person shows up

- When someone posts in any channel she answers or speaks in, their due
  threads become an opening at once — event-driven, not polled.
- No tag and no consent question: she is joining a room they are in, not
  going after them from elsewhere. It goes through Consider as a moment of
  its own ("Rook is here; you meant to ask how the naming went"), so she can
  still decide the moment is wrong.
- Reaching someone who is away (`TriggerReach`, `/attention` consent) stays,
  and becomes rare: for what will not keep.

### H3. Joining out of interest, not by chance

Overhearing today is a 30% roll at most once every ten minutes per channel,
so she joins conversations at random and never the ones she would care
about.

- The code filters for **attention**, not meaning. A remark gets her
  attention when it touches her specifics, self-facts, wants or open
  threads (keywords, the way recall already matches), involves someone her
  dossier is warm about, or connects to something from a recent walk.
- Only then is Consider asked, and it can still stay out of it.
- No random roll in the core design: what she has on her mind, what she
  remembers and what happened recently are the variation, and a dice roll
  for "people are spontaneous" is exactly the kind of simulated trait this
  spec rejects. A small baseline for serendipitous joining exists only as
  its own experimental switch, off by default (`CHAT_SERENDIPITY`).

Keywords are acceptable here where v1's word lists were not, because they
decide only what she *notices*. What it means, and whether to join, stays
with the model.

### H4. More than starting a message

| Form | How | Why |
|---|---|---|
| **reacting to other people's posts** | an impulse may be a reaction on someone's message in a channel she answers in; thinking call only, no voice | low stakes and very human; shows she is around without talking |
| **picking up an older line** | a Discord reply to a message from earlier: "wait, going back to this" | people do it all the time and bots never do; the message id is kept, so the code anchors it |
| **bringing something back** | recall plus an impulse: "been thinking about what you said about the city thing" | memory shown as something that happened between them, not as retrieval |
| **arriving and going** | tied to presence (B2): now and then a line on coming online or leaving | timed by the body; not every time |

### H5. Learning whether she was welcome

- The journal records what became of each thing she started: **answered,
  reacted to, or ignored**, and after how long.
- That outcome is a fact for reflection: "I brought up the sketch with Rook;
  he ran with it", "I spoke up in #general and nobody answered". Reflection
  folds it into the dossiers and her self-description, so over time she
  learns who likes being approached, and when.
- No rate is shown to her. The code's stop after two unanswered reaches
  stays as the safety net.
- **Ignored against the base rate.** On a small server most messages from
  anyone go unanswered. Told only that she was ignored, reflection concludes
  nobody wants her, she starts less, learns from even less, and goes quiet —
  a one-way spiral of the same shape as v1's tension. So reflection also gets
  the base rate as a fact, per channel: "in #general yesterday, 14 of 20
  messages got no reply from anyone". Being ignored then reads as the room,
  not as her.

### H6. Rails

| Rail | v3 |
|---|---|
| daily cap, gap between starts, `/attention` consent, two unanswered reaches | unchanged |
| quiet hours 23:00–09:00 | replaced by ASLEEP |
| presence and battery | ONLINE only; B > 0.3 |
| reaches while she is asleep | never — nothing is queued to arrive at wake-up either |
| the quiet-room opening | removed |
| reactions she starts | count towards the daily cap at a third of a start each; never in a `reads` channel |

---

## I. Provenance: she knows how she came to know things

**Problem.** In v2 the model writes straight into memory: an appraisal's
note lands in a dossier, reflection rewrites the dossier, the self-description
and the threads. v3 adds self-facts, life, feelings and wants, all written by
the model and all read back by it — including by the reflection that wrote
them. That is a loop:

```
she says something → self-fact → reflection → life → next prompt
      ↑                                                      │
      └──────────── she says something consistent ◄──────────┘
```

and through it a detail she made up once can harden into a history. The v2
safeguards (dedup, the cap, reflection's own judgement) do not break the
loop; they only slow it. This is the second LLM-written character card
forming beside the authored one.

**Design.**

**Every persistent item carries its provenance**, stamped by the code:

| Kind | Source kind | Source | Written by |
|---|---|---|---|
| specifics (F1) | `authored` | `character.md` | the author |
| a walk moment, a conversation moment | `observed` | channel, message ids, time | code, from what happened |
| life (F3) | `observed` | the moments it cites | reflection, validated |
| self-fact (F2) | `stated` | the message of hers it comes from | reflection, validated |
| a note on someone | `stated` | the message of theirs it comes from | appraisal, stamped |
| feeling (C) | `interpreted` | the message being appraised | appraisal, stamped |
| want (E2), between-paragraph, who-paragraph | `interpreted` | the day it was set | reflection |

**The code stamps; the model does not cite.** A feeling is born in the
appraisal of one message, so the code knows its source and attaches it; the
model is never asked "which event was it", because a free-text answer cannot
be checked. Where the model must point at a source — reflection deriving a
self-fact or a line of life — it points by the number the line was listed
under, the way reflection already refers to people, and the code checks the
number is real, of the right kind (a self-fact must cite one of *her*
messages; life must cite observed moments), and drops the item if not.

**Rank.** `authored > observed > stated > interpreted`. A lower kind never
overwrites a higher one silently.

**Invariant: derived information never gains strength.** Anything derived
from something else is at most as strong as the weakest thing it rests on,
and is usually `interpreted`. `observed → interpreted`, `stated →
interpreted` and `interpreted → interpreted` are allowed; `interpreted →
stated` and `interpreted → observed` never are. A line of life is `observed`
only while it keeps its observed sources; lose them and it is gone, not
demoted into something she simply knows. `mind.Commit` enforces this, and it
is what an audit of the files checks first.

**Conflicts with the card.** The card is canon; what she said is history. A
self-fact contradicting the card never rewrites it — but it is not hidden
either, because hiding something she said breaks v2's founding rule that she
owns her words.

- Both stay, and she sees both when the topic comes up: the canon as part of
  who she is, the statement as something she has said ("you've also said
  Rust has grown on you"). The model reconciles them the way a person does —
  changed her mind, was in a mood, was joking.
- The conflict is **flagged for the author**: `/chat status` lists open
  conflicts, and they are kept in their own section of `me.md`. The author
  resolves it by changing the card deliberately, or by marking the statement
  a slip. For an experiment, which of the two it was matters, and the
  contradictions become part of her history instead of corruption.

**Interpretations lose to observations.** A "between" line saying Rook never
talks to her does not survive a day of moments with Rook.

**Breaking the loop, by rule rather than by ranking alone.**

- Self-facts come only from messages she sent — never from intents,
  appraisals, life or earlier reflection.
- Life cites only observed moments — never self-facts, never earlier life.
- Nothing reflection writes may be the only source of something else
  reflection writes. An interpretation can lean on observations and
  statements; it cannot lean only on another interpretation.

**No migration.** v2's memory comes from solo testing in a private room and
is not carried over. v3 starts with an empty memory directory, so every item
in it has provenance from its first write, and there is no `legacy` kind to
rank, expire or explain.

**Proposals, then commits.** Appraisal, idle mind and reflection all return
proposals. One place in the code — `mind.Commit` — validates each (the
person exists and is in the scene or the day; the source is real and of the
right kind; the number is in range; it is not a duplicate; it does not
overwrite a higher kind), stamps it and writes it. Everything that is
dropped is logged with why, so the rejection rate is itself a measurement:
a model whose proposals are often refused is one inventing things.

**What the prompt sees.** Provenance is for the code and for the person
reading the files. The model sees content, not source kinds — with one
exception: when an interpretation is shown next to what it rests on, it may
say so in plain words ("you took it as…"), because *she thinks he meant it*
and *he said it* are different things to reason about.

**What it gives her.** The character acquires an epistemology: she does not
merely know things; the system knows how she came to know each one. That is
stronger than any prompt instruction not to make things up.

### Lifetimes: persistence is not visibility

Creation and provenance are only half of memory; the other half is when
things stop mattering. Without explicit lifetimes, persistence becomes
accumulation, and accumulation ends up in the prompt. A person keeps far more
than they have in mind at once.

Two separate questions for every kind of state: **when does it leave the
prompt**, and **when does it leave the files**.

| State | Leaves the prompt | Leaves the files |
|---|---|---|
| feeling | when its faded strength drops below 0.1 | then too; reflection may fold it into a "between" line first |
| on her mind | at the next idle tick | not kept; the moment it came from is |
| want | when reflection retires it, or after 14 days without being acted on or mentioned | at retirement; the day file records that she let it go |
| life item | when none of its sources is recallable any more, or after 10 days not advanced | then; the day summaries keep the gist |
| self-fact | shown only when it matches the conversation's words or people, at most 8 at once | when reflection marks it superseded — kept with the note "changed her mind", never shown again as current |
| specifics (card) | always, while the section fits its budget; beyond it, matched like self-facts | never; authored |
| dossier notes | last 6 (as v2) | after the last 20 (as v2) |
| dossier paragraphs, kept moments | always, for people in the scene | never; rewritten, not deleted |
| thread | when done, or 7 days past due | when closed |
| moment in a day file | when recall no longer reaches it (a fortnight light, three months heavy, as v2) | never; about a megabyte a year |
| walk moment | as a moment, but weight ≤ 0.3, so a day or two | never |
| card conflict | while open, in `/chat status`; both sides visible to her (above) | when the author resolves it |

The rule behind the table: **everything that can be shown has a way to stop
being shown, and a reason in the table.** A new kind of state is not added
without its row.

---

## How it fits the existing rails

| Rail (v2) | v3 |
|---|---|
| `overrule`: never ignore the same person's direct approach twice | ONLINE only. Silence while away reads as away, not broken. |
| settle before reading a burst | unchanged; stretched ×1.5 when B < 0.6 |
| deferrals for backend failures | unchanged; the missed list is separate |
| second thoughts | ONLINE and B > 0.6 only; dropped when she leaves |
| overhearing | ONLINE and B > 0.6 only |
| channel modes and the gate in `Observe` | `reads` added; a channel in no mode is still closed to her |
| mention rails | unchanged; nothing seen on a walk makes someone taggable |
| initiative | reworked; its own rails in H6 |
| echo, repeat, control-word checks; mention limits; `Casual` | unchanged |
| writes to memory (`Absorb`, reflection) | through `mind.Commit`: validated, stamped, ranked (I) |
| `/chat why` | new outcomes: "she was away" (with when she saw it), "asleep"; for something she started, the impulse it came from and how it was received |
| `/chat status` | shows presence, today's wake time, battery as a bar, active feelings, on her mind, wants, open provenance conflicts |

## Where things live

| What | Where | Why |
|---|---|---|
| S, B, presence, missed list, reaction tallies | datastore | operational, and bot-wide |
| `reads` channels, last walk per channel | datastore | operational, next to the other channel modes |
| what she took from a walk | day file, as a moment | memory like any other |
| backend order and on/off | datastore | operational, bot-wide |
| what became of each thing she started | journal | already where initiatives are recorded |
| feelings, on her mind, wants, life | `self.md` per guild | prose the model reads and writes |
| self-facts | `me.md` per guild | prose; grows and is pruned by reflection |
| specifics | `character.md` | authored |
| provenance of each item | alongside the item, in the same file | a person reading `self.md` should see where a line came from without another file |
| conflicts with the card | `me.md` per guild, under *In conflict with the card* | beside the facts they are; the author resolves one by changing the card or removing the line |
| dropped proposals | log, and the journal entry of the moment | the rejection rate is a measurement |

## Switches

Every part can be turned off on its own, so each can be measured by taking
it away — except provenance (I), which is not a feature but how memory is
written, and has no switch.

| Variable | Default | Turns off |
|---|---|---|
| `CHAT_BACKEND_ORDER` | `priority` | `score` restores v2 ranking |
| `CHAT_VOICE_BACKENDS` | empty | voice uses the full list |
| `CHAT_BODY` | `on` | presence, sleep and battery: always online, as v2 |
| `CHAT_FEELINGS` | `on` | `Mood` as in v2 |
| `CHAT_IDLE_MIND` | `on` | no idle tick, no "on her mind" |
| `CHAT_WALKS` | `on` | no walks; `reads` channels are ignored |
| `CHAT_DRIFT` | `0.25` | `0` turns off associative recall |
| `CHAT_SELF_FACTS` | `on` | no `me.md` |
| `CHAT_EXAMPLES_SAMPLE` | `8` | `0` sends all examples, as v2 |
| `CHAT_IMPULSES` | `on` | v2's timer openings, quiet room included |
| `CHAT_FOLLOWUP_ON_SIGHT` | `on` | due threads wait for the timer |
| `CHAT_INTEREST` | `on` | overhearing is the v2 random roll |
| `CHAT_SERENDIPITY` | `0` | a probability of joining without an interest match; experimental, off in the core design |
| `CHAT_REACT_FIRST` | `on` | she reacts only to what is said to her |

---

## Measuring

The question is not "is it better" but "which part moves which signal".

### Tools

- **chatprobe** gains a simulated clock (the log's own timestamps drive the
  body), `-voice-backends`, and the switches above as flags. The v1
  production log stays the regression scenario: v3 must still own her words,
  soften to an apology and never post a control word.
- **bodysim**, `cmd/bodysim`: runs the body alone — no model — over a
  synthetic week of activity (quiet days, a busy evening, a burst of gifts)
  and prints a timeline of state, S, B and sleepiness. Presence constants
  are fitted here, cheaply, and a unit test pins the quiet-week targets
  (asleep about 00:30, awake about 09:30, 3–6 online stretches a day).

### Metrics

| Signal | Metric | Expected to move with |
|---|---|---|
| template lock | share of her replies whose first two words match an earlier reply's; distinct openings per 50 replies | G (examples) |
| behavioural lock | an offline judge labels each moment's stimulus class (compliment, jab, question, apology, gift…) and her response class (deflecting joke, sincere answer, question back, tease, ignore…); for each stimulus class, the entropy of her responses. Surface metrics can improve while every compliment still meets a deflecting joke; this is the metric that catches it | F, G, C, D |
| monotony | coefficient of variation of reply length | G, B3 |
| specificity | share of replies naming something from specifics, self-facts, life, notes or recall | F, G |
| self-consistency | contradictions against `me.md`, checked by an offline judge model over a week of transcript | F2 |
| substrate drift | the voice metrics above, per backend, same log | A |
| inner life | share of replies or initiatives that draw on "on her mind" or a want; share that change the subject | E |
| the town | share of replies and initiatives drawing on a walk; how long after the walk | F3, D |
| the town's rails | zero quotes from, and zero tags because of, a `reads` channel — a test, not a metric | F3 |
| welcome | share of what she started that was answered or reacted to, and how fast; by form (H4) and by source of the impulse | H |
| grounded initiative | share of starts whose reason names something specific (a person, a thread, a walk, a want) rather than a silence | H1 |
| joining | share of the conversations she joined that she stayed in for more than one exchange | H3 |
| presence realism | distribution of reply latency; missed approaches answered on return vs let go | B |
| invention | share of proposals `mind.Commit` refuses, by kind and by backend | I |
| **causal carryover** | for each reply, an offline judge classifies what it draws on: the current message, the recent conversation, an older memory, an active feeling, on her mind, a want, a walk, a self-fact, a specific. Measured: the share of replies containing anything that existed before the message arrived, and the share drawing on more than one independent source. The most direct test of the thesis — a reply should come from a mind with state, not from the last message | all, E and F most |
| drift | for replies that used a drifted memory: the share where the connection was useful or plausible, and the share where it produced a made-up link between two unrelated moments. Drift is kept only if the first clearly outweighs the second | D |
| card conflicts | self-facts that contradicted the card, per week, and how each was resolved | I, F |

### Blind reading

The deciding test is people. The same log, replayed through different
configurations, shown side by side without labels, one reader at a time.
Readers answer two questions, not one: *which of these is someone?* and, for
configurations with walks, *which of these felt like it was monitoring the
server?* Human transcripts from the server are used as a baseline only with
the consent of the people in them.

### Ablation, both ways

The question the experiment can answer, and the one worth answering: **does
the sense of a person come mainly from identity, perception, continuity,
inner life, initiative, the body, or affect?** The features interact, so it
is asked from both ends.

**Additive** — what each part does alone on top of v2:

| Configuration | Adds |
|---|---|
| v2 | — |
| v2 + A | backends pinned; the baseline every other row is read against |
| v2 + A + F/G | identity and voice |
| v2 + A + D | perception limits |
| v2 + A + B | body |
| v2 + A + E/F3 | inner life and the town |
| v2 + A + H | initiative |

A is in every row because without it every comparison is between different
models rather than different designs.

**Interaction** — the hypothesis is not really that the parts add up. It is
that *a stable identity becomes legible as a mind when it meets constrained
perception and persistent inner state*. So three combinations are read
too, chosen for the synergy they would expose:

| Configuration | Tests |
|---|---|
| v2 + A + F/G + D | does identity pay off more when she cannot see everything? |
| v2 + A + F/G + E/F3 | does identity pay off more with a life to draw on? |
| v3 without B | the whole mind without the body: is the body carrying anything? |

**Subtractive** — what each part is still needed for with everything else
present: v3 full, then v3 with one switch off at a time.

Blind reading by people is the expensive part, so the matrix stays at these
rows and no more; the metrics above run on all of them for free. The prediction on
record, to be proved wrong: identity first, perception second, then
continuity, inner life and initiative, with the body and affect last.

Presence and latency only show in a transcript that carries timestamps, so
the reading copies keep them, and chatprobe's simulated clock produces them.

---

## Order of work

Chosen so each step can be measured before the next one muddies it.

1. **Backends (A).** Small and separate, and every later measurement is
   noise until the voice comes from a known model. Rank the relays on voice
   with chatprobe; set the voice order.
2. **Provenance, then identity and voice (I, F, G, face value from D).**
   `mind.Commit` first, and v2's existing writes moved behind it, because
   F2 is the first thing that writes a new kind of memory and must not be
   written the old way. Then all prompt-side and measurable with chatprobe
   alone on a real log: specifics, self-facts, the rule change, examples and
   sampling, the voice changes, face value. Expected to be the largest
   single gain.
3. **Initiative that needs no inner life, and drift (H2, H3, H5, drift from
   D).** Follow-ups when the person shows up, joining out of interest, and
   recording how what she started was received — all three stand on what
   exists today, and H5 starts collecting the welcome data every later step
   is judged by. Associative drift is independent and cheap, and rides
   along.
4. **Body, with attention (B1–B3, B5, B6, attention from D).** bodysim
   first, then the service: presence, missed list and catch-up, Discord
   status, battery limits — and the attention limits, which are driven by
   the battery and cannot come before it. Voice stickiness moves from
   `engagedWindow` to presence sessions here.
5. **Energy and reactions (B4).**
6. **Affect (C).** Feelings with fading; drive facts.
7. **Inner life, the town, and the rest of initiative (E, F3, H1, H4, H6).**
   Idle mind, wants, the `reads` mode and walks, life; then impulses replace
   the timer openings, and the new forms of starting something. Walks and
   impulses ride on the idle tick, so they come with it; `## Life` needs
   walks to have anything to cite.

Perception limits come as early as their dependencies allow — face value in
step 2, drift in 3, attention in 4 — ahead of affect and inner life. The bet
is that *specific identity + limited perception + persistent consequences +
independent causes* is the core, and sleep, feelings, wants and walks are
built on top of it, not instead of it.

## Open questions and risks

- **Calls on free relays.** v3 adds about 15 idle calls a day and nothing per
  message. Relay rate limits are the constraint to watch, not cost.
- **Timezones.** One body, one clock: people in other timezones will find
  her asleep in their evening. Acceptable for one community; per-guild
  bodies are the fallback if it is not.
- **Eight hours asleep is a long silence** for a server that is used to her
  answering. The idle status and the late answers on waking are meant to make
  it read as a person sleeping; the blind reading will say whether it does.
- **Tamagotchi risk.** If gifts visibly work every time, people will play
  her like a pet. Diminishing returns, the model's right to refuse, and never
  stating the battery are the defence. Watch for farming in the logs.
- **A hallucination achieving persistence.** Something she makes up once
  becoming permanent, through self-facts, life and reflection feeding each
  other. Provenance (I) is the defence: sources stamped by code, the rank,
  and the rules that break the loop. The invention metric shows whether it
  holds; `me.md` and `self.md` stay readable and editable like the dossiers.
- **Life drifting off-character.** Reflection advancing her life a little
  each night could wander. F1 is the anchor, and life may cite only observed
  moments; check `self.md` weekly early on.
- **The battery becoming a mood.** If `energy` is set on more than a small
  share of moments, B is tracking how she feels about people rather than
  how much talking she has done. Log the share; if it creeps up, narrow the
  field again.
- **Being watched.** Even opted in and announced, a bot that brings up what
  was said in another room can read as surveillance rather than as a
  neighbour. Light weights, gist only, no quotes and no tags are the defence;
  the blind reading and the server's own reaction will say whether it is
  enough. The switch turns walks off without touching anything else.
- **A quiet town.** On a small server a walk will often find nothing new.
  That is honest — and a reason the outside world may be needed after all.
- **Too forward, or too shy.** Impulses come from a model that has been told
  most of the time the answer is nothing; she may barely start anything, or,
  with a rich inner life in front of her, start too much. The daily cap bounds
  the second; the welcome metric shows both.
- **Interest keywords drifting into v1.** H3's filter decides only what she
  notices. The day it starts deciding what she does, it is v1's word lists
  again.
- **Follow-up on sight in front of others.** "How did the interview go" is a
  different question in a busy channel than in a quiet one. Consider sees
  the room and can wait; watch the first weeks for questions that should
  have been private.
- **Feelings vs reflection.** Both touch how she feels; reflection must fold
  feelings in and clear the settled ones, not keep a second copy.
- **Authoring.** F1 and G are writing, not code, and they are what the
  experiment most depends on. Budget time for them.

## Known failure points

From the last review before building. None of them changes the architecture;
each is a gap filled, a rule added, or a thing watched.

**Settled before or while building.**

| Failure | Settled by |
|---|---|
| **Reflection as a single point of failure.** v3 asks one call for the day summary, people, self-facts with citations, life with citations, wants, feelings folded in and conflicts — seven jobs in one JSON on a free relay — and `chat.Service` gives up on a day after a few attempts, losing all of it. | Reflection is split into three calls: *the day and the people*; *self-facts*; *life and wants*. Each is retried and fails on its own, so a bad answer costs one part of a day. |
| **No total prompt budget.** Lifetimes cap each piece, but nothing caps the sum, and the free relays refuse or time out on long prompts — anonymously, Pollinations already refuses a 4 KB one (see `ai/setup.go`). | One character budget per call, and a fixed order of what gives way when it is exceeded: drifted recall, then older recall, then life, then self-facts beyond the best matches, then specifics beyond their budget, then dossier notes. Persona, the live transcript and the person being answered never give way. Every trim is logged. |
| **Timezone.** `CHAT_TIMEZONE` empty means UTC, and a circadian clock in UTC puts her to sleep at the wrong end of the community's day. | With `CHAT_BODY=on`, the service refuses to start without a timezone. |
| **Irreproducible bugs.** Relays are non-deterministic, and v3 adds randomness in code. | Code-side randomness comes from one seeded source. chatprobe gains a record/replay cache of model answers, so a run can be repeated exactly. |

**Rules added** (in the sections they belong to): no judging people behind
their backs (F3); a paced catch-up on waking (B2); no interruption
mid-exchange without a word (B2); being ignored read against the base rate
(H5).

**Watched.**

| Failure | Watched by |
|---|---|
| **Relationship inflation.** Models drift agreeable, and reflection lets good faith become friendship; two weeks in, everyone could be close to her, and "warms up slowly" is gone. | The distribution of "between" lines and feelings per person over time. If it only ever rises, the card's slow warmth is not holding. |
| **Jokes and play hardening into self-facts.** "I'm basically a toaster", or a line from inside a roleplay scene — likely on this server — extracted as sincere. `_gives her a potion that makes her obey_` is the adversarial form. | The self-fact extraction call is told to skip what was said in jest or in play; conflicts with the card catch part of the rest; `me.md` is read by hand weekly at first. |
| **"sorry, was asleep" every morning.** The late path fires daily, and the repeat check looks only at recent turns. | The behavioural-lock metric; the first week's mornings read by hand. |
| **Withdrawal despite the base rate.** If she still stops starting things on a quiet server. | The welcome metric per week, and the count of starts per day. |

**Measurement risks.**

- **Few readers, one of them the author.** Blind readings on a small server
  will not reach significance. They are qualitative evidence, and the author
  does not read configurations they can recognise.
- **A model judging a model.** Carryover, behavioural lock and drift use a
  judge model. The judge model and its prompts are fixed for the whole
  experiment, and a sample of its labels is checked by hand.

**Known limitation: one body, several minds.** Presence and battery are
bot-wide; `self.md` — on her mind, feelings, wants — is per guild. In several
servers she has one body and several minds, and the idle tick's cost
multiplies by the number of guilds. Accepted for one community; recorded so
it is not mistaken for a design choice.

## Deferred

Considered and set aside for now, to keep the first experiment inside the
server. Written down so the reasoning is not lost.

- **Life sources from outside.** The same shape as a walk — a fixed source,
  read on the idle tick, summarised by the thinking call into a light
  moment — pointed at the world:
  - *weather in a home city* (Open-Meteo, no key): mundane, true, changes
    daily, makes her somewhere;
  - *a book she reads through* (Project Gutenberg), a chapter a day: a long
    arc with real content she cannot make up;
  - *a few subreddits* matched to her specifics, never r/popular: the
    breadth is the risk, the narrowness is the value. Posts without top
    comments, so the opinion is hers and not the thread's; `over_18` and
    NSFW subreddits filtered in code. API access and the data terms need
    checking from the real host before any of it is built.
  - General news, rejected rather than deferred: it makes her a feed, and
    every day a choice between no opinion and a risky one.
- **Sharing links.** "saw this, thought of you" is one of the most human
  things done on Discord. A model invents URLs, so the code would keep the
  real permalink behind a short handle (`[r2]`), the model would name the
  handle, and the service would substitute the link on send; any URL the
  model wrote itself is dropped.
- **Untrusted input** is the rule for all of it: fixed sources, no link
  following, raw text never reaches the voice. The same rule applies to
  walks today, since a channel's lines are other people's text too.
- **Recall as activation, not ranking.** D's drift is a cheap first step.
  Human recall is closer to a cue activating many memories that compete,
  where the wrong one sometimes wins and the useful one sometimes does not
  come at all. A later model would score each memory on relevance, recency,
  emotional weight, association with the people present and current
  attention, add interference, and let them compete. Not before the 0.25
  experiment has shown whether drift is noticed at all.

## Non-goals

- Claiming or pursuing consciousness. The aim is the signals from which
  people infer a mind, measured by whether they do.
- Any score, dial or level in a prompt.
- Invented typos, or any imperfection asked of the model rather than
  produced by what it is given.
- Changing the Discord plumbing, the rails listed above, or the privacy rule
  that no Discord id reaches a model.

## Change log

Architecture changes after the freeze, with the evidence for each.

- 21 Sep 2026 — frozen.
- 22 Sep 2026 — last review before building; gaps filled without changing
  the architecture: *Known failure points* added; rules added to B2 (paced
  catch-up, no silent interruption mid-exchange), F3 (no judging people
  behind their backs) and H5 (the base rate); reflection split into three
  calls; a total prompt budget; the timezone required with the body on; no
  migration of v2 memory (I).
- 22 Sep 2026 — A, while building: the voice order falls back to the rest
  of the list when every preferred voice backend is down, instead of
  leaving her silent. Reliability over a pure voice; the fallback is visible
  in the journal's backend field.
- 22 Sep 2026 — step 2, while building: conflicts with the card live in
  their own section of `me.md` rather than a separate `conflicts.md`, so a
  conflict and the fact it is stay one line in one file. Reflection is two
  calls for now — the day and the people; self-facts — and the third, life
  and wants, lands with step 7. The prompt budget covers the appraisal and
  the voice; initiative, a call every few hours at most, is not trimmed yet.
