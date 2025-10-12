package confess

import (
	"fmt"
	"server-domme/internal/core"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

type ManageConfessCommand struct{}

func (c *ManageConfessCommand) Name() string        { return "manage-confess" }
func (c *ManageConfessCommand) Description() string { return "Confession settings" }
func (c *ManageConfessCommand) Group() string       { return "confess" }
func (c *ManageConfessCommand) Category() string    { return "⚙️ Settings" }
func (c *ManageConfessCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *ManageConfessCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "set-channel",
				Description: "Set the confession channel",
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
				Name:        "list-channel",
				Description: "Show the currently configured confession channel",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset-channel",
				Description: "Remove the confession channel",
			},
		},
	}
}

func (c *ManageConfessCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	s, e, storage := context.Session, context.Event, context.Storage
	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	sub := data.Options[0]
	return c.runManageConfessionChannel(s, e, *storage, sub)
}

func (c *ManageConfessCommand) runManageConfessionChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {

	switch sub.Name {
	case "set-channel":
		channelID := sub.Options[0].ChannelValue(s).ID
		if err := storage.SetConfessChannel(e.GuildID, channelID); err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to set confession channel: `%v`", err),
			})
		}
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Confession channel has been set to <#%s>.", channelID),
		})

	case "list-channel":
		channelID, err := storage.GetConfessChannel(e.GuildID)
		if err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to get confession channel: `%v`", err),
			})
		}
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Current confession channel is <#%s>.", channelID),
		})

	case "reset-channel":
		if err := storage.RemoveConfessChannel(e.GuildID); err != nil {
			return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
				Description: fmt.Sprintf("Failed to remove confession channel: `%v`", err),
			})
		}
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Confession channel has been removed.",
		})

	default:
		return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&ManageConfessCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
