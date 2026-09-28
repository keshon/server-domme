package adapter

import (
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// The two kinds of invocation, each carrying what a command may ask of it and
// nothing that names a library. Session and Event used to sit at the top of
// every one of these; what replaced them is Invoker for the questions whose
// answers are fixed, Responder for what can be said back, and API for what
// must be asked of the connection.
//
// The root handlers build these, which is the one place that knows what an
// invocation arrived as.

type SlashInteractionContext struct {
	Invoker   Invoker
	Responder Responder
	API       SessionAPI

	// Arguments are the options this command was invoked with, resolved when
	// the context was built.
	Arguments []SlashArgument

	// Attachments are the files the caller uploaded, by attachment id. An
	// OptionAttachment argument arrives as the id; the file itself is here.
	Attachments map[string]Attachment

	Args    []string
	Storage *storage.Storage
	Config  *config.Config
	Audit   AuditLog
	AppLog  zerolog.Logger
	Syncer  CommandSyncer
}

type ComponentInteractionContext struct {
	Invoker   Invoker
	Responder Responder
	API       SessionAPI

	// ComponentID identifies which component was used -- the button's own id,
	// set when the message was built.
	ComponentID string

	// MessageID is the message the component sits on.
	MessageID string
	// MessageContent is that message's text. Buttons act on posted state,
	// and the message is the record.
	MessageContent string
	// MessageEmbed is that message's first embed, or nil when it has none.
	// Buttons act on posted state (ask's request tracking), and the message
	// is the record -- refetching it is a request per click that the event
	// already carried.
	MessageEmbed *Embed

	Storage *storage.Storage
	Config  *config.Config
	Audit   AuditLog
	AppLog  zerolog.Logger
}
