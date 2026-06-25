package maintenance

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/keshon/server-domme/internal/discord/cmdadapter"
	"github.com/keshon/server-domme/internal/discord/discordreply"
	"github.com/keshon/server-domme/internal/storage"
)

func runExportData(s *discordgo.Session, e *discordgo.InteractionCreate, storage storage.Storage) error {
	guildID := e.GuildID
	record, err := storage.GuildRecord(guildID)
	if err != nil {
		return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to fetch record: ```%v```", err),
			Color:       discordreply.EmbedColor,
		})
	}

	jsonBytes, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("JSON encode failed: ```%v```", err),
			Color:       discordreply.EmbedColor,
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🧠 Database Dump",
		Description: "Here's your current in-memory datastore snapshot.",
		Color:       discordreply.EmbedColor,
	}

	fileName := fmt.Sprintf("%s_database_dump.json", guildID)
	return discordreply.RespondEmbedEphemeralWithFile(s, e, embed, bytes.NewReader(jsonBytes), fileName)
}

// RunSync triggers a guild command sync.
func RunSync(s *discordgo.Session, e *discordgo.InteractionCreate, syncer cmdadapter.CommandSyncer) error {
	return runSync(s, e, syncer)
}

func runSync(s *discordgo.Session, e *discordgo.InteractionCreate, syncer cmdadapter.CommandSyncer) error {
	if syncer != nil {
		_ = syncer.SyncGuildCommands(e.GuildID)
	}
	return discordreply.RespondEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "Command sync requested — it may take some time to apply.",
	})
}
