package task

import (
	"bytes"
	"fmt"
	"log"
	"os"
	"server-domme/internal/config"
	"server-domme/internal/core"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type DumpTasksCommand struct{}

func (c *DumpTasksCommand) Name() string { return "get-tasks" }
func (c *DumpTasksCommand) Description() string {
	return "Dumps all tasks for this server as JSON file"
}
func (c *DumpTasksCommand) Aliases() []string  { return []string{} }
func (c *DumpTasksCommand) Group() string      { return "task" }
func (c *DumpTasksCommand) Category() string   { return "⚙️ Settings" }
func (c *DumpTasksCommand) RequireAdmin() bool { return true }
func (c *DumpTasksCommand) RequireDev() bool   { return false }

func (c *DumpTasksCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
	}
}

func (c *DumpTasksCommand) Run(ctx interface{}) error {
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

	if len(tasks) == 0 {
		core.RespondEphemeral(session, event, "No tasks found, darling. Either you're lazy or I'm losing my edge.")
		return nil
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
	b.WriteString("# Task Statistics\n")
	b.WriteString(fmt.Sprintf("Total Tasks      : %d\n", total))
	b.WriteString(fmt.Sprintf("Open to Anyone   : %d\n", open))
	b.WriteString(fmt.Sprintf("Restricted Tasks : %d\n", total-open))

	if len(roleCounts) > 0 {
		b.WriteString("\n# Roles in Use\n")
		for role, count := range roleCounts {
			b.WriteString(fmt.Sprintf("- %s: %d\n", role, count))
		}
	}
	b.WriteString("\n```")

	cfg := config.New()
	fileContent, err := os.ReadFile(cfg.TasksPath)
	if err != nil {
		log.Println("Failed to read tasks file:", err)
		core.RespondEphemeral(session, event, "Couldn't read the tasks file. Try again later when I’m in a better mood.")
		return nil
	}

	file := &discordgo.File{
		Name:        fmt.Sprintf("%s_tasks.json", event.GuildID),
		Reader:      bytes.NewReader(fileContent),
		ContentType: "application/json",
	}

	err = session.InteractionRespond(event.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: b.String(),
			Files:   []*discordgo.File{file},
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
	if err != nil {
		log.Println("Failed to respond to dump-tasks:", err)
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
			&DumpTasksCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
