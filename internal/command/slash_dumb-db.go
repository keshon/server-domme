package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

type DumpDBCommand struct{}

func (c *DumpDBCommand) Name() string        { return "dump-db" }
func (c *DumpDBCommand) Description() string { return "Dumps the bot database" }
func (c *DumpDBCommand) Aliases() []string   { return []string{} }

func (c *DumpDBCommand) Group() string    { return "dump-db" }
func (c *DumpDBCommand) Category() string { return "⚙️ Maintenance" }

func (c *DumpDBCommand) RequireAdmin() bool { return false }
func (c *DumpDBCommand) RequireDev() bool   { return true }

func (c *DumpDBCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *DumpDBCommand) Run(ctx interface{}) error {
	slash, ok := ctx.(*SlashContext)
	if !ok {
		return fmt.Errorf("wrong context type")
	}
	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	userID := event.Member.User.ID
	if !isDeveloper(userID) {
		respondEphemeral(session, event, "🚫 You don't have permission to use this command.")
		return nil
	}

	dumpData, err := storage.Dump()
	if err != nil {
		respondEphemeral(session, event, fmt.Sprintf("❌ Failed to dump datastore: ```%v```", err))
		return nil
	}

	jsonBytes, err := json.MarshalIndent(dumpData, "", "  ")
	if err != nil {
		respondEphemeral(session, event, fmt.Sprintf("❌ JSON encode failed: ```%v```", err))
		return nil
	}

	file := &discordgo.File{
		Name:        "datastore_dump.json",
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

	return nil
}

func init() {
	Register(WithGuildOnly(&DumpDBCommand{}))
}
