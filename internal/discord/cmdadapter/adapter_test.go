package cmdadapter_test

import (
	"testing"

	"github.com/keshon/command"
	. "github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/middleware"
	"github.com/rs/zerolog"
)

// full implements every optional capability and records what reached it.
type full struct{ modal, component bool }

func (f *full) Name() string                   { return "full" }
func (f *full) Description() string            { return "" }
func (f *full) Group() string                  { return "" }
func (f *full) Category() string               { return "" }
func (f *full) UserPermissions() []int64       { return nil }
func (f *full) Run(interface{}) error          { return nil }
func (f *full) ObserveMessage(*MessageContext) {}
func (f *full) ModalSubmit(*ComponentInteractionContext) error {
	f.modal = true
	return nil
}
func (f *full) Component(*ComponentInteractionContext) error {
	f.component = true
	return nil
}

// The dispatcher sees a command the way Register leaves it: wrapped in an
// Adapter and in middleware, unwrapped with command.Root, which stops at the
// Adapter. Every capability a command can have has to survive that trip.
// /welcome template's editor did not: ModalSubmit was never forwarded, and no
// modal reached its command.
func TestEveryCapabilitySurvivesRegistration(t *testing.T) {
	cmd := &full{}
	// The same chain main registers every command with.
	wrapped := command.Apply(&Adapter{Cmd: cmd},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(zerolog.Nop()),
	)
	root := command.Root(wrapped)

	modal, ok := root.(ModalSubmitHandler)
	if !ok {
		t.Fatal("a registered command has no modal handler for the dispatcher to find")
	}
	if err := modal.ModalSubmit(&ComponentInteractionContext{}); err != nil || !cmd.modal {
		t.Error("a modal submission did not reach the command")
	}
	component, ok := root.(ComponentInteractionHandler)
	if !ok {
		t.Fatal("a registered command has no component handler for the dispatcher to find")
	}
	if err := component.Component(&ComponentInteractionContext{}); err != nil || !cmd.component {
		t.Error("a component interaction did not reach the command")
	}
	if observer, ok := root.(MessageObserverAdapter); !ok || !observer.ObserveMessage(&MessageContext{}) {
		t.Error("a message did not reach an observing command")
	}
}
