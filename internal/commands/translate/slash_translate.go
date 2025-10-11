package translate

import (
	"fmt"
	"server-domme/internal/core"
	"server-domme/internal/storage"

	"github.com/bwmarrin/discordgo"
)

// Need to add at least one channel or translation wont work

type TranslateCommand struct{}

func (c *TranslateCommand) Name() string        { return "translate" }
func (c *TranslateCommand) Description() string { return "Manage translation reaction channels" }
func (c *TranslateCommand) Group() string       { return "translate" }
func (c *TranslateCommand) Category() string    { return "🌐 Utilities" }
func (c *TranslateCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

func (c *TranslateCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommandGroup,
				Name:        "manage",
				Description: "Manage channels for translate reactions",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "add",
						Description: "Add a channel to the translate list",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionChannel,
								Name:        "channel",
								Description: "Select a channel to enable translation reactions",
								Required:    true,
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "remove",
						Description: "Remove a channel from the translate list",
						Options: []*discordgo.ApplicationCommandOption{
							{
								Type:        discordgo.ApplicationCommandOptionChannel,
								Name:        "channel",
								Description: "Select a channel to remove from translation reactions",
								Required:    true,
							},
						},
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "list",
						Description: "List all channels enabled for translation reactions",
					},
					{
						Type:        discordgo.ApplicationCommandOptionSubCommand,
						Name:        "reset",
						Description: "Reset all channels for translation reactions",
					},
				},
			},
		},
	}
}

func (c *TranslateCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	s, e, storage := context.Session, context.Event, context.Storage

	options := e.ApplicationCommandData().Options
	if len(options) == 0 {
		return core.RespondEphemeral(s, e, "No subcommand provided.")
	}

	first := options[0]
	if first.Type != discordgo.ApplicationCommandOptionSubCommandGroup || first.Name != "manage" {
		return core.RespondEphemeral(s, e, "Unknown subcommand or command structure.")
	}

	sub := first.Options[0]
	switch sub.Name {
	case "add":
		return runAddChannel(s, e, *storage, sub)
	case "remove":
		return runRemoveChannel(s, e, *storage, sub)
	case "list":
		return runListChannels(s, e, *storage)
	case "reset":
		return runResetChannels(s, e, *storage)
	default:
		return core.RespondEphemeral(s, e, fmt.Sprintf("Unknown subcommand: %s", sub.Name))
	}
}

// --------------------- Subcommand Handlers ---------------------

func runAddChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	channelID := sub.Options[0].ChannelValue(s).ID
	if err := storage.AddTranslateChannel(e.GuildID, channelID); err != nil {
		return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to add channel: `%v`", err))
	}
	return core.RespondEphemeral(s, e, fmt.Sprintf("✅ <#%s> added to translate reaction channels.", channelID))
}

func runRemoveChannel(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage, sub *discordgo.ApplicationCommandInteractionDataOption) error {
	channelID := sub.Options[0].ChannelValue(s).ID
	if err := storage.RemoveTranslateChannel(e.GuildID, channelID); err != nil {
		return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to remove channel: `%v`", err))
	}
	return core.RespondEphemeral(s, e, fmt.Sprintf("✅ <#%s> removed from translate reaction channels.", channelID))
}

func runListChannels(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	channels, err := storage.GetTranslateChannels(e.GuildID)
	if err != nil {
		return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to fetch channels: `%v`", err))
	}

	if len(channels) == 0 {
		return core.RespondEphemeral(s, e, "No channels currently configured for translation reactions.")
	}

	desc := "Channels enabled for translation reactions:\n"
	for _, ch := range channels {
		desc += fmt.Sprintf("- <#%s>\n", ch)
	}

	return core.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Title:       "🌐 Translate Channels",
		Description: desc,
		Color:       core.EmbedColor,
	})
}

func runResetChannels(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	if err := storage.ResetTranslateChannels(e.GuildID); err != nil {
		return core.RespondEphemeral(s, e, fmt.Sprintf("Failed to reset channels: `%v`", err))
	}
	return core.RespondEphemeral(s, e, "✅ All translate reaction channels have been reset.")
}

// --------------------- Init ---------------------
func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&TranslateCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
