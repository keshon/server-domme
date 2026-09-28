package maintenance

import (
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

func runStatus(ctx *adapter.SlashInteractionContext) error {
	guild, err := ctx.Guild()
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to fetch guild: `%v`", err),
			Color:       reply.EmbedColor,
		})
	}

	// Build message
	desc := fmt.Sprintf(
		"**Guild name: %s**\n"+
			"**Guild ID: %s**\n"+
			"**Guild statistics:**\n"+
			"- Members: %d\n"+
			"- Roles: %d\n"+
			"- Channels: %d\n",
		guild.Name,
		guild.ID,
		guild.Members,
		guild.Roles,
		guild.Channels,
	)

	return ctx.RespondEphemeral(&adapter.Embed{
		Title:       "📊 Guild Status",
		Description: desc,
		Color:       reply.EmbedColor,
	})
}
