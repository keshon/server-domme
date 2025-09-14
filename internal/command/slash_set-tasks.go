package command

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
func (c *SetTasksCommand) RequireDev() bool    { return false }

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
	slash, ok := ctx.(*core.SlashContext)
	if !ok {
		return fmt.Errorf("invalid context")
	}

	session := slash.Session
	event := slash.Event
	storage := slash.Storage

	guildID := event.GuildID
	member := event.Member

	data := slash.Event.ApplicationCommandData()
	if len(data.Options) == 0 {
		return core.RespondEphemeral(slash.Session, slash.Event, "No file uploaded.")
	}

	attachmentID, ok := data.Options[0].Value.(string)
	if !ok {
		return core.RespondEphemeral(slash.Session, slash.Event, "Failed to get attachment ID.")
	}

	if slash.Event.ApplicationCommandData().Resolved == nil ||
		slash.Event.ApplicationCommandData().Resolved.Attachments == nil {
		return core.RespondEphemeral(slash.Session, slash.Event, "No attachments found in resolved data.")
	}

	attachment, exists := slash.Event.ApplicationCommandData().Resolved.Attachments[attachmentID]
	if !exists {
		return core.RespondEphemeral(slash.Session, slash.Event, "Failed to get the uploaded file.")
	}

	if !strings.HasSuffix(attachment.Filename, ".json") {
		return core.RespondEphemeral(slash.Session, slash.Event, "Only .json files are accepted.")
	}

	resp, err := http.Get(attachment.URL)
	if err != nil {
		log.Println("Failed to download file:", err)
		return core.RespondEphemeral(slash.Session, slash.Event, "Failed to download the uploaded file.")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("HTTP error when downloading file: %d %s", resp.StatusCode, resp.Status)
		return core.RespondEphemeral(slash.Session, slash.Event, "Failed to download the uploaded file.")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read file body:", err)
		return core.RespondEphemeral(slash.Session, slash.Event, "Failed to read the uploaded file.")
	}

	if len(body) == 0 {
		return core.RespondEphemeral(slash.Session, slash.Event, "Uploaded file is empty.")
	}

	var parsed []Task
	if err := json.Unmarshal(body, &parsed); err != nil {
		log.Printf("Failed to parse uploaded JSON: %v", err)
		return core.RespondEphemeral(slash.Session, slash.Event, fmt.Sprintf("Invalid JSON format: %v", err))
	}

	if len(parsed) == 0 {
		return core.RespondEphemeral(slash.Session, slash.Event, "No tasks found in the uploaded file.")
	}

	path := fmt.Sprintf("data/%s_tasks.json", guildID)

	if err := os.MkdirAll("data", 0755); err != nil {
		log.Println("Failed to create data directory:", err)
		return core.RespondEphemeral(slash.Session, slash.Event, "Failed to create data directory.")
	}

	if err := os.WriteFile(path, body, 0644); err != nil {
		log.Println("Failed to save file:", err)
		return core.RespondEphemeral(slash.Session, slash.Event, "Failed to save the uploaded file.")
	}

	err = core.LogCommand(session, storage, guildID, event.ChannelID, member.User.ID, member.User.Username, c.Name())
	if err != nil {
		log.Println("Failed to log:", err)
	}

	return core.RespondEphemeral(slash.Session, slash.Event, fmt.Sprintf("Successfully uploaded %d tasks for this guild.", len(parsed)))
}

func init() {
	core.RegisterCommand(
		core.ApplyMiddlewares(
			&SetTasksCommand{},
			core.WithGroupAccessCheck(),
			core.WithGuildOnly(),
		),
	)
}
