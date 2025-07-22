package commands

import (
	"fmt"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           440,
		Name:           "init-commands",
		Description:    "Re-register all slash commands with authority.",
		Category:       "🏰 Court Administration",
		DCSlashHandler: initCommandsHandler,
	})
}

func initCommandsHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate

	if !isAdmin(s, i.GuildID, i.Member) {
		respondEphemeral(s, i, "You're not an admin, darling. Hands off the arsenal.")
		return
	}

	s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "Re-registering slash commands... Please hold your breath, or your tongue.",
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	go func() {
		appID := s.State.User.ID
		if appID == "" {
			app, err := s.User("@me")
			if err != nil {
				respondEphemeral(s, i, "Failed to get bot application ID: "+err.Error())
				return
			}
			appID = app.ID
		}

		existing, err := s.ApplicationCommands(appID, "")
		if err != nil {
			sendFollowup(s, i, "Failed to fetch existing commands: "+err.Error())
			return
		}

		for _, cmd := range existing {
			if err := s.ApplicationCommandDelete(appID, "", cmd.ID); err != nil {
				sendFollowup(s, i, fmt.Sprintf("Failed to delete command `%s`: %s", cmd.Name, err.Error()))
				return
			}
		}

		for _, cmd := range All() {
			if cmd.DCSlashHandler == nil {
				continue
			}
			_, err := s.ApplicationCommandCreate(appID, "", &discordgo.ApplicationCommand{
				Name:        cmd.Name,
				Description: cmd.Description,
				Options:     cmd.SlashOptions,
			})
			if err != nil {
				sendFollowup(s, i, fmt.Sprintf("Failed to register command `%s`: %s", cmd.Name, err.Error()))
				return
			}
		}

		sendFollowup(s, i, "Slash commands successfully refreshed. Praise be to your glorious uptime.")
	}()
}

func sendFollowup(s *discordgo.Session, i *discordgo.InteractionCreate, content string) {
	_, _ = s.FollowupMessageCreate(i.Interaction, false, &discordgo.WebhookParams{
		Content: content,
		Flags:   discordgo.MessageFlagsEphemeral,
	})
}
