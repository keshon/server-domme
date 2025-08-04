package commands

import (
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type SlashContext struct {
	Session           *discordgo.Session
	InteractionCreate *discordgo.InteractionCreate
	Args              []string
	Storage           *storage.Storage
}

type ComponentContext struct {
	Session           *discordgo.Session
	InteractionCreate *discordgo.InteractionCreate
	Storage           *storage.Storage
}

type ReactionContext struct {
	Session  *discordgo.Session
	Reaction *discordgo.MessageReactionAdd
	Storage  *storage.Storage
}
