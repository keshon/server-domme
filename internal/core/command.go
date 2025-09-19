package core

import (
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type Command interface {
	Name() string
	Description() string
	Aliases() []string
	Group() string
	Category() string
	RequireAdmin() bool
	RequireDev() bool
	Run(ctx interface{}) error
}

// Providers - how this command should be registered with Discord
type SlashProvider interface {
	SlashDefinition() *discordgo.ApplicationCommand
}

type ContextMenuProvider interface {
	ContextDefinition() *discordgo.ApplicationCommand
}

// Contexts - what runtime hands you when executing a command
// Slash command
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

// Hook for component beyond Run
type ComponentInteractionHandler interface {
	Component(*ComponentInteractionContext) error
}

// Reaction to a message
type MessageReactionContext struct {
	Session  *discordgo.Session
	Reaction *discordgo.MessageReactionAdd
	Storage  *storage.Storage
}

// Contexnt menu over a message
type MessageApplicationCommandContext struct {
	Session *discordgo.Session
	Event   *discordgo.InteractionCreate
	Storage *storage.Storage
	Target  *discordgo.Message
}

// Message
type MessageContext struct {
	Session *discordgo.Session
	Event   *discordgo.MessageCreate
	Storage *storage.Storage
}
