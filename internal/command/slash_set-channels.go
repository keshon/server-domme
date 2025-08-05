package command

import (
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type SetChannelsCommand struct{}

func (c *SetChannelsCommand) Name() string        { return "set-channels" }
func (c *SetChannelsCommand) Description() string { return "Designate special-purpose channels" }
func (c *SetChannelsCommand) Category() string    { return "⚙️ Maintenance" }
func (c *SetChannelsCommand) Aliases() []string   { return nil }

func (c *SetChannelsCommand) RequireAdmin() bool { return true }
func (c *SetChannelsCommand) RequireDev() bool   { return false }

func (c *SetChannelsCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Type:        discordgo.ChatApplicationCommand,
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionString,
				Name:        "type",
				Description: "What kind of channel are you setting?",
				Required:    true,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Confession Channel", Value: "confession"},
					{Name: "Announcement Channel", Value: "announce"},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionChannel,
				Name:        "channel",
				Description: "Pick a channel from this server",
				Required:    true,
			},
		},
	}
}

func (c *SetChannelsCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}

	session, event, storage, options := slash.Session, slash.Event, slash.Storage, slash.Event.ApplicationCommandData().Options

	if !isAdministrator(session, event.GuildID, event.Member) {
		return respondEphemeral(session, event, "You must be an Admin to use this command, darling.")
	}

	var kind, channelID string
	for _, opt := range options {
		switch opt.Name {
		case "type":
			kind = opt.StringValue()
		case "channel":
			channelID = opt.ChannelValue(session).ID
		}
	}

	if kind == "" || channelID == "" {
		return respondEphemeral(session, event, "Missing required parameters. Don't make me repeat myself.")
	}

	err := storage.SetSpecialChannel(event.GuildID, kind, channelID)
	if err != nil {
		return respondEphemeral(session, event, fmt.Sprintf("Couldn’t save it: `%s`", err.Error()))
	}

	var confirmation string
	switch kind {
	case "confession":
		confirmation = "💬 Confession channel updated. May secrets drip in silence."
	case "announce":
		confirmation = "📢 Announcement channel set. Don’t disappoint me with boring news."
	default:
		confirmation = fmt.Sprintf("✅ Channel for `%s` set.", kind)
	}

	err = respondEphemeral(session, event, confirmation)
	if err != nil {
		return err
	}

	err = logCommand(session, storage, event.GuildID, event.ChannelID, event.Member.User.ID, event.Member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log command:", err)
	}

	return nil
}

func init() {
	Register(WithGuildOnly(&SetChannelsCommand{}))
}
