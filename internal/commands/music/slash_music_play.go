package music

import (
	"fmt"
	"log"
	"server-domme/internal/core"
	"server-domme/internal/music/source_resolver"

	"github.com/bwmarrin/discordgo"
)

type PlayCommand struct {
	Bot core.BotVoice
}

func (c *PlayCommand) Name() string        { return "music-play" }
func (c *PlayCommand) Description() string { return "Play music track" }
func (c *PlayCommand) Aliases() []string   { return []string{} }
func (c *PlayCommand) Group() string       { return "music" }
func (c *PlayCommand) Category() string    { return "🎵 Music" }
func (c *PlayCommand) RequireAdmin() bool  { return false }
func (c *PlayCommand) RequireDev() bool    { return false }

func (c *PlayCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "input",
				Description: "Link to youtube/soundcloud or song name",
				Required:    true,
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "source",
				Description: "Source to use if song name is given",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "youtube", Value: "youtube"},
					{Name: "soundcloud", Value: "soundcloud"},
					{Name: "radio", Value: "radio"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "parser",
				Description: "Parser to use (overrides autodetect)",
				Required:    false,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "ytdlp pipe", Value: "ytdlp-pipe"},
					{Name: "ytdlp link", Value: "ytdlp-link"},
					{Name: "kkdai pipe", Value: "kkdai-pipe"},
					{Name: "kkdai link", Value: "kkdai-link"},
					{Name: "ffmpeg direct link", Value: "ffmpeg-link"},
				},
			},
		},
	}
}

func (c *PlayCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	guildID := event.GuildID
	member := event.Member

	if err := core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name()); err != nil {
		log.Println("Failed to log:", err)
	}

	options := event.ApplicationCommandData().Options
	var input, selectedParser, selectedSource string

	for _, opt := range options {
		switch opt.Name {
		case "input":
			input = opt.StringValue()
		case "source":
			selectedSource = opt.StringValue()
		case "parser":
			selectedParser = opt.StringValue()
		}
	}

	if input == "" {
		return session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "🎵 Error: input is required",
			},
		})
	}

	if err := session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
	}); err != nil {
		return fmt.Errorf("failed to send deferred response: %w", err)
	}

	voiceState, err := c.Bot.FindUserVoiceState(guildID, member.User.ID)
	if err != nil {
		_, _ = session.FollowupMessageCreate(event.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("🎵 Error: %s", err.Error()),
		})
		return nil
	}

	resolver := source_resolver.New()
	tracks, err := resolver.Resolve(input, selectedSource, selectedParser)
	if err != nil || len(tracks) == 0 {
		_, _ = session.FollowupMessageCreate(event.Interaction, true, &discordgo.WebhookParams{
			Content: fmt.Sprintf("🎵 Error: failed to resolve track: %v", err),
		})
		return nil
	}

	currentTrack := tracks[0]

	player := c.Bot.GetOrCreatePlayer(guildID)
	player.Enqueue(currentTrack.URL, selectedSource, selectedParser)
	if !player.IsPlaying() {
		player.PlayNext(voiceState.ChannelID)
	}

	listenPlayerStatusSlash(session, event, player)

	// _, _ = session.FollowupMessageCreate(event.Interaction, true, &discordgo.WebhookParams{
	// 	Content: fmt.Sprintf("🎵 Now playing: **%s**\n%s", currentTrack.Title, currentTrack.URL),
	// })

	return nil
}

// We dont register this command here, it is registered in the bot package as we need access to the bot instance
