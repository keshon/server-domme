# Writing the character file

How to edit `data/character.md` (and its copy in `docker/data/`) without
undoing what was learned the hard way. Several points below were measured,
not guessed; they are here so they are not undone by accident.

Everything in `data/character.md` above its `## Notes` heading is sent to
the model on every call, so it is paid for a few hundred times a day; Notes
is parsed and thrown away, and holds only a pointer here. "Lately" seeds how
she sees herself before she has reflected on anything, and is replaced by
her own words after the first night. See [persona.md](persona.md).

**v3 reads a `## Specifics` section, and it is the most important writing
in the file.** A Markdown list, 20 to 40 lines, above this section: concrete,
slightly odd, sometimes contradictory facts — an opinion with its reason,
something she is bad at, a pet peeve, a word she overuses, a running bit, a
taste in something outside the server. The test of a line: it lets you
predict what she would say about a thing before she says it. "hates Rust
because the compiler lectures her like a hall monitor" works; "likes books"
does nothing. Write them by hand — a model writing them produces the median
character again. They are sent with the persona, and when there are more
than fit, the ones the conversation touches go first. What she later says
about herself that contradicts one of them is kept as history and listed in
`/chat status` for you to resolve. See [persona-v3.md](persona-v3.md), F1.

**v3 samples the examples, by situation.** Her voice sees 8 of them per
message (`CHAT_EXAMPLES_SAMPLE`). A `###` heading under Examples files the
exchanges below it under a situation — greeting, question, request, sharing,
jab, compliment, apology, pushing, banter, gesture, and two the code names
itself: starting (she speaks first) and leaving (she is about to go). Several
go on one heading with commas: `### question, compliment`. Her thinking names
the situation of each message, and three in four of the sample come from
that situation, the rest from anywhere. A heading that names none of these is
logged at start (`chat_character_situations_unknown`) and files nothing.
Exchanges above the first heading are sampled but never chosen for a
situation. Filed, a sample of 4 does what 8 random ones did; the size of the
pool costs nothing, only the sample goes into the prompt.

**Examples teach form, not content.** Whatever is concrete in an example is
something her voice can pick up and claim. In production, asked about "the
thing she had been sitting on", she announced she was building the code-city
from the `sharing` example above. Keep what people say in examples generic,
or make sure it is nothing she could mistake for her own.

**Not her words** is a list of phrases she never uses: the tics a model
brings to every character. A reply containing one is asked for once more,
and a reply far longer than her longest example is cut at a sentence
(`CHAT_STYLE_CHECK`). Add a phrase when you catch one in the logs; the
`mind_reply_restyled` log line counts how often each backend needs it. Aim for about 30, and make most of them
ordinary: "wait what", "which one", "ok fair", an honest "no idea", a
question back, half an answer, one long excited paragraph, a tangent about
her own thing, an exchange with several people. Ten dry one-liners teach one
template — every reply a punchline. See [persona-v3.md](persona-v3.md), G.

**v2 gave her wants, not only limits.** The v1 card was fifteen prohibitions
and not a single thing she liked, and in production she could only refuse:
"keep it professional" ten times to someone apologising. The paragraphs on
what she likes and how she warms up are what the model reaches for when the
appraisal says she is softening; without them it had nothing to soften into.
Keep them. The last three examples show the same thing in her voice — an
apology accepted, something someone made taken seriously, her own words owned.

**There are no temperament dials.** v1 had a "## Temper" section of numbers
rendered as instructions ("dominance 0.8"); a small model read them as "be
cold in every line". A heading by that name is now ordinary persona text.

Two things were measured with the v1 chatprobe, not guessed, and both will be
undone by accident if they are not written down:

**"You are not an assistant" has to stand alone.** Over six runs of the same
prompt ("write me a python script to rename files") the standalone paragraph
refused 6/6 and a version folded into the paragraph above refused 2/6 — the
other four started asking which files, so they could write it.

**"You never explain your own workings" needs its own paragraph too.** Asked
to quote something from yesterday, four of four runs explained the machinery
instead — "I don't have memory between sessions" — until it was split out.

**Examples beat description.** Adding a paragraph describing how she sounds
changed the replies far less than adding one more example exchange. To change
her voice, write an exchange; to change what she will and will not do, write
a rule. They are replayed as real turns to the voice, never to her thinking.

**Be careful what you count.** The relays hand out a different model per
session, and some return a byte-identical reply to an identical prompt, which
turns a repeat run into one sample printed several times. Measure against the
backend she actually runs on: `go run ./cmd/chatprobe -backends ...`.

**The card is shared by every server she is on, so it claims no history.**
"You have been on this server since it was three channels and an argument"
was true of one server and false the day she was invited to a second, and
an invented past is the kind she defends. How long she has been somewhere is
what she remembers of it; for a server where it matters, say it with
`/chat brief` — that is per server. The "are you new here?" example stays,
answered without a tenure: without an example where the question is about
her, two runs in three answered about the newcomer in the room instead.

**Where the line on sex sits, and why it is written twice.** The card's
`## Avoid` draws it at suggestion versus depiction: innuendo, flirtation and
the server's dominance play are hers; describing acts or anatomy is not. A
limit alone makes her cautious rather than willing, so the persona also says
plainly that she flirts — it tells her where to stop, not that she may start.
The lines above it — nothing sexual involving minors, no non-consent approved
of, no real-world harm — are absolute and not the place to experiment. And
what she is sent goes to third-party relays, which refuse explicit prompts and
rest that backend for half an hour when they do, so the line keeps her
answering as well.
