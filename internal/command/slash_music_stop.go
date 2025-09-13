package command

import (
	"fmt"
	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type StopCommand struct {
	Bot core.BotVoice
}

func (c *StopCommand) Name() string        { return "music-stop" }
func (c *StopCommand) Description() string { return "Stop playback and clear queue" }
func (c *StopCommand) Aliases() []string   { return []string{} }
func (c *StopCommand) Group() string       { return "music" }
func (c *StopCommand) Category() string    { return "🎵 Music" }
func (c *StopCommand) RequireAdmin() bool  { return false }
func (c *StopCommand) RequireDev() bool    { return false }

func (c *StopCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *StopCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*core.SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	guildID := event.GuildID

	if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		return fmt.Errorf("failed to defer response: %w", err)
	}

	player := c.Bot.GetOrCreatePlayer(guildID)

	go func() {
		player.Stop(true)
	}()

	_, _ = session.FollowupMessageCreate(event.Interaction, true, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{{
			Title:       "⏹️ Playback Stopped",
			Description: "Queue cleared.",
			Color:       core.EmbedColor,
		}},
	})

	return nil
}

func init() {
	core.RegisterCommand(
		core.WithGroupAccessCheck()(
			core.WithGuildOnly(
				&StopCommand{},
			),
		),
	)
}
