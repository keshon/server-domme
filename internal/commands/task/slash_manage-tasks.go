package task

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"server-domme/internal/core"
	"strings"

	"github.com/bwmarrin/discordgo"
)

type ManageTasksCommand struct{}

func (c *ManageTasksCommand) Name() string { return "manage-tasks" }
func (c *ManageTasksCommand) Description() string {
	return "Manage the tasks for this server"
}
func (c *ManageTasksCommand) Aliases() []string { return []string{} }
func (c *ManageTasksCommand) Group() string     { return "task" }
func (c *ManageTasksCommand) Category() string  { return "⚙️ Settings" }
func (c *ManageTasksCommand) UserPermissions() []int64 {
	return []int64{discordgo.PermissionAdministrator}
}

// SlashDefinition with subcommands: set, get, reset
func (c *ManageTasksCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "set",
				Description: "Upload a new task list",
				Options: []*discordgo.ApplicationCommandOption{
					{
						Type:        discordgo.ApplicationCommandOptionAttachment,
						Name:        "file",
						Description: "JSON file with tasks",
						Required:    true,
					},
				},
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "get",
				Description: "Download the current task list",
			},
			{
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Name:        "reset",
				Description: "Reset all tasks for this guild",
			},
		},
	}
}

func (c *ManageTasksCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	s := context.Session
	e := context.Event
	guildID := e.GuildID

	if err := core.RespondDeferredEphemeral(s, e); err != nil {
		log.Printf("[ERROR] Failed to defer interaction: %v", err)
		return err
	}

	data := e.ApplicationCommandData()
	if len(data.Options) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No subcommand provided.",
		})
	}

	sub := data.Options[0]
	switch sub.Name {
	case "set":
		return runSetTasks(s, e, guildID, sub, &data)
	case "get":
		return runGetTasks(s, e, guildID)
	case "reset":
		return runResetTasks(s, e, guildID)
	default:
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Unknown subcommand: %s", sub.Name),
		})
	}
}

func runSetTasks(s *discordgo.Session, e *discordgo.InteractionCreate, guildID string, sub *discordgo.ApplicationCommandInteractionDataOption, topLevelData *discordgo.ApplicationCommandInteractionData) error {
	if len(sub.Options) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No file uploaded.",
		})
	}

	attachmentOption := sub.Options[0]
	attachmentID, ok := attachmentOption.Value.(string)
	if !ok {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to retrieve attachment ID.",
		})
	}

	attachment, exists := topLevelData.Resolved.Attachments[attachmentID]
	if !exists {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to get the uploaded file.",
		})
	}

	if !strings.HasSuffix(strings.ToLower(attachment.Filename), ".json") {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Uploaded file is not a JSON file.",
		})
	}

	resp, err := http.Get(attachment.URL)
	if err != nil {
		log.Println("Failed to download file:", err)
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to download the uploaded file.",
		})
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to read the uploaded file or file is empty.",
		})
	}

	// for i, r := range string(body) {
	// 	if r > unicode.MaxASCII {
	// 		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
	// 			Description: fmt.Sprintf("Invalid file: non-ASCII character at byte %d", i),
	// 		})
	// 	}
	// }

	var parsed []Task
	if err := json.Unmarshal(body, &parsed); err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Invalid JSON format: %v", err),
		})
	}

	if len(parsed) == 0 {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Uploaded JSON file contains no tasks.",
		})
	}

	path := fmt.Sprintf("data/%s_task.list.json", guildID)
	if err := os.MkdirAll("data", 0755); err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to create data directory.",
		})
	}

	if err := os.WriteFile(path, body, 0644); err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to save the uploaded file.",
		})
	}

	return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: fmt.Sprintf("Successfully uploaded %d tasks for this guild.", len(parsed)),
	})
}

func runGetTasks(s *discordgo.Session, e *discordgo.InteractionCreate, guildID string) error {
	path := fmt.Sprintf("data/%s_task.list.json", guildID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No tasks found for this guild.",
		})
	}

	content, err := os.ReadFile(path)
	if err != nil {
		log.Println("Failed to read tasks file:", err)
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to read the tasks file.",
		})
	}

	file := &discordgo.File{
		Name:        fmt.Sprintf("%s_task.list.json", guildID),
		Reader:      bytes.NewReader(content),
		ContentType: "application/json",
	}

	_, err = s.FollowupMessageCreate(e.Interaction, true, &discordgo.WebhookParams{
		Content: fmt.Sprintf("Tasks for guild `%s`:", guildID),
		Files:   []*discordgo.File{file},
	})
	if err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "Failed to send tasks file.",
		})
	}

	return nil
}

func runResetTasks(s *discordgo.Session, e *discordgo.InteractionCreate, guildID string) error {
	path := fmt.Sprintf("data/%s_task.list.json", guildID)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: "No task list exists to reset.",
		})
	}

	if err := os.Remove(path); err != nil {
		return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
			Description: fmt.Sprintf("Failed to remove tasks file: %v", err),
		})
	}

	return core.FollowupEmbedEphemeral(s, e, &discordgo.MessageEmbed{
		Description: "Task list reset successfully.",
	})
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&ManageTasksCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithUserPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
