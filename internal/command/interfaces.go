package command

import "github.com/bwmarrin/discordgo"

// Responder is used by commands to reply without importing the discord package (avoids import cycles).
type Responder interface {
	RespondEmbedEphemeral(s *discordgo.Session, e *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) error
	RespondEmbed(s *discordgo.Session, e *discordgo.InteractionCreate, embed *discordgo.MessageEmbed) error
	CheckBotPermissions(s *discordgo.Session, channelID string) bool
	EmbedColor() int
}

// Logger logs command execution. It is injected into contexts so commands and middleware
// don't import the discord package.
type Logger interface {
	LogCommand(guildID, channelID, userID, username, commandName string) error
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

// Meta is exposed by the adapter so middleware can read Group/Category/Permissions
// without depending on the concrete command type.
type Meta interface {
	Group() string
	Category() string
	UserPermissions() []int64
}

// Handler is what individual Discord commands implement (Run takes interface{} for Discord contexts).
type Handler interface {
	Name() string
	Description() string
	Group() string
	Category() string
	UserPermissions() []int64
	Run(ctx interface{}) error
}

// Back-compat aliases (existing code will be migrated in later phases).
type DiscordMeta = Meta
type DiscordCommand = Handler
