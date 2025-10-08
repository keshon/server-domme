package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
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
func (c *DumpDBCommand) RequireDev() bool   { return false }

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

	session, event, storage := context.Session, context.Event, context.Storage
	guildID, member := event.GuildID, event.Member

	// Fetch guild record
	record, err := storage.GetGuildRecord(guildID)
	if err != nil {
		return core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to fetch record: ```%v```", err),
		})
	}

	// Encode record as JSON
	jsonBytes, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return core.RespondEmbedEphemeral(session, event, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("JSON encode failed: ```%v```", err),
		})
	}

	// Create response embed
	embed := &discordgo.MessageEmbed{
		Title:       "🧠 Database Dump",
		Description: "Here’s your current in-memory datastore snapshot.",
		Color:       core.EmbedColor,
	}

	// Send embed + file
	fileName := fmt.Sprintf("%s_database_dump.json", guildID)
	if err := core.RespondEmbedEphemeralWithFile(session, event, embed, bytes.NewReader(jsonBytes), fileName); err != nil {
		log.Println("Failed to send dump:", err)
	}

	// Log usage
	if err := core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name()); err != nil {
		log.Println("Failed to log:", err)
	}

	return nil
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&DumpDBCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
		),
	)
}
