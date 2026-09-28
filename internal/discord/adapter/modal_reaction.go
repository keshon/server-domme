package adapter

import (
	"github.com/keshon/server-domme/internal/config"
	"github.com/keshon/server-domme/internal/storage"
	"github.com/rs/zerolog"
)

// Domme interaction kinds beyond slash and components.
//
// The melodix adapter this package is ported from only knows slash commands
// and message components. Server-domme additionally needs three kinds, each
// kept by exactly one command: /welcome opens a modal editor,
// /announce ships a message context-menu entry, and /translate answers flag
// reactions with a DM. They live here rather than in a separate package
// because dispatch, middleware and help rendering all switch on the same
// invocation shape, and a second adapter would have to repeat every one of
// those.

// ModalSubmitHandler is implemented by commands that open a modal and handle
// its submission. The modal's customID starts with the command name, the same
// convention components follow.
type ModalSubmitHandler interface {
	ModalSubmit(*ModalSubmitContext) error
}

// MessageCommandHandler is implemented by commands that act on a message
// picked through a message context-menu entry (Type MessageMenuCommand).
type MessageCommandHandler interface {
	MessageCommand(*MessageCommandContext) error
}

// ReactionHandler is implemented by commands triggered by a message reaction
// rather than by an interaction. A reaction carries no token to answer, so
// the context offers the connection instead of a responder.
type ReactionHandler interface {
	React(*ReactionContext) error
}

// ModalField is one paragraph input in a modal.
type ModalField struct {
	CustomID    string
	Label       string
	Placeholder string
	Value       string
	Required    bool
	MaxLength   int
}

// Modal is what a command asks the user to fill in.
type Modal struct {
	CustomID string
	Title    string
	Fields   []ModalField
}

// ModalSubmitContext is a submitted modal: which one, and what was typed.
type ModalSubmitContext struct {
	Invoker   Invoker
	Responder Responder
	API       SessionAPI

	// ComponentID is the modal's own customID, set when it was opened.
	ComponentID string
	// Values maps field customID to the text typed into it.
	Values map[string]string

	Storage *storage.Storage
	Config  *config.Config
	Audit   AuditLog
	AppLog  zerolog.Logger
}

// MessageCommandContext is a message context-menu invocation: which message
// was picked, answered like any other interaction.
type MessageCommandContext struct {
	Invoker   Invoker
	Responder Responder
	API       SessionAPI

	// TargetMessageID is the message the entry was used on.
	TargetMessageID string

	Storage *storage.Storage
	Config  *config.Config
	Audit   AuditLog
	AppLog  zerolog.Logger
	Syncer  CommandSyncer
}

// ReactionContext is a reaction added to a message. There is nothing to
// answer through — the handler acts through the API (a DM, a cleanup) — so
// there is no responder and no audit row.
type ReactionContext struct {
	Invoker Invoker
	API     SessionAPI

	MessageID string
	// Emoji is the reaction as Discord spells it (unicode glyph or name).
	Emoji string

	Storage *storage.Storage
	Config  *config.Config
	AppLog  zerolog.Logger
}

// ModalValue reads one typed field, empty when absent.
func (c *ModalSubmitContext) ModalValue(fieldID string) string {
	if c == nil {
		return ""
	}
	return c.Values[fieldID]
}

// --- dispatch ---

// RunModal dispatches a modal submission to the command's submit handler.
func (a *Adapter) RunModal(ctx *ModalSubmitContext) error {
	if mh, ok := a.Cmd.(ModalSubmitHandler); ok {
		return mh.ModalSubmit(ctx)
	}
	return nil
}

// RunMessageCommand dispatches a context-menu invocation.
func (a *Adapter) RunMessageCommand(ctx *MessageCommandContext) error {
	if mh, ok := a.Cmd.(MessageCommandHandler); ok {
		return mh.MessageCommand(ctx)
	}
	return nil
}

// WantsReactions reports whether the wrapped command handles reactions.
func (a *Adapter) WantsReactions() bool {
	_, ok := a.Cmd.(ReactionHandler)
	return ok
}

// React forwards a reaction to the wrapped command.
func (a *Adapter) React(ctx *ReactionContext) error {
	if rh, ok := a.Cmd.(ReactionHandler); ok {
		return rh.React(ctx)
	}
	return nil
}

// --- middleware view ---

func (c *ModalSubmitContext) GuildID() string         { return c.Invoker.GuildID }
func (c *ModalSubmitContext) ChannelID() string       { return c.Invoker.ChannelID }
func (c *ModalSubmitContext) UserID() string          { return c.Invoker.UserID }
func (c *ModalSubmitContext) Username() string        { return c.Invoker.Username }
func (c *ModalSubmitContext) AuditLog() AuditLog      { return c.Audit }
func (c *ModalSubmitContext) Store() *storage.Storage { return c.Storage }
func (c *ModalSubmitContext) MemberPermissions() (int64, error) {
	return memberPermissions(c.API, c.Invoker)
}
func (c *ModalSubmitContext) CanReplyPrivately() bool { return c.Responder != nil }
func (c *ModalSubmitContext) ReplyEphemeral(msg string) error {
	return respondEphemeral(c.Responder, c.AppLog, msg)
}

func (c *MessageCommandContext) GuildID() string         { return c.Invoker.GuildID }
func (c *MessageCommandContext) ChannelID() string       { return c.Invoker.ChannelID }
func (c *MessageCommandContext) UserID() string          { return c.Invoker.UserID }
func (c *MessageCommandContext) Username() string        { return c.Invoker.Username }
func (c *MessageCommandContext) AuditLog() AuditLog      { return c.Audit }
func (c *MessageCommandContext) Store() *storage.Storage { return c.Storage }
func (c *MessageCommandContext) MemberPermissions() (int64, error) {
	return memberPermissions(c.API, c.Invoker)
}
func (c *MessageCommandContext) CanReplyPrivately() bool { return c.Responder != nil }
func (c *MessageCommandContext) ReplyEphemeral(msg string) error {
	return respondEphemeral(c.Responder, c.AppLog, msg)
}

// Reactions run outside interactions: there is nobody to answer quietly, so
// a refusal is silence and the audit trail does not apply.
func (c *ReactionContext) GuildID() string         { return c.Invoker.GuildID }
func (c *ReactionContext) ChannelID() string       { return c.Invoker.ChannelID }
func (c *ReactionContext) UserID() string          { return c.Invoker.UserID }
func (c *ReactionContext) Username() string        { return c.Invoker.Username }
func (c *ReactionContext) AuditLog() AuditLog      { return nil }
func (c *ReactionContext) Store() *storage.Storage { return c.Storage }
func (c *ReactionContext) MemberPermissions() (int64, error) {
	return memberPermissions(c.API, c.Invoker)
}
func (c *ReactionContext) CanReplyPrivately() bool { return false }
func (c *ReactionContext) ReplyEphemeral(_ string) error {
	return nil
}

// --- invocation helpers ---

func init() {
	// Compile-time proof that the extended contexts satisfy what middleware
	// asserts on. A context that stops being a CommandContext passes dispatch
	// and then falls through every middleware check in silence.
	var _ CommandContext = (*ModalSubmitContext)(nil)
	var _ CommandContext = (*MessageCommandContext)(nil)
	var _ CommandContext = (*ReactionContext)(nil)
}

// ConfigFromInvocation already unwraps the extended contexts (see helpers.go).
