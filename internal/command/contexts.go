package command

import (
	"server-domme/internal/config"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

// Discord-specific contexts (what the runtime passes when executing).
// Config is injected so handlers and middleware never call config.New().

type SlashInteractionContext struct {
	Session   *discordgo.Session
	Event     *discordgo.InteractionCreate
	Args      []string
	Storage   *storage.Storage
	Config    *config.Config
	Responder Responder
	Logger    Logger
}

type ComponentInteractionContext struct {
	Session   *discordgo.Session
	Event     *discordgo.InteractionCreate
	Storage   *storage.Storage
	Config    *config.Config
	Responder Responder
	Logger    Logger
}

type MessageReactionContext struct {
	Session *discordgo.Session
	Event   *discordgo.MessageReactionAdd
	Storage *storage.Storage
	Config  *config.Config
	Logger  Logger
}

type MessageApplicationCommandContext struct {
	Session   *discordgo.Session
	Event     *discordgo.InteractionCreate
	Storage   *storage.Storage
	Target    *discordgo.Message
	Config    *config.Config
	Responder Responder
	Logger    Logger
}

type MessageContext struct {
	Session *discordgo.Session
	Event   *discordgo.MessageCreate
	Storage *storage.Storage
	Config  *config.Config
}

