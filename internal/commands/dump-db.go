package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Name:        "dump-db",
		Description: "Dump the entire datastore to a file.",
		Category:    "Debug",
		Sort:        999,
		DCSlashHandler: func(ctx *SlashContext) {
			dumpDbSlashHandler(ctx)
		},
	})
}

func dumpDbSlashHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate

	if !isDeveloper(ctx) {
		respondEphemeral(s, i, "🚫 You don't have permission to use this command.")
		return
	}

	dumpData, err := ctx.Storage.Dump()
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("❌ Failed to dump datastore: ```%v```", err))
		return
	}

	jsonBytes, err := json.MarshalIndent(dumpData, "", "  ")
	if err != nil {
		respondEphemeral(s, i, fmt.Sprintf("❌ JSON encode failed: ```%v```", err))
		return
	}

	file := &discordgo.File{
		Name:        "datastore_dump.json",
		ContentType: "application/json",
		Reader:      bytes.NewReader(jsonBytes),
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
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
}
