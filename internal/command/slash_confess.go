package command

import (
	"fmt"
	"log"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type ConfessCommand struct{}

func (c *ConfessCommand) Name() string        { return "confess" }
func (c *ConfessCommand) Description() string { return "Send an anonymous confession" }
func (c *ConfessCommand) Aliases() []string   { return []string{} }

func (c *ConfessCommand) Group() string    { return "confess" }
func (c *ConfessCommand) Category() string { return "🎭 Roleplay" }

func (c *ConfessCommand) RequireAdmin() bool { return false }
func (c *ConfessCommand) RequireDev() bool   { return false }

func (c *ConfessCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "message",
				Description: "What do you need to confess?",
				Required:    true,
			},
		},
	}
}

func (c *ConfessCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member

	var message string
	for _, opt := range event.ApplicationCommandData().Options {
		if opt.Name == "message" {
			message = strings.TrimSpace(opt.StringValue())
		}
	}

	if message == "" {
		respondEphemeral(session, event, "You can't confess silence. Try again.")
		return nil
	}

	confessChannelID, err := storage.GetSpecialChannel(event.GuildID, "confession")
	if err != nil || confessChannelID == "" {
		respondEphemeral(session, event, "No confession channel is configured. Ask a mod to set it up.")
		return nil
	}

	embed := &discordgo.MessageEmbed{
		Title:       "📢 Anonymous Confession",
		Description: fmt.Sprintf("> %s", message),
		Color:       embedColor,
	}

	_, err = session.ChannelMessageSendEmbed(confessChannelID, embed)
	if err != nil {
		respondEphemeral(session, event, fmt.Sprintf("Couldn’t send your confession: ```%v```", err))
		return nil
	}

	if event.ChannelID != confessChannelID {
		link := fmt.Sprintf("https://discord.com/channels/%s/%s", event.GuildID, confessChannelID)
		respondEphemeral(session, event, fmt.Sprintf("Your secret has been dropped into the void.\nSee it echo: %s", link))
	} else {
		respondEphemeral(session, event, "💌 Delivered. Nobody saw a thing.")
	}

	err = logCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	Register(
		WithGroupAccessCheck()(
			WithGuildOnly(
				&ConfessCommand{},
			),
		),
	)
}
