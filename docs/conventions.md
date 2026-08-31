# Conventions

These are the house rules, shared with
[melodix](https://github.com/keshon/melodix) so work can move between the two
bots. Where a rule reads identically in both, keep it that way.

## How to read this

Every rule below carries a tier, because a rule you cannot verify is worse
than no rule: it creates the belief of compliance without the fact, and it
teaches readers that the rules around it are optional too.

- **[enforced: <name>]** — a check fails, and `<name>` says which one.
  `internal/conventions` is an ordinary Go test, so `go test ./...` and CI run
  it with everything else; `.golangci.yml`, `gofmt` and the race detector
  cover the tags they own. `TestDocumentAndChecksAgree` keeps these tags and
  those checks in step, so this file cannot advertise enforcement that does
  not run, and a check cannot enforce something this file never mentions. The
  wording of each enforced rule is read back out of this document for the
  failure message, so the sentence you are reading is the one the build quotes
  at you.
- **[invariant]** — nothing checks it, and violating it breaks something
  nameable, often silently and often for users. These are the expensive ones.
  Each states the failure it prevents, so the cost is on the page rather than
  in someone's memory.
- **[practice]** — how things are done here. Nothing breaks if you deviate;
  the codebase just gets less coherent. Deliberately few.

A rule earns a place here only if it can claim one of those three. A
requirement without a tolerance and a test method is a preference, and
preferences get deleted rather than documented — that filter is what keeps
this file short enough to be read.

If a rule gets in the way of something that genuinely needs doing, pragmatism
wins — take the exception and leave a note in the code explaining it.

**This file is executable.** `internal/conventions` reads it while the tests
run, so editing it is a code change, not a documentation change. Reword a
paragraph tagged `[enforced: x]` and you have reworded a build failure
message. Remove the tag, rename it, or move this file, and the build goes red
until the checks are updated to match — deliberately, because a rule that
quietly stops being enforced is exactly the failure this whole arrangement
exists to prevent. Prose under `[invariant]` and `[practice]` is free to
change; nothing reads it.

**Adding a rule.** Give it a tier. If it would be [enforced], write the check
in `internal/conventions/conventions_test.go` first and let it fail, then
record the baseline. If it would be [invariant], name the failure in the rule
itself. If it is neither, do not add it.

## Design principles

**[practice]** Go stays minimal here. No frameworks, no speculative
abstraction — an interface only exists if it has two real implementations or a
real test seam behind it. Everything else stays concrete.

**[practice]** New capability shows up as a command implementing
`cmdadapter.Handler` and whichever surface interfaces it needs, not as a new
layer. See [architecture.md](architecture.md#commands).

**[invariant]** `main` owns process lifetime. A gateway event handler may not
start anything that outlives the session it fired on — `onReady` runs again on
every reconnect, so a service started there is either duplicated or
unstoppable. Publish readiness and let `main` react.

## Naming

**[practice]** Package names are single lowercase words describing what the
package does (`reply`, `perm`, `watchdog`, `execguard`). A package inside
`internal/discord` does not repeat "discord" in its own name.

**[enforced: named-constants]** Strings that select behavior get named
constants rather than raw literals at the call site. Task statuses and purge
modes live on `storage.TaskStatus*` and `storage.PurgeMode*`; the check counts
raw literals of those values so the number can only go down. A literal repeated
across branches is one typo away from a comparison that silently never matches,
and the compiler cannot help because both sides are strings. Where such a value
is also written to disk it is frozen as well as named — see Frozen identifiers.

## Frozen identifiers

**[enforced: frozen-identifiers]** Component custom IDs are frozen once
shipped. A chooser already posted to a channel keeps sitting there, and its ids
come back whenever someone presses a button — possibly long after a restart or
a deploy. Add new ids; never rename or repoint existing ones. The action tokens
and id layouts that are already out there are pinned by
`TestFrozenIdentifiers`.

**[enforced: frozen-identifiers]** Stored key layouts and the values written
into rows — task statuses, purge modes — are frozen for the obvious reason:
they are what addresses and describes records already on disk. Changing one
orphans them and needs a migration rather than an edit — `cmd/migrate-store` is
the precedent. The layouts in `internal/storage/schema.go` are pinned by the
same test.

**[invariant]** An unrecognised component id has to fail closed rather than
resolve to some default. One that silently resolves elsewhere acts on the wrong
record from a button the user pressed in good faith.

**[invariant]** Slash command and subcommand names are nearly as sticky:
renaming one costs every user their muscle memory and forces a re-sync. Worth
doing, sometimes — but deliberately, not as a drive-by.

## Concurrency contracts

**[invariant]** `RunSession` builds a fresh `*discordgo.Session` every call.
Anything outliving one session resolves it per use via `Bot.Session()` — never
by capturing the pointer, because a captured session goes stale and its writes
target a closed connection.

**[invariant]** Every goroutine has an owner and a clear way to exit.
Background services take `rootCtx` and are awaited in `main`'s WaitGroup; a
goroutine that cannot be cancelled is a bug, not a shortcut.

**[invariant]** A read-modify-write against storage takes a transaction
whenever two callers can race for the same row. Concurrent HTTP redirects
hitting `IncrementClicks` are the live example: without one, clicks are lost
silently and nothing ever reports an error.

## Errors and logging

**[enforced: no-stdlib-log]** Everything goes through zerolog. The standard
library `log` package is not imported anywhere in this repo, and reintroducing
it means the event lands outside the configured sinks and rotation — visible in
a terminal during development and simply missing in production, which is the
worst shape a logging bug can take. Checked by `TestNoBannedImports`.

**[enforced: log-event-naming]** Log events are lowercase, snake_case,
verb-last (`purge_recurring_started`, `shortlink_redirected`,
`task_store_failed`), with structured fields rather than interpolated strings.
`Msgf` and a computed event name are violations for the same reason — neither
can be grepped or aggregated.

**[enforced: error-prefix]** Errors carry the package name as a prefix, like
`storage: open task: …`. Exported sentinels are exempt: their text is also the
string a user is shown, and a package name in a Discord embed helps nobody. The
rule covers wrapped and internal errors — everything that exists to be read in
a log. A command is `package main`, where that name would say nothing, so it
prefixes with its directory instead: `migrate-store: read legacy store`.

**[invariant]** Sentinel errors are exported and matched with `errors.Is`.
Never pattern-match the error text: it is display copy, it gets reworded, and a
match against it fails silently when that happens.

**[practice]** Command contexts carry `AppLog`; use it rather than threading a
logger by hand. Helpers that outlive a context take a `zerolog.Logger`
parameter.

**[invariant]** Errors returned to a user come back as embeds. A failed *reply*
is best-effort: log it and move on, because by then the work has already
succeeded or failed, and reporting a reply error as a command error tells the
user the wrong thing. The `.golangci.yml` errcheck exclusions mark exactly
which calls that covers; anything not on that list gets handled.

## Comments

A comment earns its place by saying something the code cannot. The code
already says what it happens to do; a restatement is just a second thing to
keep in sync, and it sits in the way of the comment that actually matters.
`// Create embed` above a struct literal called `embed` is the shape to avoid,
and so is a doc comment that only expands the identifier back into a sentence.

The comments worth writing answer a question the code raises but cannot
settle. Why the purge scheduler resolves the session per use. Why short-link
ids have no guild prefix. Why `HeartbeatLatency()` is avoided. Whoever asks
those next — a maintainer months from now, or an agent told to "clean this
up" — cannot recover the answer from the code, and will helpfully undo it.

**[invariant]** Four things make such a comment hold up:

**Say whether it was measured or assumed.** "Verified against the live API,
not inferred" is worth more than any amount of confident prose: it tells a
reader which claims they may reason from and which they should re-check. A
guess is fine to write down — label it as one.

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

**[invariant] One fact, one home.** The four qualities above say what makes a
comment worth writing; they do not license writing it four times. A fact told
in two places will disagree with itself within a release, and the disagreement
is silent — both copies read as authoritative. When two comments would explain
the same decision, the one at the point of decision keeps it and the other
points there by name. This bites hardest right after a design is worked out,
when the reasoning feels worth repeating everywhere it touches: that is exactly
when to write it once.

Long explanations belong in one prose block at the top of the file or above the
declaration they explain, rather than sprinkled line by line through the body.
Struct fields carrying a contract get their own comment, and
concurrency-relevant ones say who owns the field and what lock covers it.

**[practice]** Every exported identifier gets a doc comment starting with its
own name, per Go convention — except methods implementing an interface, which
inherit the interface's doc and should not repeat it.

**[enforced: comment-width]** Comments wrap at 80 columns and are written in
English. A tab counts as one column, and a line carrying an unbreakable token
(a URL, a long identifier) is exempt.

**[enforced: file-headers]** No file-path headers (`// FILE: internal/…`).
Nothing checks them, so they survive every rename: the one this repo carried
named `melodix/internal/discord/middleware/command_logger.go`, a path that has
never existed in either project, and it had been wrong since the commit that
introduced it.

**[invariant]** Two more things go stale silently, so they don't get written at
all: a claim about what does not exist yet, which still reads as fact long
after it stopped being one — describe what the design allows instead; and any
restatement of a constant's value, which the constant already carries. A
comment that has drifted from the code is worse than no comment, because it is
believed.

## Testing & verification

**[enforced: race]** `go test -race ./...` is the bar to clear. CI runs it on
every push.

**[practice]** Storage tests open a real store in `t.TempDir()` rather than
mocking the datastore — the invariants worth testing (trimming, guild
isolation, expiry, ownership) only exist end to end.

**[invariant]** A regression test has to be watched failing before it is
trusted. Reintroduce the bug, confirm the new test goes red, then take it back
out. A test that passes for a reason other than the one you intended is the
normal outcome, not the rare one, and one that stays green with its bug
reintroduced is worse than no test because it is counted as coverage.

**[invariant]** Test the entry point, not only the helpers underneath it. A
check sitting between the helpers and the caller otherwise belongs to no test
at all.

## Formatting & CI

**[enforced: golangci]** Code is gofmt-clean and `go vet`-clean, and
`.golangci.yml` passes with zero findings — the set is kept deliberately
curated so a finding always means something. `internal/conventions` runs as
part of the same `go test ./...`.

**The baseline is currently empty.** Every ratcheted rule holds everywhere, so
a violation is a real regression rather than a number creeping up. It got there
by burning the debt down rather than by lowering the bar.

The convention checks ratchet: `internal/conventions/baseline.json` records
what each file owed when a rule was introduced, and a rule fails only when a
file gets **worse**. A new file carries no allowance, so it meets every rule in
full; a file that already owes something is only required not to owe more. That
is what lets a rule be adopted on a live codebase without a repo-wide edit
nobody can review.

Two things to know. It is a per-file **count**, not a per-line record: inside a
file that has an allowance you could remove one violation and add another
without the build noticing. And a `git mv` moves a file out from under its
allowance, which reads as a regression until the baseline is re-accepted.
Line-exact ratcheting is available off the shelf — `golangci-lint run
--new-from-merge-base=origin/main` — if either trade stops being acceptable.

After fixing violations, lock the gain in:

```bash
CONVENTIONS_UPDATE=1 go test ./internal/conventions/
```

On Windows, `conventions.bat` runs the checks and prints the scorecard, and
`conventions.bat accept` does the line above. `check.bat` runs the whole gate —
gofmt, vet, lint, race tests — which is what CI does.

An update run **always fails**, on purpose: it skips the ratchet, so a green
exit would mean one stray environment variable — a shell profile, a CI block,
an agent's environment — silently disabling every ratcheted rule with nothing
to show for it. Re-run without the variable to verify, and read the diff to
`baseline.json` before committing it. The number going up is the finding; the
baseline is only bookkeeping.

**[invariant]** `.gitattributes` pins the working tree to LF. Without it,
`core.autocrlf` hands Windows checkouts CRLF and golangci-lint's gofmt
formatter reports files the gofmt CLI considers fine — local lint then
disagrees with CI for reasons that have nothing to do with the code.

**[practice]** CI (`.github/workflows/build.yml`) runs vet, race tests and lint
on every push and PR, then cross-compiles all release targets.

**[invariant]** `README.md` is generated, not hand-edited: change
`README.md.tmpl` and run `go run ./cmd/discord -readme` from the repo root.
Editing the output means losing the edit on the next regeneration. The bot
never writes files at runtime.

## Release notes

**[practice]** Release notes are read by people deciding whether to run this
bot, not by whoever fixed the bug. Lead with what changed *for them*, in plain
language, and say whether upgrading costs them anything: config changes, a
migration, a new dependency. One short paragraph of context is plenty.

**[practice]** Root-cause detail lives in commit messages, where the next
maintainer actually looks — not on the release page, and not folded into a
`<details>` block at the end of it either.

**[invariant]** The release body comes from the **annotated tag's message
body** — `.github/workflows/release.yml` reads `%(contents:body)` — so that is
where the notes get written:

```bash
git tag -a --cleanup=verbatim vYYYY.MM.DD -F notes.md
```

`--cleanup=verbatim` is not optional. Git's default cleanup strips every line
beginning with `#` as a comment, which silently eats Markdown headings and
publishes a release with its structure missing. The tag's subject line is not
part of the release body either, so nothing load-bearing goes there.
