// /internal/commands/context.go
package commands

import (
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type SlashContext struct {
	Session     *discordgo.Session
	Interaction *discordgo.InteractionCreate
	Args        []string
	Storage     *storage.Storage
}

type ComponentContext struct {
	Session     *discordgo.Session
	Interaction *discordgo.InteractionCreate
	Storage     *storage.Storage
}
