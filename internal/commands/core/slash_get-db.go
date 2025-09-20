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

func (c *DumpDBCommand) Name() string        { return "get-db" }
func (c *DumpDBCommand) Description() string { return "Dumps server database as JSON file" }
func (c *DumpDBCommand) Aliases() []string   { return []string{} }
func (c *DumpDBCommand) Group() string       { return "core" }
func (c *DumpDBCommand) Category() string    { return "🛠️ Maintenance" }
func (c *DumpDBCommand) RequireAdmin() bool  { return true }
func (c *DumpDBCommand) RequireDev() bool    { return false }

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
	member := event.Member

	if !core.IsAdministrator(session, event.GuildID, event.Member) {
		core.RespondEphemeral(session, event, "You must be an Admin to use this command, darling.")
		return nil
	}

	record, err := storage.GetGuildRecord(event.GuildID)
	if err != nil {
		core.RespondEphemeral(session, event, fmt.Sprintf("Failed to fetch record: ```%v```", err))
		return nil
	}

	jsonBytes, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		core.RespondEphemeral(session, event, fmt.Sprintf("JSON encode failed: ```%v```", err))
		return nil
	}

	file := &discordgo.File{
		Name:        fmt.Sprintf("%s_database_dump.json", event.GuildID),
		ContentType: "application/json",
		Reader:      bytes.NewReader(jsonBytes),
	}

	err = session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: "🧠 Here's your juicy in-memory datastore dump.",
			Files:   []*discordgo.File{file},
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Println("Failed to send dump:", err)
	}

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
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
		),
	)
}
