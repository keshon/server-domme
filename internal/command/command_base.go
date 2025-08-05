package command

import (
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type Command interface {
	Name() string
	Description() string
	Category() string
	Aliases() []string
	Run(ctx interface{}) error

	RequireAdmin() bool
	RequireDev() bool
}

type SlashProvider interface {
	SlashDefinition() *discordgo.ApplicationCommand
}

type ContextMenuProvider interface {
	ContextDefinition() *discordgo.ApplicationCommand
}

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
