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

// Handlers - optional specialized hooks beyond Run
type ComponentHandler interface {
	Component(*ComponentContext) error
}

type MessageHandler interface {
	Message(*MessageContext) error
}

// Contexts - what runtime hands you when executing a command
type SlashContext struct {
	Session *discordgo.Session
	Event   *discordgo.InteractionCreate
	Args    []string
	Storage *storage.Storage
}

type ComponentContext struct {
	Session *discordgo.Session
	Event   *discordgo.InteractionCreate
	Storage *storage.Storage
}

type ReactionContext struct {
	Session  *discordgo.Session
	Reaction *discordgo.MessageReactionAdd
	Storage  *storage.Storage
}

type MessageApplicationContext struct {
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

type CLIContext struct {
	Args    []string
	Storage *storage.Storage
}
