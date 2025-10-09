package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"server-domme/internal/core"

	"github.com/bwmarrin/discordgo"
)

type DumpDBCommand struct{}

func (c *DumpDBCommand) Name() string { return "get-db" }
func (c *DumpDBCommand) Description() string {
	return "Dump the current server database as a JSON file"
}
func (c *DumpDBCommand) Aliases() []string  { return []string{} }
func (c *DumpDBCommand) Group() string      { return "core" }
func (c *DumpDBCommand) Category() string   { return "🛠️ Maintenance" }
func (c *DumpDBCommand) RequireAdmin() bool { return true }
func (c *DumpDBCommand) Permissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}

func (c *DumpDBCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *DumpDBCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	session := context.Session
	event := context.Event
	storage := context.Storage

	guildID := event.GuildID

	// Fetch guild record
	record, err := storage.GetGuildRecord(guildID)
	if err != nil {
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to fetch record: ```%v```", err),
		})
		return nil
	}

	// Encode record as JSON
	jsonBytes, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("JSON encode failed: ```%v```", err),
		})
		return nil
	}

	// Create response embed
	embed := &discordgo.MessageEmbed{
		Title:       "🧠 Database Dump",
		Description: "Here’s your current in-memory datastore snapshot.",
		Color:       core.EmbedColor,
	}

	// Send embed + file
	fileName := fmt.Sprintf("%s_database_dump.json", guildID)
	core.RespondEmbedEphemeralWithFile(session, event, embed, bytes.NewReader(jsonBytes), fileName)

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&DumpDBCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithCommandLogger(),
		),
	)
}
