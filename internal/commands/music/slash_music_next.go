package music

import (
	"fmt"
	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type NextCommand struct {
	Bot core.BotVoice
}

func (c *NextCommand) Name() string        { return "music-next" }
func (c *NextCommand) Description() string { return "Skip to the next track" }
func (c *NextCommand) Aliases() []string   { return []string{} }
func (c *NextCommand) Group() string       { return "music" }
func (c *NextCommand) Category() string    { return "🎵 Music" }
func (c *NextCommand) RequireAdmin() bool  { return false }
func (c *NextCommand) RequireDev() bool    { return false }

func (c *NextCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *NextCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event

	guildID := event.GuildID
	member := event.Member

	if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		return fmt.Errorf("failed to defer response: %w", err)
	}

	voiceState, err := c.Bot.FindUserVoiceState(guildID, member.User.ID)
	if err != nil {
		session.FollowupMessageCreate(event.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("🎵 Error: %s", err.Error()),
		})
		return nil
	}

	player := c.Bot.GetOrCreatePlayer(guildID)
	queue := player.Queue()
	if len(queue) == 0 {
		session.FollowupMessageCreate(event.Interaction, true, &discordgo.WebhookParams{
			Content: "🎵 No tracks in queue.",
		})
		return nil
	}

	player.Stop(false)

	err = player.PlayNext(voiceState.ChannelID)
	if err != nil {
		session.FollowupMessageCreate(event.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("🎵 Error: %s", err.Error()),
		})
		return nil
	}

	listenPlayerStatusSlash(session, event, player)

	// session.FollowupMessageCreate(event.Interaction, true, &discordgo.WebhookParams{
	// 	Content: "🎵 Skipped to next track.",
	// })

	return nil
}

// We dont register this command here, it is registered in the bot package as we need access to the bot instance
