package music

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
func (c *StopCommand) Permissions() []int64 {
	return []int64{
		discordgo.PermissionSendMessages,
	}
}

func (c *StopCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *StopCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
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

	embed := &discordgo.MessageEmbed{
		Description: "⏹️ Playback Stopped. Queue cleared.",
	}

	session.FollowupMessageCreate(event.Interaction, true, &discordgo.WebhookParams{
		Embeds: []*discordgo.MessageEmbed{embed},
	})

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&StopCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithCommandLogger(),
		),
	)
}
