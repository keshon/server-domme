package middleware

import (
	"testing"

	"github.com/keshon/command"
	"github.com/keshon/server-domme/internal/command/confess"
	"github.com/keshon/server-domme/internal/command/roll"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"

	"github.com/rs/zerolog"
)

// /confess promises anonymity, and an audit row naming the caller would undo it
// from the other side. The guarantee is only worth as much as this test:
// without it, adding a middleware or renaming SkipAuditLog would quietly start
// recording who confessed, and nothing user-visible would change until someone
// read the log.
func TestCommandLoggerSkipsConfess(t *testing.T) {
	mw := WithCommandLogger(zerolog.Nop())

	got := command.Apply(&cmdadapter.Adapter{Cmd: &confess.ConfessCommand{}}, mw)
	if _, wrapped := got.(command.Unwrappable); wrapped {
		t.Fatal("/confess was wrapped by the audit logger: its caller would be written to storage")
	}
}

// The control: without the opt-out the logger must still wrap, or the test
// above would pass just as happily on a middleware that logs nothing at all.
func TestCommandLoggerWrapsOrdinaryCommands(t *testing.T) {
	mw := WithCommandLogger(zerolog.Nop())

	got := command.Apply(&cmdadapter.Adapter{Cmd: &roll.RollCommand{}}, mw)
	if _, wrapped := got.(command.Unwrappable); !wrapped {
		t.Fatal("/roll was not wrapped by the audit logger: nothing would be logged for any command")
	}
}

// The opt-out has to survive the full chain, not just a lone middleware:
// command.Root unwraps to the Adapter, and that is what the check reads.
func TestConfessStaysUnloggedThroughFullMiddlewareChain(t *testing.T) {
	chain := []command.Middleware{
		WithGroupAccessCheck(),
		WithGuildOnly(),
		WithUserPermissionCheck(),
		WithCommandLogger(zerolog.Nop()),
	}

	got := command.Apply(&cmdadapter.Adapter{Cmd: &confess.ConfessCommand{}}, chain...)

	root, ok := command.Root(got).(*cmdadapter.Adapter)
	if !ok {
		t.Fatalf("command.Root returned %T, not the Adapter the opt-out is read from", command.Root(got))
	}
	if !root.SkipAuditLog() {
		t.Fatal("ConfessCommand no longer opts out of the audit log")
	}
}
