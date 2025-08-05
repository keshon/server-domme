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
	slashCtx, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("не тот тип контекста")
	}

	s := slashCtx.Session
	i := slashCtx.Event
	st := slashCtx.Storage
	opts := i.ApplicationCommandData().Options

	if !isAdministrator(s, i.GuildID, i.Member) {
		return respondEphemeral(s, i, "You must be an Admin to use this command, darling.")
	}

	var kind, channelID string
	for _, opt := range opts {
		switch opt.Name {
		case "type":
			kind = opt.StringValue()
		case "channel":
			channelID = opt.ChannelValue(s).ID
		}
	}

	if kind == "" || channelID == "" {
		return respondEphemeral(s, i, "Missing required parameters. Don't make me repeat myself.")
	}

	err := st.SetSpecialChannel(i.GuildID, kind, channelID)
	if err != nil {
		return respondEphemeral(s, i, fmt.Sprintf("Couldn’t save it: `%s`", err.Error()))
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

	err = respondEphemeral(s, i, confirmation)
	if err != nil {
		return err
	}

	err = logCommand(s, st, i.GuildID, i.ChannelID, i.Member.User.ID, i.Member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log command:", err)
	}

	return nil
}

func init() {
	Register(WithGuildOnly(&SetChannelsCommand{}))
}
