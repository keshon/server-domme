# Conventions

These are the house rules, shared with
[melodix](https://github.com/keshon/melodix) so work can move between the two
bots. Most are enforced by tooling (gofmt, go vet, staticcheck via
golangci-lint, `go test -race`); the rest by review. If a rule gets in the way
of something that genuinely needs doing, pragmatism wins — just leave a note in
the code explaining the exception.

## Design principles

Go stays minimal here. No frameworks, no speculative abstraction — an interface
only exists if it has two real implementations or a real test seam behind it.
Everything else stays concrete.

New capability shows up as a command implementing `cmdadapter.Handler` and
whichever surface interfaces it needs, not as a new layer. See
[architecture.md](architecture.md#commands).

`main` owns process lifetime. A gateway event handler may not start anything
that outlives the session it fired on — `onReady` runs again on every reconnect,
so a service started there is either duplicated or unstoppable. Publish
readiness and let `main` react.

## Naming

Package names are single lowercase words describing what the package does
(`reply`, `perm`, `watchdog`, `execguard`). A package inside `internal/discord`
does not repeat "discord" in its own name.

Strings that select behavior get named constants rather than raw literals at the
call site. Task statuses (`"pending"`, `"completed"`, `"failed"`, `"safeword"`)
and purge modes (`"delayed"`, `"recurring"`) are the current holdouts and are
worth flagging in review.

## Frozen identifiers

Component custom IDs are frozen once shipped. A chooser already posted to a
channel keeps sitting there, and its ids come back whenever someone presses a
button — possibly long after a restart or a deploy. Add new ids; never rename or
repoint existing ones, and let an unrecognised one fail closed rather than
resolve to some default.

Slash command and subcommand names are nearly as sticky: renaming one costs
every user their muscle memory and forces a re-sync. Worth doing, sometimes —
but deliberately, not as a drive-by.

Stored keys are frozen for the obvious reason. The key layouts in
`storage/schema.go` are what addresses existing records; changing one needs a
migration, like `cmd/migrate-store`.

## Concurrency contracts

`RunSession` builds a fresh `*discordgo.Session` every call. Anything outliving
one session resolves it per use via `Bot.Session()` — never by capturing the
pointer.

Every goroutine has an owner and a clear way to exit. Background services take
`rootCtx` and are awaited in `main`'s WaitGroup; a goroutine that cannot be
cancelled is a bug, not a shortcut.

A read-modify-write against storage takes a transaction whenever two callers can
race for the same row. Concurrent HTTP redirects hitting `IncrementClicks` are
the live example.

## Errors and logging

Log events are lowercase, snake_case, verb-last (`purge_recurring_started`,
`shortlink_redirected`, `task_store_failed`), with structured fields rather than
interpolated strings. Everything goes through zerolog — the standard library
`log` package is not imported anywhere in this repo, and reintroducing it means
the event lands outside the configured sinks and rotation.

Command contexts carry `AppLog`; use it rather than threading a logger by hand.
Helpers that outlive a context take a `zerolog.Logger` parameter.

Errors returned to a user come back as embeds. A failed *reply* is best-effort:
log it and move on, because by then the work has already succeeded or failed and
reporting a reply error as a command error tells the user the wrong thing. The
`.golangci.yml` errcheck exclusions mark exactly which calls that covers;
anything not on that list gets handled.

## Comments

A comment earns its place by saying something the code cannot. The code already
says what it happens to do; a restatement is just a second thing to keep in
sync, and it sits in the way of the comment that actually matters. `// Create
embed` above a struct literal called `embed` is the shape to avoid, and so is a
doc comment that only expands the identifier back into a sentence.

The comments worth writing answer a question the code raises but cannot settle.
Why the purge scheduler resolves the session per use. Why short-link ids have no
guild prefix. Why `HeartbeatLatency()` is avoided. Whoever asks those next — a
maintainer months from now, or an agent told to "clean this up" — cannot recover
the answer from the code, and will helpfully undo it.

Three things make such a comment hold up:

**Name the failure it prevents.** "a captured session goes stale and its writes
target a closed connection" turns an arbitrary-looking indirection into
something nobody deletes by accident. A rule with no consequence attached reads
as a preference.

**Say what not to do.** `Do NOT call ClearExpiredCooldowns here`, `do not reach
for HeartbeatLatency`. A deliberate non-obvious choice needs a fence around it
or it gets optimized away — this is the highest-value kind of comment here, and
the one an agent is likeliest to violate in its absence.

**Point at the next hop by name.** `see purge.SessionFunc`, `see
storage/schema.go`. A reader who needs more should be told where it is, in a
form that greps.

On mechanics: every exported identifier gets a doc comment starting with its own
name, per Go convention — except methods implementing an interface, which
inherit the interface's doc and should not repeat it. Comments wrap at 80
columns and are written in English.

Three things go stale silently, so they don't get written at all: file-path
headers (`// FILE: internal/…`), which nothing checks and which outlive a `git
mv`; a claim about what does not exist yet, which still reads as fact long after
it stopped being one; and any restatement of a constant's value, which the
constant already carries. A comment that has drifted from the code is worse than
no comment, because it is believed.

## Testing & verification

`go test -race ./...` is the bar to clear.

Storage tests open a real store in `t.TempDir()` rather than mocking the
datastore — the invariants worth testing (trimming, guild isolation, expiry,
ownership) only exist end to end.

A regression test has to be watched failing before it is trusted. Reintroduce
the bug, confirm the new test goes red, then take it back out. The storage
suite was checked this way; a test that stays green with its bug reintroduced is
worse than no test, because it is counted as coverage.

Test the entry point, not only the helpers underneath it.

## Formatting & CI

Code should be gofmt-clean and `go vet`-clean, and the `.golangci.yml` set
should pass with zero findings — it's kept deliberately curated so that a
finding actually means something when it shows up.

`.gitattributes` pins the working tree to LF. Without it, `core.autocrlf` hands
Windows checkouts CRLF and golangci-lint's gofmt formatter reports files the
gofmt CLI considers fine — local lint then disagrees with CI for reasons that
have nothing to do with the code.

CI (`.github/workflows/build.yml`) runs vet, race tests and lint on every push
and PR, then cross-compiles all release targets.

`README.md` is generated, not hand-edited: change `README.md.tmpl` and run
`go run ./cmd/discord -readme` from the repo root. The bot never writes files at
runtime.

## Release notes

Release notes are read by people deciding whether to run this bot, not by
whoever fixed the bug. Lead with what changed *for them*, in plain language, and
say whether upgrading costs them anything: config changes, a migration, a new
dependency. One short paragraph of context is plenty.

Root-cause detail lives in commit messages, where the next maintainer actually
looks.

The release body comes from the **annotated tag's message body** —
`.github/workflows/release.yml` reads `%(contents:body)` — so that is where the
notes get written, and it has one sharp edge:

```bash
git tag -a --cleanup=verbatim vYYYY.MM.DD -F notes.md
```

`--cleanup=verbatim` is not optional. Git's default cleanup strips every line
beginning with `#` as a comment, which silently eats Markdown headings and
leaves a published release with its structure missing.
