# The persona, v3 — doings (workstream J, proposed)

> She can only be in the middle of something that exists.

Status: **proposed, not built.** Written 22 September 2026 after the first
live log of v3. It extends [persona-v3.md](persona-v3.md) with one new
layer. It changes no frozen part of the architecture: it applies the
principle, provenance and lifetimes as they stand to a kind of state v3 does
not have yet. It goes into the change log there when the first step is
built.

## Why

The first live log showed where the gap is. Asked by her own idle mind
for "something she is in the middle of", and with nothing she was in the
middle of, she announced "that thing i've been sitting on since last week".
Asked what it was, her voice took the code-city from a voice example and
presented it as her project, then defended the claim for six messages.
Everything else in the log read like a person.

The idle-mind rules asked for something she is in the middle of. Nothing in
the system could supply one.

v3 gives her perception of the town (walks), an interpretation of it
(feelings, wants, life), and a body that decides when she is there. Her
life is all perception and interpretation. **She never does anything that
leaves a trace**: nothing she started, nothing that moved on while nobody
talked to her, nothing she could show someone. People feed social hunger by
bringing things — "finished the book", "lost again", "look at this". She has
nothing to bring, so the model brings the median of the archetype, or
something lying around in its context.

The v3 principle already names the fix — *specificity is imported, not
generated* — and the Deferred section already sketches its shape (a book a
chapter a day). Walks import other people's specifics. Doings import her
own.

## The principle, applied

> **A doing is state the code owns and advances. The model chooses and
> reacts; it never reports progress the code did not make.**

| Code | Model | Code must not |
|---|---|---|
| keeps the ledger; runs each session; decides when a session happens, from the body; fetches the content (a chapter, a position, a list of what she has seen); validates every artifact; states progress as facts | chooses what to take up from what the code offers; reacts to each session in a line; picks what to keep from it (a line she underlines, a move); decides she is bored and drops it | state how she feels about it, write progress on her behalf, or let a claim of doing something stand without a record behind it |

Three rules follow, each answering the failure in the log:

1. **Progress is mechanical.** Chapter 7 of 40 is a counter the code
   advanced, not a sentence the model wrote. The model cannot be halfway
   through something the ledger has not started.
2. **Artifacts are checked the way sources are.** An underlined line must
   be a substring of the chapter she was given. A chess move must be legal
   in the position. A collection entry must cite a moment that exists.
   Anything else is refused and logged, like any other proposal
   (workstream I).
3. **A claim of doing is checked against the ledger.** If she says she is
   building, reading or playing something, that claim is kept as true only
   when the ledger has a matching doing. Otherwise it is refused as an
   invention (see J5).

## The ledger

One record per doing, **bot-wide**, like the body: she is one person
reading one book, whichever server asks about it. The guilds' minds each
see the ledger; what she said about it in each guild stays in that guild's
memory, as now.

| Field | Written by | Kind |
|---|---|---|
| id, kind (`reading`, `playing`, `collecting`), title | code | observed |
| params — which book, which opponent, what she collects | code, from the author's seed or the model's choice among offers | authored / interpreted |
| progress — chapter 7 of 40, move 22, 14 entries | code | observed |
| sessions — when, what the code did, how long | code | observed |
| her take on each session, one line | model, validated as text only | interpreted |
| artifacts — an underlined line, a finished game, a list | model proposes, code validates and keeps | observed once validated |
| status — active, paused, finished, dropped (and why) | code; dropping is the model's choice | observed / interpreted |

Stored in the datastore next to the body (operational, bot-wide). A
`doings.md` beside `self.md` in each guild's memory is rendered from it for
people reading the files, the same way `self.md` shows provenance inline.

## Kinds

A kind is a Go driver: something that can take a session, produce content
the model cannot make up, and validate what the model keeps. Each kind is
judged on four things: the content is real, it moves over days, it gives
her something to show, and it is safe and cheap.

### J-a. Reading — the first, and possibly enough

- **Source.** Project Gutenberg plain text, public domain, fetched once and
  cached. Which books are open to her comes from the card (see *Seeding*).
- **A session** gives the model the next piece: about 6,000 characters,
  ended at a paragraph. One thinking call answers:
  `{"take": "one line, what she made of it", "underline": "an exact line from it, or empty", "stop": true if she is done with the book}`.
- **Validated.** `underline` must be a substring of the piece, whitespace
  normalised. Otherwise it is refused. `take` is kept as interpreted.
- **Advances** by the piece, whatever she answers. A book of 400,000
  characters lasts about two months at one session a day, which is the arc
  the Deferred section wanted.
- **What it gives her:** "halfway through Moby Dick and it is mostly rope",
  a line she underlined that she can quote correctly, an opinion she formed
  about something real, and reasons to mention it that arrive on their own.

### J-b. Playing — a game with a result she cannot choose

- **Source.** Chess against a small engine in-process (a Go chess library
  for the rules, a fixed-depth engine for the opponent), or the daily puzzle
  from a public puzzle API with no key.
- **A session** is a few moves. The code lists the legal moves; the model
  picks one and says in a word why. The engine answers.
- **Validated.** The move must be in the list. The result is the engine's
  and the rules', never hers to report.
- **What it gives her:** losing. Streaks, a blunder she is annoyed about, a
  game someone on the server can replay from the PGN. A real outcome she did
  not choose is the opposite of a character card.
- **Social.** People can offer to play her. That is the same driver with a
  person as the opponent, their moves read from a channel. Deferred until the
  solo version is measured.

### J-c. Collecting — the town, kept

- **Source.** Her own walks and conversations. The collection is about
  something the author chose ("names people gave their projects", "things
  people built this month") and grows only from moments that exist.
- **A session** runs at reflection: the model is shown the day's observed
  moments that match the collection's subject and may add entries, each
  citing a moment by number, exactly as life items do.
- **Validated.** Each entry must cite a real observed moment. Neutral by
  construction: a collection is never a judgement of anyone (the F3 rule on
  opinions behind people's backs applies to what goes in it).
- **What it gives her:** a reason to walk, and an artifact the server can
  see — a pinned message she keeps, edited by the code when an entry lands.

### Not kinds

- **Writing, drawing, composing by the model.** The artifact would be real
  once it exists, but it is generated, which makes it the median again. It
  is also where "look what i made" turns into a model showing off. Not
  before reading has shown what a real input does.
- **Anything with an account, a purchase, or a feed.** News, social feeds
  and anything that follows links stay rejected for the reasons in
  *Deferred* in persona-v3.md.
- **Her own code-city.** The log's invention could be made true: a driver
  that takes a public repository someone shares and the code renders a
  skyline from file sizes and folders. It is listed here because it is
  tempting and because it is the author's idea. If it is built, the code
  builds the city and she only reacts to it. Considered after J-a to J-c.

## Seeding and choosing

- **The card seeds what she is into.** A new `## Doings` section lists
  kinds and their options, authored:

  ```markdown
  ## Doings
  - reading: sea stories and old detective novels — 2701, 1661, 244
  - playing: chess, badly
  - collecting: names people give their projects
  ```

  The numbers are Gutenberg ids the author picked. The code never searches
  on its own.
- **The model chooses among offers.** When a book is finished or dropped,
  the idle mind is offered the next few from the seed (title and a line of
  description, no more) and picks one, or none for a while. The choice is
  interpreted and hers. The options were the author's.
- **At most two active doings** at a time. A person with nine hobbies is a
  character sheet.

## Sessions and the body

This is where the doings layer closes a loop v3 left open. B2's *life
interrupts* sends her AWAY at random with no cause, so "where were you" has
no true answer.

- **An interruption can be a session.** When the body sends her AWAY because
  life interrupted, the code picks an active doing about half the time and
  runs its session during the absence. The absence keeps its length from the
  body. The session only fills it.
- **Otherwise on the idle tick**, at most once per doing per day, and only
  while awake and not in a conversation.
- **Never while asleep, never mid-exchange.** A session costs one thinking
  call, and B3 does not apply: she is not taking people in.
- **Coming back from a session** is an ordinary return. The one fact added
  to her next scene is when she last did it: "you were reading for 25
  minutes, until 14:10". It is a clock fact, the same kind B5 already
  allows.

"where did you go" — "reading. the whale has not shown up yet and i am
starting to take it personally" is then a true answer from a mind with a
day. That is the effect the whole layer is for.

## What the prompt may see

| Allowed (facts and her own takes) | Not allowed |
|---|---|
| "reading Moby Dick — chapter 31 of 135, since 12 Sep; last read 3 hours ago" | a percentage, a score, "she is enjoying it" |
| her last take: "Ishmael will not stop talking about rope" (her words, dated) | a summary of the book the model did not read in a session |
| an underlined line, quoted exactly, with its chapter | a line she did not underline |
| "chess: lost 4, won 1 this week; last game lost on move 31" | "she is frustrated with chess" |
| "you were reading for 25 minutes, until 14:10" | why she chose to |
| the list of what she is doing, and nothing else she is doing | — |

Where it goes:

- **Consider and Speak** see the active doings as above. The voice sees only
  the one line of the doing the conversation touches, matched the way
  self-facts are.
- **The idle mind** gets each active doing as a numbered source, so an
  impulse can come from one ("saw a line about the sea and thought of
  Rook's boat").
- **Recall.** Session takes are moments of kind interpreted, and artifacts
  are moments of kind observed, marked `did`, so drift and recall bring them
  back sideways like anything else.

The idle rule "something she is in the middle of" changes to *something
she is doing — only what is listed as what she is doing.*

## J5. Claims checked against the ledger

This closes the path the log showed: a claim made once becoming a self-fact,
then her life, then her history.

- **At self-fact reflection**, the model marks a fact that claims she is
  doing or did something (`"doing": true`) and names the doing it refers to
  by number from the ledger. Whether a sentence is a claim of doing is a
  reading, so it is the model's call. Whether the doing exists is the code's.
- **A doing-claim with no matching doing is refused**, with the reason "she
  is not doing that". The claim stays in the day's moments as something she
  said, and does not become true of her. It goes in `me.md` under *Said
  without doing*, beside *In conflict with the card*, for the author to see.
- **Jokes and play** are already meant to be skipped. A claim in play that
  gets through is caught here by the same rule.
- **The count is the invention metric for doings.** It should fall to near
  zero once she has real doings to talk about. If it does not, she is making
  things up for another reason, and that is worth knowing.

## Artifacts and showing them

- **She never writes an artifact out herself.** The voice names it by a
  handle: `[a3]`. The service substitutes the real thing on send: the
  underlined line in a quote block with its book and chapter, a game as PGN
  or a board in a code block, the collection as a list. A handle that does
  not exist is dropped. This is the *Sharing links* idea from Deferred,
  applied to her own artifacts first, because they need no outside source.
- **Only to people she is talking with, or in a room she speaks in.** An
  artifact is never posted in a `reads` channel, and nothing from a walk is
  quoted, as F3 already requires.
- **A collection can be pinned.** One message per collection in a channel
  the admin picks (`/chat doing pin`). The code edits it when an entry lands.

## Commands

| Command | Who | Does |
|---|---|---|
| `/chat doings` | anyone with `/chat` | lists what she is doing, with progress, and the artifacts by handle |
| `/chat doing pin collection channel` | admin | where a collection lives |
| `/chat doing drop id` | bot developer | ends a doing, recorded as dropped by the author |

She is not started on a doing by command. Seeding is the card's job, and
choosing among the offers is hers.

## Provenance and lifetimes

New rows for the tables in workstream I:

| Kind | Source kind | Source | Written by |
|---|---|---|---|
| doing and its progress | observed | the ledger | code |
| session take | interpreted | the session | model, stamped |
| artifact | observed | the session, validated against its content | model proposes, code keeps |
| choice of a doing | interpreted | the offers it was made from | model among code's offers |

| State | Leaves the prompt | Leaves the files |
|---|---|---|
| active doing | when finished or dropped | never; a finished book is part of her history |
| finished doing | shown only when the conversation matches it, like a self-fact | never |
| session take | last one shown while active; older ones by recall | never; about a line a day |
| artifact | by handle while its doing is active; by recall after | never |
| "said without doing" | while open, in `/chat status` | when the author clears it |

## Where things live

| What | Where |
|---|---|
| the ledger, progress, sessions, artifacts | datastore, bot-wide, next to the body |
| a cached book | `data/doings/`, fetched once |
| session takes, artifacts as moments | the guild's day file, the day they happened — in each guild she is in, like one body seen by several minds |
| `doings.md`, rendered for reading | each guild's memory directory |
| said without doing | `me.md`, its own section |

## Switches

| Variable | Default | Turns off |
|---|---|---|
| `CHAT_DOINGS` | `on` once J-a lands | no doings: her life is walks and conversations, as v3 |
| `CHAT_DOINGS_KINDS` | `reading` | which kinds run; the rest are ignored even if seeded |
| `CHAT_DOINGS_AWAY` | `0.5` | the odds an interruption is spent on a doing; `0` keeps interruptions without a cause |

## Measuring

| Signal | Metric | Expected |
|---|---|---|
| invention | doing-claims refused at self-fact reflection, per week | falls to near zero |
| grounded specificity | share of replies drawing on a doing (the causal-carryover judge gains the source "a doing") | rises; above walks on a quiet server |
| true absence | "where were you" and similar, answered with the session that filled the absence | answered truly whenever a session filled it |
| showing | artifacts posted, and how they were received (H5's welcome, by form) | some, rarely; answered more than her other starts |
| substance | the judge scores whether a mention of the book says something about the text (an underline, a chapter) or only that she is reading | mostly about the text |
| drift | takes over a book read by hand: do they stay hers (F1), or turn into book-report voice | stay hers |

Ablation adds one row: **v3 + J-a** against v3, on the same log with a
simulated clock. This is the cheapest test of the layer's thesis: that one
real doing does more for "felt like someone" than any number of perceptual
mechanisms.

## Order of work

1. **Ledger, commit, prompt facts, and J5 claims** — with no driver yet. J5
   is worth having before any doing exists, because it turns every invented
   project into a counted refusal instead of a self-fact.
2. **Reading (J-a)**, on the idle tick. Measure.
3. **Sessions in interruptions.** The body gets causes.
4. **Artifacts by handle**, starting with underlines.
5. **Collecting (J-c)**, which gives walks a purpose.
6. **Playing (J-b)**, then people as opponents.

## Risks

- **Book-report voice.** A model given a chapter writes a review. The
  take is one line, the voice sees only the last take, and the card's
  specifics decide what she notices. Read the takes by hand for the first
  week.
- **She becomes the book.** Every conversation turns into Moby Dick. Only
  the doing a conversation touches reaches the voice, and the idle mind
  sees doings as one source among many.
- **Relay limits on long inputs.** A 6,000-character piece is larger than
  anything else the thinking backends get. The piece size is a constant to
  tune against the budget in persona-v3.md.
- **Copyright.** Public-domain texts only; Gutenberg ids are authored, and
  the code refuses any source that is not on its list. Underlines are short
  by construction.
- **A hobby as a timer in costume.** If sessions fire on a clock and she
  reports each one, this is v1's heartbeat again. Sessions fill absences or
  run silently, and mentioning one is always the model's choice from what
  it sees, never an opening the code creates.
- **Two doings in two guilds.** One ledger, so one book, whichever server
  asks. What she said about it differs by guild, as her conversations do.
