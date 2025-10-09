package task

import (
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

type SetTasksCommand struct{}

func (c *SetTasksCommand) Name() string        { return "set-tasks" }
func (c *SetTasksCommand) Description() string { return "Upload a new task list for this server" }
func (c *SetTasksCommand) Aliases() []string   { return []string{} }
func (c *SetTasksCommand) Group() string       { return "task" }
func (c *SetTasksCommand) Category() string    { return "⚙️ Settings" }
func (c *SetTasksCommand) RequireAdmin() bool  { return true }
func (c *SetTasksCommand) Permissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}
func (c *SetTasksCommand) BotPermissions() []int64 {
	return []int64{
		discordgo.PermissionAdministrator,
	}
}

func (c *SetTasksCommand) SlashDefinition() *discordgo.ApplicationCommand {
	return &discordgo.ApplicationCommand{
		Name:        c.Name(),
		Description: c.Description(),
		Options: []*discordgo.ApplicationCommandOption{
			{
				Type:        discordgo.ApplicationCommandOptionAttachment,
				Name:        "file",
				Description: "Upload a JSON file with tasks list",
				Required:    true,
			},
		},
	}
}

func (c *SetTasksCommand) Run(ctx interface{}) error {
	context, ok := ctx.(*core.SlashInteractionContext)
	if !ok {
		return nil
	}

	event := context.Event

	guildID := event.GuildID

	data := context.Event.ApplicationCommandData()
	if len(data.Options) == 0 {
		return core.RespondEphemeral(context.Session, context.Event, "No file uploaded.")
	}

	attachmentID, ok := data.Options[0].Value.(string)
	if !ok {
		return core.RespondEphemeral(context.Session, context.Event, "Failed to get attachment ID.")
	}

	if context.Event.ApplicationCommandData().Resolved == nil ||
		context.Event.ApplicationCommandData().Resolved.Attachments == nil {
		return core.RespondEphemeral(context.Session, context.Event, "No attachments found in resolved data.")
	}

	attachment, exists := context.Event.ApplicationCommandData().Resolved.Attachments[attachmentID]
	if !exists {
		return core.RespondEphemeral(context.Session, context.Event, "Failed to get the uploaded file.")
	}

	if !strings.HasSuffix(attachment.Filename, ".json") {
		return core.RespondEphemeral(context.Session, context.Event, "Only .json files are accepted.")
	}

	resp, err := http.Get(attachment.URL)
	if err != nil {
		log.Println("Failed to download file:", err)
		return core.RespondEphemeral(context.Session, context.Event, "Failed to download the uploaded file.")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("HTTP error when downloading file: %d %s", resp.StatusCode, resp.Status)
		return core.RespondEphemeral(context.Session, context.Event, "Failed to download the uploaded file.")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read file body:", err)
		return core.RespondEphemeral(context.Session, context.Event, "Failed to read the uploaded file.")
	}

	if len(body) == 0 {
		return core.RespondEphemeral(context.Session, context.Event, "Uploaded file is empty.")
	}

	var parsed []Task
	if err := json.Unmarshal(body, &parsed); err != nil {
		log.Printf("Failed to parse uploaded JSON: %v", err)
		return core.RespondEphemeral(context.Session, context.Event, fmt.Sprintf("Invalid JSON format: %v", err))
	}

	if len(parsed) == 0 {
		return core.RespondEphemeral(context.Session, context.Event, "No tasks found in the uploaded file.")
	}

	path := fmt.Sprintf("data/%s_tasks.json", guildID)

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Println("Failed to create data directory:", err)
		return core.RespondEphemeral(context.Session, context.Event, "Failed to create data directory.")
	}

	if err := os.WriteFile(path, body, 0644); err != nil {
		log.Println("Failed to save file:", err)
		return core.RespondEphemeral(context.Session, context.Event, "Failed to save the uploaded file.")
	}

	return core.RespondEphemeral(context.Session, context.Event, fmt.Sprintf("Successfully uploaded %d tasks for this guild.", len(parsed)))
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&SetTasksCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
			core.WithAccessControl(),
			core.WithPermissionCheck(),
			core.WithBotPermissionCheck(),
			core.WithCommandLogger(),
		),
	)
}
