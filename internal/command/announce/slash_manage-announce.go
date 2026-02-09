package announce

import (
	"fmt"
	"server-domme/internal/bot"
	"server-domme/internal/command"
	"server-domme/internal/middleware"

	"github.com/bwmarrin/discordgo"
)

type ManageAnnounceCommand struct{}

func (c *ManageAnnounceCommand) Name() string { return "manage-announce" }
func (c *ManageAnnounceCommand) Description() string {
	return "Announcement settings"
}
func (c *ManageAnnounceCommand) Group() string    { return "announce" }
func (c *ManageAnnounceCommand) Category() string { return "⚙️ Settings" }
func (c *ManageAnnounceCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *ManageAnnounceCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "set-channel",
				Description: "Set or update the announcement channel",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionChannel,
						Name:        "channel",
						Description: "Pick a channel from this server",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset-channel",
				Description: "Reset and remove the current announcement channel",
			},
		},
	}
}

func (c *ManageAnnounceCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*command.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	st := context.Storage

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	sub := data.Options[0]
	switch sub.Name {
	case "set-channel":
		channel := sub.Options[0].ChannelValue(s)
		if channel == nil {
			return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: "Invalid channel.",
			})
		}

		if err := st.SetAnnounceChannel(e.GuildID, channel.ID); err != nil {
			return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to set announcement channel: `%v`", err),
			})
		}

		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Announcement channel updated to <#%s>.", channel.ID),
		})

	case "reset-channel":
		if err := st.SetAnnounceChannel(e.GuildID, ""); err != nil {
			return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to reset announcement channel: `%v`", err),
			})
		}

		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Announcement channel has been reset.",
		})

	default:
		return bot.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Unknown subcommand.",
		})
	}
}

func init() {
	command.RegisterCommand(
		&ManageAnnounceCommand{},
		middleware.WithGroupAccessCheck(),
		middleware.WithGuildOnly(),
		middleware.WithUserPermissionCheck(),
		middleware.WithCommandLogger(),
	)
}
