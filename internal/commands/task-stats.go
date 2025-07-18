package commands

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"server-domme/internal/config"
	"strings"

	"github.com/bwmarrin/discordgo"
)

func init() {
	Register(&Command{
		Sort:           400,
		Name:           "task-stats",
		Description:    "Show tasks statistics",
		Category:       "Tasks",
		DCSlashHandler: listTasksHandler,
	})

}

func listTasksHandler(ctx *SlashContext) {
	s, i := ctx.Session, ctx.InteractionCreate

	if !isAdmin(s, i.GuildID, i.Member) {
		respondEphemeral(s, i, "You must be an Admin to use this command, darling.")
		return
	}

	cfg := config.New()

	if len(tasks) == 0 {
		respondEphemeral(s, i, "No tasks found, darling. Either you're lazy or I'm losing my edge.")
		return
	}

	total := len(tasks)
	open := 0
	roleCounts := map[string]int{}
	rolesUsed := map[string]bool{}

	for _, t := range tasks {
		if len(t.RolesAllowed) == 0 {
			open++
		} else {
			for _, role := range t.RolesAllowed {
				roleCounts[role]++
				rolesUsed[role] = true
			}
		}
	}

	var b strings.Builder
	b.WriteString("```md\n")
	b.WriteString(fmt.Sprintf("# Task Statistics 📊\n"))
	b.WriteString(fmt.Sprintf("Total Tasks        : %d\n", total))
	b.WriteString(fmt.Sprintf("Open to Anyone     : %d\n", open))
	b.WriteString(fmt.Sprintf("Restricted Tasks   : %d\n", total-open))

	if len(roleCounts) > 0 {
		b.WriteString("\n# Roles in Use 🔐\n")
		for role, count := range roleCounts {
			b.WriteString(fmt.Sprintf("- %s: %d\n", role, count))
		}
	}
	b.WriteString("\n```")

	fileContent, err := os.ReadFile(cfg.TasksPath)
	if err != nil {
		log.Println("Failed to read tasks file:", err)
		respondEphemeral(s, i, "Couldn't read the tasks file. Try again later when I’m in a better mood.")
		return
	}

	file := &discordgo.File{
		Name:        "tasks.private.json",
		Reader:      bytes.NewReader(fileContent),
		ContentType: "application/json",
	}

	err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: b.String(),
			Files:   []*discordgo.File{file},
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Println("Failed to respond to list-tasks command:", err)
	}
}
