package maintenance

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/keshon/server-domme/internal/discord/adapter"
	"github.com/keshon/server-domme/internal/discord/reply"
)

func runExportData(ctx *adapter.SlashInteractionContext) error {
	guildID := ctx.GuildID()
	record, err := ctx.Storage.ExportGuild(guildID)
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("Failed to fetch record: ```%v```", err),
			Color:       reply.EmbedColor,
		})
	}

	jsonBytes, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return ctx.RespondEphemeral(&adapter.Embed{
			Description: fmt.Sprintf("JSON encode failed: ```%v```", err),
			Color:       reply.EmbedColor,
		})
	}

	fileName := fmt.Sprintf("%s_database_dump.json", guildID)
	return ctx.RespondWith(adapter.Reply{
		Embed: &adapter.Embed{
			Title:       "🧠 Database Dump",
			Description: "Here's your current in-memory datastore snapshot.",
			Color:       reply.EmbedColor,
		},
		File:      bytes.NewReader(jsonBytes),
		FileName:  fileName,
		Ephemeral: true,
	})
}

// runSync triggers a guild command sync.
func runSync(ctx *adapter.SlashInteractionContext) error {
	if ctx.Syncer != nil {
		_ = ctx.Syncer.SyncGuildCommands(ctx.GuildID())
	}
	return ctx.RespondEphemeral(&adapter.Embed{
		Description: "Command sync requested — it may take some time to apply.",
	})
}
