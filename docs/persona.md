# The persona, v2

This replaces the design described in earlier versions of
[architecture.md](architecture.md#the-chat-persona). The Discord plumbing
around it — two opt-in gates, deferred answers, the backfill, typing, the
mention rules — is unchanged; what changed is where the thinking happens.

## Why v1 was retired

v1 took the premise that the language model is a speech cortex and nothing
else: every judgement lived in Go as a number (closeness, tension, welcome,
drives, fatigue, salience) or a word list (address, closers, reception,
brush-offs), and the model was handed the result as directives — "you are
short with Big M". About sixteen thousand lines of it.

The production log that ended it (September 2026) showed the same failures
over and over, and they share one root:

| What happened | Why |
|---|---|
| "keep it professional" about ten times; no apology or "I mean it" ever softened her | Tension rose on *countable* events (double pings) and nothing countable lowers it. An apology is not a word-list match. |
| She disowned her own supportive replies from that afternoon — "those don't sound like me" — and dug in | Her self was a persona plus numbers. She had no memory of *having said* anything, or why, so text that did not fit the persona read as someone else's. |
| A hostile ping out of nowhere, thirteen hours later | Initiative fired on residual tension with no concrete reason attached. |
| "i tagged you because you were mentioned" | The reason for reaching out lived in one prompt and was gone the next turn. |
| "SKIP" posted to the channel; "#help", a channel that does not exist | Control words and rules in the prompt lose to the model. |
| "morning" three times | The repeat check exempts short lines. |

Numbers turned into directives become a caricature: a small model reads
"dominance 0.8, you are short with him" as "be cold in every line". Word
lists cannot read pragmatics — apology, irony, good faith — which is exactly
what makes an exchange feel human. And the thing that makes a person
consistent is not stable chemistry but remembering what they said, why, and
what they want.

## The principle

**The model is the mind; the code is the body and the clock.**

- The model interprets, appraises, decides and remembers — in prose.
- The code keeps time, stores and retrieves memory, enforces rhythm and
  safety limits, and checks the output before it reaches the channel.

This is the shape of the published work on believable agents: a memory
stream with retrieval, periodic reflection and plans (Park et al.,
*Generative Agents*, 2023); memory the agent edits itself as text rather than
fields (MemGPT / Letta); appraisal done by the model against the agent's own
goals rather than by a fixed table (OCC-style appraisal in LLM agents).

## One moment, two calls

Every time something reaches her, two calls are made.

**1. Consider** (`mind.Mind.Consider`) — private, low temperature, JSON out.
It sees the persona, her current self-description and mood, a dossier on
each person present, a handful of recalled memories — including what *she*
said and meant — her open intentions, and the transcript. It answers:

```json
{
  "read":     "what they mean or want, reading between the lines",
  "feel":     "how it lands with her",
  "toward":   "how she feels about them now",
  "mood":     "her mood after this, a few words",
  "act":      "reply | react | ignore",
  "emoji":    "for act=react",
  "intent":   "for act=reply: the gist of what she wants to get across",
  "note":     "a new fact about the speaker worth keeping",
  "between":  "how things stand between them now, if it changed",
  "remember": "something from this moment worth remembering",
  "later":    "something she means to follow up on", "later_hours": 24,
  "weight":   "0..1, how much it got to her",
  "back_off": "true if they asked her to leave them alone"
}
```

The note, the relationship line, the memory and the intention are written to
memory straight away. The appraisal is also kept for `/chat why`.

**2. Speak** (`mind.Mind.Speak`) — the voice. Persona, the authored example
exchanges as real turns (v1 measured that this is what holds a voice), the
scene, the mood and the decided intent. It writes only the words.

Everything she sends is written down as a memory with its meaning attached:
`said to Big M: "…" — meant: the idea is actually clever`. This is the
single most important line in v2: it is what lets her own what she said
yesterday.

## Memory is Markdown

Under `CHAT_MEMORY_PATH` (default `./data/mind`), one directory per guild:

```
<guild>/
  self.md            who she is lately, and her mood; rewritten nightly
  people/<user>.md   one dossier per person: who they are, where things
                     stand between them, dated notes
  days/YYYY-MM-DD.md what happened that day, one line per moment, with a
                     summary written at night
  threads.md         what she means to do: "- [ ] 2026-09-22 18:00
                     [Big M:123] ask how the code city naming went"
```

Files rather than the datastore, deliberately:

- **Everything in here is prose the model reads and writes.** Markdown is its
  native form; a row of fields would be serialised to prose on every read and
  parsed back on every write.
- **A person can read and fix it.** Open `people/123.md` and you see exactly
  what she thinks of someone, and can correct it with a text editor. That is
  the debugging tool for a mind.
- **The scale is small.** A few people, a few dozen moments a day: the whole
  guild directory is kilobytes, and reading a dossier per reply costs less
  than the network round trip to the model. Recall reads the last two weeks of
  day files per call; at this size a cache would be one more thing to go
  stale.

Writes go through one mutex per store and land via write-to-temp-and-rename,
so a crash leaves the old file or the new one, never half of each. The
datastore keeps what is operational rather than mental: which channels she
reads, the brief, consent to be reached, message counts and reach
bookkeeping. `/chat forget` moves the guild directory aside to
`.forgotten/` rather than deleting it.

## How much she carries

Nothing grows with her age. Every call is assembled from bounded pieces,
whatever she has lived through:

| Piece | Bound |
|---|---|
| How she has been lately | about 900 characters, rewritten nightly |
| A dossier per person in the scene | two paragraphs and the last 6 notes; the file keeps 20 |
| What the last days meant | the summaries of 3 days |
| What stays with her about each person | 5 defining moments, chosen at night |
| Moments recalled | 8, by who is here, what is being said, how recent and how much it hit |
| What she means to do | 12 open at most |
| The live conversation | the channel buffer: half an hour, at least 8 lines |

That is how she forgets. Every moment carries a weight, 0 to 1, that her
appraisal gives it: small talk is light, hurt, pride and real warmth are
heavy. A light moment fades from recall in about a day and a half and is
gone after two weeks; a heavy one fades up to five times as slowly, counts
for more on its own, and stays recallable for three months. What survives
after that is what reflection carried into a day's summary, her account of
herself, or the few moments that stay with her about someone — the gist kept,
the detail let go.
On disk the files stay, at roughly a megabyte a year, for a person to read.

What is not there yet is a layer for long ago. Months on, an event lives only
in what it did to her self-description and her dossiers. If that proves too
thin, weekly and monthly summaries folded from the daily ones are the next
step, recalled the same way days are.

## Reflection

Once a day, at `CHAT_REFLECT_HOUR` in the community's timezone, each guild
that had moments the day before is reflected on in one call: the day's lines,
her self-description, the dossiers of everyone who appeared, her open
intentions. She writes a short summary of the day into its file, rewrites her
self-description, rewrites the "who they are" and "between us" paragraphs of
each dossier, chooses the few moments with each person that stay with her,
and closes or opens intentions. A dossier keeps its last 20
dated notes; what matters in older ones is expected to have been folded into
the paragraphs by then.

This is where relationships move. Not per message and not by a formula — by
her looking back at a day and deciding what it meant. Someone persistently
in good faith can become a friend; v1 could never let that happen.

## Starting something

Every ten minutes or so, varied, she may consider speaking unprompted. The
code gathers the candidates — an intention that has come due, someone who
opted into `/attention` and has not talked to her in a while, a channel set
to `/chat channel mode:speaks-first` that has gone quiet — and only then asks the model
whether she wants to act on any of them and why. The reason is written to
memory with the message, so "why did you tag me" has a true answer.

The guards are safety limits, not personality: never between 23:00 and
09:00, a daily cap per guild, nobody tagged without their consent, nothing
more to someone who has left two reaches unanswered until they speak again.

## A second thought, and typing like a person

The appraisal may carry `then`: something she will want to add a little
after her reply — a question it leaves her curious about, a thought on its
heels — and `then_after`, how many seconds later (5 to 600). It is born with
the reply, so there is no extra call to decide it; usually it is empty. It
waits in a queue the service owns and is sent only while her reply is still
the last word in the channel: if the person has answered, or anyone else has
spoken, the moment has passed and it is dropped. At most two an hour per
server, never after a late answer, and remembered as "a moment later I
added" like anything else she says.

A reply the voice writes as two paragraphs goes out as two messages, the
second typed after the first is seen. Every message waits after its typing
indicator about as long as a person would take to type it — a second plus
60 ms a character, at most eight — less the time the model already took.

## The rails the code keeps

The model decides; the code keeps a few promises the model cannot be trusted
with:

- **She never ignores the same person's direct approach twice running.** A
  silence is indistinguishable from a broken bot, so a second one in a row is
  overruled into a reply.
- **A burst gets one answer.** She waits for someone to stop typing — a few
  seconds of quiet — before reading.
- **Nothing the model says is posted unchecked:** control words and JSON never
  reach the channel, echoes of someone's line are dropped, a repeat of her own
  line is retried once with the repeat ruled out, and dropped if it repeats
  again. A line that opens the way her last two did counts as a repeat too,
  however short: "morning." three times, or "Big M, …" on every reply.
- **Only the person she answers can be notified** by a mention she writes.
- **A failed backend is not a decision.** An answer no backend would produce
  is held and retried, and goes out late as a reply to the message it
  answers.

## What was kept from v1

The conversation buffer and backfill, deferred answers, typing rules, reply
anchoring and mention resolution, `ai.Clean`, `Casual`, the echo and repeat
checks, the backend pool, the authored character file and its examples, and
`/attention` consent. What went: every word-list classifier and every
emotional scalar, the drives, the bond, the appraisal table, the
afterthought and volunteer gates, perception and inner-voice modes, and the
per-trigger odds.

## Discord ids never reach the model

Memory files keep Discord ids, because that is what finds someone's dossier.
Nothing sent to a model does. Message text has user, role and channel
mentions, custom emoji, message links and pasted ids rewritten to names or
words before it is recorded (`chat.plain`); reflection refers to people by
the number they were listed under; a channel the cache cannot name is "a
channel". `TestNoPromptCarriesAnID` and `TestIDsNeverReachTheModel` hold it.

## Measuring

`go run ./cmd/chatprobe -log temp/log.txt -as "Server Domme"` replays a
conversation copied out of Discord through v2, with a scratch memory
directory, and prints what she thought and said next to what v1 said at the
same point. The production log above is the regression scenario: v2 has to
own her own words, soften to an apology, and never post a control word.
