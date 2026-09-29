package confess

import (
	"fmt"
	"strings"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

type ConfessCommand struct{}

func (c *ConfessCommand) Name() string        { return "confess" }
func (c *ConfessCommand) Description() string { return "Send an anonymous confession" }
func (c *ConfessCommand) Group() string       { return "confess" }
func (c *ConfessCommand) Category() string    { return "🎭 Roleplay" }

// Unlogged keeps this command out of the audit log. Without it the posted
// confession stays anonymous while /settings commands log records who ran
// /confess and when — enough to identify the author by lining the two up.
func (c *ConfessCommand) Unlogged() {}

func (c *ConfessCommand) UserPermissions() []int64 {
	return []int64{}
}

func (c *ConfessCommand) SlashDefinition() *adapter.SlashCommand {
	return &adapter.SlashCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []adapter.SlashOption{
			{
				Type:        adapter.OptionString,
				Name:        "message",
				Description: "What do you need to confess?",
				Required:    true,
			},
		},
	}
}

func (c *ConfessCommand) Run(ctx *adapter.SlashInteractionContext) error {
	message := strings.TrimSpace(ctx.StringOption("message"))
	if message == "" {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: "No confession provided.",
			Color:       reply.EmbedColor,
		})
	}

	confessChannelID, err := ctx.Storage.GetConfessChannel(ctx.GuildID())
	if err != nil || confessChannelID == "" {
		// No confession channel set - fallback to current channel
		confessChannelID = ctx.ChannelID()
	}

	if err := ctx.API.SendChannelEmbed(confessChannelID, &adapter.Embed{
		Title:       "📢 Anonymous Confession",
		Description: fmt.Sprintf("> %s", message),
		Color:       reply.EmbedColor,
	}); err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to send confession: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	// Notify the user privately (ephemeral)
	if confessChannelID != ctx.ChannelID() {
		link := fmt.Sprintf("https://discord.com/channels/%s/%s", ctx.GuildID(), confessChannelID)
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Delivered. Nobody saw a thing.\nSee it here: %s", link),
			Color:       reply.EmbedColor,
		})
	}
	return ctx.RespondEphemeral(&adapter.Embed{
		Description: "Delivered. Nobody saw a thing.",
		Color:       reply.EmbedColor,
	})
}
