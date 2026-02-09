package command

import (
	"context"

	"server-domme/internal/storage"
	"server-domme/pkg/cmd"

	"github.com/bwmarrin/discordgo"
)

// Discord-specific contexts (what the runtime passes when executing).

type SlashInteractionContext struct {
	Session *discordgo.Session
	Event   *discordgo.InteractionCreate
	Args    []string
	Storage *storage.Storage
}

type ComponentInteractionContext struct {
	Session *discordgo.Session
	Event   *discordgo.InteractionCreate
	Storage *storage.Storage
}

type MessageReactionContext struct {
	Session *discordgo.Session
	Event   *discordgo.MessageReactionAdd
	Storage *storage.Storage
}

type MessageApplicationCommandContext struct {
	Session *discordgo.Session
	Event   *discordgo.InteractionCreate
	Storage *storage.Storage
	Target  *discordgo.Message
}

type MessageContext struct {
	Session *discordgo.Session
	Event   *discordgo.MessageCreate
	Storage *storage.Storage
}

// Providers — how a command is registered with Discord (slash, context menu, reaction).

type SlashProvider interface {
	SlashDefinition() *discordgo.ApplicationCommand
}

type ContextMenuProvider interface {
	ContextDefinition() *discordgo.ApplicationCommand
}

type ReactionProvider interface {
	ReactionDefinition() string
}

type ComponentInteractionHandler interface {
	Component(*ComponentInteractionContext) error
}

// DiscordMeta is exposed by the Discord adapter so middleware can read Group/Category/Permissions
// without depending on the concrete Discord command type.
type DiscordMeta interface {
	Group() string
	Category() string
	UserPermissions() []int64
}

// DiscordCommand is what individual Discord commands implement (Run takes interface{} for Discord contexts).
type DiscordCommand interface {
	Name() string
	Description() string
	Group() string
	Category() string
	UserPermissions() []int64
	Run(ctx interface{}) error
}

// DiscordAdapter adapts a DiscordCommand to cmd.Command so it can live in the universal registry.
// It also implements SlashProvider, ContextMenuProvider, ReactionProvider, ComponentInteractionHandler,
// and DiscordMeta by delegating to the inner command.
type DiscordAdapter struct {
	Cmd DiscordCommand
}

func (a *DiscordAdapter) Name() string        { return a.Cmd.Name() }
func (a *DiscordAdapter) Description() string  { return a.Cmd.Description() }
func (a *DiscordAdapter) Group() string        { return a.Cmd.Group() }
func (a *DiscordAdapter) Category() string    { return a.Cmd.Category() }
func (a *DiscordAdapter) UserPermissions() []int64 { return a.Cmd.UserPermissions() }

func (a *DiscordAdapter) Run(ctx context.Context, inv *cmd.Invocation) error {
	return a.Cmd.Run(inv.Data)
}

func (a *DiscordAdapter) SlashDefinition() *discordgo.ApplicationCommand {
	if sp, ok := a.Cmd.(SlashProvider); ok {
		return sp.SlashDefinition()
	}
	return nil
}

func (a *DiscordAdapter) ContextDefinition() *discordgo.ApplicationCommand {
	if cp, ok := a.Cmd.(ContextMenuProvider); ok {
		return cp.ContextDefinition()
	}
	return nil
}

func (a *DiscordAdapter) ReactionDefinition() string {
	if rp, ok := a.Cmd.(ReactionProvider); ok {
		return rp.ReactionDefinition()
	}
	return ""
}

func (a *DiscordAdapter) Component(ctx *ComponentInteractionContext) error {
	if ch, ok := a.Cmd.(ComponentInteractionHandler); ok {
		return ch.Component(ctx)
	}
	return nil
}

// RegisterCommand registers a Discord command with the universal registry and applies middlewares.
func RegisterCommand(discordCmd DiscordCommand, mws ...cmd.Middleware) {
	c := cmd.Apply(&DiscordAdapter{Cmd: discordCmd}, mws...)
	cmd.DefaultRegistry.Register(c)
}
