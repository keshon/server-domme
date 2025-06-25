// /internal/commands/context.go
package commands

import (
	"github.com/bwmarrin/discordgo"
)

type SlashContext struct {
	Session     *discordgo.Session
	Interaction *discordgo.InteractionCreate
	Args        []string

	// We expand struct with new values/functions here if needed (and pass to it from bot.go)
}
